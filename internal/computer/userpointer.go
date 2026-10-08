package computer

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"cu/internal/session"

	virtualpointer "github.com/bnema/wayland-virtual-input-go/virtual_pointer"
)

// ErrUserPointer indicates a failed live viewer pointer operation.
var ErrUserPointer = errors.New("live user pointer")

// UserPointerError preserves the failed operation and its cause.
type UserPointerError struct {
	Operation string
	Cause     error
}

// Error describes the failed live pointer operation.
func (e *UserPointerError) Error() string { return e.Operation + ": " + e.Cause.Error() }

// Unwrap exposes the category and cause to errors.Is and errors.As.
func (e *UserPointerError) Unwrap() []error { return []error{ErrUserPointer, e.Cause} }

// UserController keeps one virtual pointer alive for ordered agent or viewer input.
type UserController struct {
	gate        chan struct{}
	command     *exec.Cmd
	input       io.WriteCloser
	encoder     *json.Encoder
	decoder     *json.Decoder
	width       int
	height      int
	closed      bool
	unavailable bool
	user        bool
}

type userPointerReply struct {
	Error string `json:"error,omitempty"`
}

// StartUserPointer opens a pointer worker tied to ctx and the selected desktop.
func StartUserPointer(ctx context.Context, info session.Info) (*UserController, error) {
	return startPointer(ctx, info, true)
}

// StartAgentPointer opens an agent pointer that persists across named desktop calls.
func StartAgentPointer(ctx context.Context, info session.Info) (*UserController, error) {
	return startPointer(ctx, info, false)
}

func startPointer(ctx context.Context, info session.Info, user bool) (*UserController, error) {
	width, height, err := screenSize(ctx, info)
	if err != nil {
		return nil, err
	}
	self, err := os.Executable()
	if err != nil {
		return nil, &UserPointerError{Operation: "find pointer executable", Cause: err}
	}
	worker := "_agent_pointer"
	if user {
		worker = "_user_pointer"
	}
	// #nosec G204 -- self is this executable and worker is one of two fixed private command names.
	command := exec.CommandContext(ctx, self, worker)
	command.Env = session.Environment(info.Runtime, info.Display)
	input, err := command.StdinPipe()
	if err != nil {
		return nil, &UserPointerError{Operation: "open pointer input", Cause: err}
	}
	output, err := command.StdoutPipe()
	if err != nil {
		_ = input.Close()
		return nil, &UserPointerError{Operation: "open pointer replies", Cause: err}
	}
	command.Stderr = os.Stderr
	err = command.Start()
	if err != nil {
		_ = input.Close()
		_ = output.Close()
		return nil, &UserPointerError{Operation: "start pointer worker", Cause: err}
	}
	return &UserController{
		command: command, input: input, encoder: json.NewEncoder(input), decoder: json.NewDecoder(output),
		width: width, height: height, gate: make(chan struct{}, 1), user: user,
	}, nil
}

// Send validates and delivers one live event, waiting for the worker's acknowledgement.
func (controller *UserController) Send(ctx context.Context, action *PointerAction) error {
	if cause := ctx.Err(); cause != nil {
		return &UserPointerError{Operation: "send pointer event", Cause: cause}
	}
	select {
	case controller.gate <- struct{}{}:
		defer func() { <-controller.gate }()
	case <-ctx.Done():
		return &UserPointerError{Operation: "wait for pointer event", Cause: ctx.Err()}
	}
	if cause := ctx.Err(); cause != nil {
		return &UserPointerError{Operation: "send pointer event", Cause: cause}
	}
	if controller.closed || controller.unavailable {
		return &UserPointerError{Operation: "send pointer event", Cause: os.ErrClosed}
	}
	if action == nil {
		return ErrPointer
	}
	request := *action
	request.Width, request.Height, request.User = controller.width, controller.height, controller.user
	err := validateStreamPointer(&request, controller.user)
	if err != nil {
		return err
	}
	cancelled := make(chan error, 1)
	stopCancellation := context.AfterFunc(ctx, func() {
		cancelled <- controller.command.Process.Kill()
	})
	err = controller.exchange(&request)
	if !stopCancellation() {
		killErr := <-cancelled
		controller.unavailable = true
		if errors.Is(killErr, os.ErrProcessDone) {
			killErr = nil
		}
		return &UserPointerError{Operation: "cancel pointer event", Cause: errors.Join(ctx.Err(), killErr, err)}
	}
	if err != nil {
		controller.unavailable = true
	}
	return err
}

func (controller *UserController) exchange(request *PointerAction) error {
	err := controller.encoder.Encode(request)
	if err != nil {
		return &UserPointerError{Operation: "send pointer event", Cause: err}
	}
	var reply userPointerReply
	err = controller.decoder.Decode(&reply)
	if err != nil {
		return &UserPointerError{Operation: "read pointer acknowledgement", Cause: err}
	}
	if reply.Error != "" {
		return &UserPointerError{Operation: "apply pointer event", Cause: errors.New(reply.Error)}
	}
	return nil
}

// Close releases the worker and any held buttons; repeated calls are safe.
func (controller *UserController) Close() error {
	controller.gate <- struct{}{}
	defer func() { <-controller.gate }()
	if controller.closed {
		return nil
	}
	controller.closed = true
	closeErr := controller.input.Close()
	waitResult := make(chan error, 1)
	go func() { waitResult <- controller.command.Wait() }()
	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()
	var waitErr, killErr error
	select {
	case waitErr = <-waitResult:
	case <-timer.C:
		killErr = controller.command.Process.Kill()
		waitErr = <-waitResult
		if killErr == nil && pointerKilled(waitErr) {
			waitErr = nil
		}
		if errors.Is(killErr, os.ErrProcessDone) {
			killErr = nil
		}
	}
	if cause := errors.Join(closeErr, killErr, waitErr); cause != nil {
		return &UserPointerError{Operation: "close pointer worker", Cause: cause}
	}
	return nil
}

func pointerKilled(err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	return ok && status.Signaled() && status.Signal() == syscall.SIGKILL
}

func validateUserPointer(action *PointerAction) error {
	request := *action
	switch request.Kind {
	case "press", "release":
		request.Kind = "click"
	case "move", "scroll":
	default:
		return ErrPointer
	}
	return request.validate()
}

func validateStreamPointer(action *PointerAction, user bool) error {
	if user {
		return validateUserPointer(action)
	}
	return action.validate()
}

// UserPointerWorker executes an acknowledged stream without changing the agent marker.
func UserPointerWorker(ctx context.Context, input io.Reader, output io.Writer) error {
	return pointerStreamWorker(ctx, input, output, true)
}

// AgentPointerWorker retains one pointer connection and reports each completed action.
func AgentPointerWorker(ctx context.Context, input io.Reader, output io.Writer) error {
	return pointerStreamWorker(ctx, input, output, false)
}

func pointerStreamWorker(ctx context.Context, input io.Reader, output io.Writer, user bool) error {
	manager, err := virtualpointer.NewVirtualPointerManager(ctx)
	if err != nil {
		return &UserPointerError{Operation: "connect live pointer", Cause: err}
	}
	defer func() { _ = manager.Close() }()
	device, err := manager.CreatePointer()
	if err != nil {
		return &UserPointerError{Operation: "create live pointer", Cause: err}
	}
	defer func() { _ = device.Close() }()
	decoder, encoder := json.NewDecoder(input), json.NewEncoder(output)
	held := make(map[string]bool, 3)
	var latest PointerAction
	for {
		var action PointerAction
		err = decoder.Decode(&action)
		if errors.Is(err, io.EOF) {
			for button := range held {
				latest.Kind, latest.Button = "release", button
				err = sendUserPointer(device, &latest)
				if err != nil {
					return &UserPointerError{Operation: "release live pointer", Cause: err}
				}
			}
			return nil
		}
		if err != nil {
			return &UserPointerError{Operation: "decode live pointer event", Cause: err}
		}
		action.User = user
		err = validateStreamPointer(&action, user)
		if err == nil {
			if user {
				err = sendUserPointer(device, &action)
			} else {
				err = sendAgentPointer(device, &action)
			}
		}
		if err == nil {
			latest = action
			switch action.Kind {
			case "press":
				held[action.Button] = true
			case "release":
				delete(held, action.Button)
			}
		}
		reply := userPointerReply{}
		if err != nil {
			reply.Error = err.Error()
		}
		replyErr := encoder.Encode(reply)
		if replyErr != nil {
			return &UserPointerError{Operation: "acknowledge live pointer event", Cause: replyErr}
		}
		if err != nil {
			return &UserPointerError{Operation: "apply live pointer event", Cause: err}
		}
	}
}

func sendUserPointer(device *virtualpointer.VirtualPointer, action *PointerAction) error {
	err := movePointer(device, action.X, action.Y, action.Width, action.Height)
	if err != nil {
		return err
	}
	switch action.Kind {
	case "move":
		return nil
	case "scroll":
		return scrollPointer(device, action)
	case "press", "release":
		button, buttonErr := buttonCode(action.Button)
		if buttonErr != nil {
			return buttonErr
		}
		state := virtualpointer.ButtonStatePressed
		if action.Kind == "release" {
			state = virtualpointer.ButtonStateReleased
		}
		err = pointerButton(device, button, state)
		if err != nil {
			return err
		}
		return device.Frame()
	default:
		return ErrPointer
	}
}
