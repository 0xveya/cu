package computer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"cu/internal/session"
	"cu/internal/waylandstate"

	virtualpointer "github.com/bnema/wayland-virtual-input-go/virtual_pointer"
)

// PointerAction is one mouse action in screenshot pixel coordinates.
type PointerAction struct {
	Kind       string  `json:"kind"`
	X          int     `json:"x"`
	Y          int     `json:"y"`
	ToX        int     `json:"to_x"`
	ToY        int     `json:"to_y"`
	Button     string  `json:"button"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	DeltaX     int     `json:"delta_x"`
	DeltaY     int     `json:"delta_y"`
	Points     []Point `json:"points,omitempty"`
	DurationMS int     `json:"duration_ms,omitempty"`
	User       bool    `json:"user,omitempty"`
}

// Point is one screenshot pixel coordinate in a held-button path.
type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

var ErrPointer = errors.New("invalid pointer action")

// Pointer sends an action to the session through a fresh virtual pointer connection.
func Pointer(ctx context.Context, info session.Info, action *PointerAction) error {
	action.User = false
	return pointer(ctx, info, action)
}

// UserPointer sends viewer input without replacing the agent's visible cursor marker.
func UserPointer(ctx context.Context, info session.Info, action *PointerAction) error {
	action.User = true
	return pointer(ctx, info, action)
}

func pointer(ctx context.Context, info session.Info, action *PointerAction) error {
	width, height, err := screenSize(ctx, info)
	if err != nil {
		return err
	}
	action.Width = width
	action.Height = height
	err = action.validate()
	if err != nil {
		return err
	}
	if action.Kind == "path" {
		action.X, action.Y = action.Points[0].X, action.Points[0].Y
	}
	data, err := json.Marshal(action)
	if err != nil {
		return fmt.Errorf("encode pointer action: %w", err)
	}
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find cu executable: %w", err)
	}
	// #nosec G204 -- self is the current cu executable; the action is validated data.
	command := exec.CommandContext(ctx, self, "_pointer", "-")
	command.Stdin = strings.NewReader(string(data))
	command.Env = session.Environment(info.Runtime, info.Display)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pointer action: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// PointerWorker executes a validated action using the session selected by its environment.
func PointerWorker(encoded string) error {
	var action PointerAction
	if err := json.Unmarshal([]byte(encoded), &action); err != nil {
		return fmt.Errorf("decode pointer action: %w", err)
	}
	if err := action.validate(); err != nil {
		return err
	}
	if action.Kind == "path" {
		action.X, action.Y = action.Points[0].X, action.Points[0].Y
	}
	manager, err := virtualpointer.NewVirtualPointerManager(context.Background())
	if err != nil {
		return fmt.Errorf("connect virtual pointer: %w", err)
	}
	defer func() { _ = manager.Close() }()
	device, err := manager.CreatePointer()
	if err != nil {
		return fmt.Errorf("create virtual pointer: %w", err)
	}
	defer func() { _ = device.Close() }()
	return sendAgentPointer(device, &action)
}

func sendAgentPointer(device *virtualpointer.VirtualPointer, action *PointerAction) error {
	err := movePointer(device, action.X, action.Y, action.Width, action.Height)
	if err != nil {
		return err
	}
	err = action.savePosition(action.X, action.Y, action.Kind == "click" || action.Kind == "drag" || action.Kind == "path")
	if err != nil {
		return err
	}
	switch action.Kind {
	case "move":
		return nil
	case "click":
		time.Sleep(30 * time.Millisecond)
		return clickPointer(device, action.Button)
	case "drag":
		time.Sleep(30 * time.Millisecond)
		return dragPointer(device, action)
	case "scroll":
		return scrollPointer(device, action)
	case "path":
		time.Sleep(30 * time.Millisecond)
		return pathPointer(device, action)
	default:
		return ErrPointer
	}
}

func (action *PointerAction) validate() error {
	if action.Kind == "path" {
		if len(action.Points) < 2 || len(action.Points) > 8192 || action.DurationMS < 0 || action.DurationMS > 5000 {
			return ErrPointer
		}
		for _, point := range action.Points {
			if point.X < 0 || point.Y < 0 || point.X >= action.Width || point.Y >= action.Height {
				return ErrPointer
			}
		}
		action.X, action.Y = action.Points[0].X, action.Points[0].Y
	}
	if action.Width <= 0 || action.Height <= 0 || action.X < 0 || action.Y < 0 ||
		action.X >= action.Width || action.Y >= action.Height ||
		uint64(action.Width) > uint64(^uint32(0)) || uint64(action.Height) > uint64(^uint32(0)) {
		return ErrPointer
	}
	switch action.Kind {
	case "move":
		return nil
	case "path":
		return nil
	case "scroll":
		if action.DeltaX < -10000 || action.DeltaX > 10000 || action.DeltaY < -10000 || action.DeltaY > 10000 ||
			(action.DeltaX == 0 && action.DeltaY == 0) {
			return ErrPointer
		}
		return nil
	case "click":
		_, err := buttonCode(action.Button)
		return err
	case "drag":
		if action.ToX < 0 || action.ToY < 0 || action.ToX >= action.Width || action.ToY >= action.Height {
			return ErrPointer
		}
		return nil
	default:
		return ErrPointer
	}
}

func screenSize(ctx context.Context, info session.Info) (width, height int, err error) {
	if err := ctx.Err(); err != nil {
		return 0, 0, err
	}
	return waylandstate.Size(info.Runtime, info.Display)
}

func movePointer(device *virtualpointer.VirtualPointer, x, y, width, height int) error {
	timestamp, err := pointerTime()
	if err != nil {
		return err
	}
	// #nosec G115 -- the validated coordinates and extents fit in uint32.
	err = device.MotionAbsolute(timestamp, uint32(x), uint32(y), uint32(width), uint32(height))
	if err != nil {
		return err
	}
	return device.Frame()
}

func clickPointer(device *virtualpointer.VirtualPointer, name string) error {
	button, err := buttonCode(name)
	if err != nil {
		return err
	}
	err = pointerButton(device, button, virtualpointer.ButtonStatePressed)
	if err != nil {
		return err
	}
	err = device.Frame()
	if err != nil {
		return err
	}
	time.Sleep(30 * time.Millisecond)
	err = pointerButton(device, button, virtualpointer.ButtonStateReleased)
	if err != nil {
		return err
	}
	err = device.Frame()
	if err != nil {
		return err
	}
	time.Sleep(30 * time.Millisecond)
	return nil
}

func dragPointer(device *virtualpointer.VirtualPointer, action *PointerAction) error {
	if err := pointerButton(device, virtualpointer.BTN_LEFT, virtualpointer.ButtonStatePressed); err != nil {
		return err
	}
	if err := device.Frame(); err != nil {
		return err
	}
	const steps = 20
	for step := 1; step <= steps; step++ {
		x := action.X + (action.ToX-action.X)*step/steps
		y := action.Y + (action.ToY-action.Y)*step/steps
		if err := movePointer(device, x, y, action.Width, action.Height); err != nil {
			return err
		}
		if stateErr := action.savePosition(x, y, true); stateErr != nil {
			return stateErr
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := pointerButton(device, virtualpointer.BTN_LEFT, virtualpointer.ButtonStateReleased); err != nil {
		return err
	}
	return device.Frame()
}

func scrollPointer(device *virtualpointer.VirtualPointer, action *PointerAction) error {
	err := device.AxisSource(virtualpointer.AxisSourceWheel)
	if err != nil {
		return err
	}
	if action.DeltaY != 0 {
		err = pointerAxis(device, virtualpointer.AxisVertical, float64(action.DeltaY))
		if err != nil {
			return err
		}
	}
	if action.DeltaX != 0 {
		err = pointerAxis(device, virtualpointer.AxisHorizontal, float64(action.DeltaX))
		if err != nil {
			return err
		}
	}
	return device.Frame()
}

func pathPointer(device *virtualpointer.VirtualPointer, action *PointerAction) error {
	err := pointerButton(device, virtualpointer.BTN_LEFT, virtualpointer.ButtonStatePressed)
	if err != nil {
		return err
	}
	err = device.Frame()
	if err != nil {
		return err
	}
	// Let the client process the button press before motion events can be coalesced.
	time.Sleep(30 * time.Millisecond)
	started := time.Now()
	published := started
	duration := time.Duration(action.DurationMS) * time.Millisecond
	for index, point := range action.Points[1:] {
		deadline := started.Add(duration * time.Duration(index+1) / time.Duration(len(action.Points)-1))
		if delay := time.Until(deadline); delay > 0 {
			time.Sleep(delay)
		}
		err = movePointer(device, point.X, point.Y, action.Width, action.Height)
		if err != nil {
			return err
		}
		if time.Since(published) >= 16*time.Millisecond || index == len(action.Points)-2 {
			err = action.savePosition(point.X, point.Y, true)
			if err != nil {
				return err
			}
			published = time.Now()
		}
	}
	time.Sleep(30 * time.Millisecond)
	err = pointerButton(device, virtualpointer.BTN_LEFT, virtualpointer.ButtonStateReleased)
	if err != nil {
		return err
	}
	err = device.Frame()
	if err != nil {
		return err
	}
	time.Sleep(30 * time.Millisecond)
	return nil
}

func (action *PointerAction) savePosition(x, y int, clicked bool) error {
	if action.User {
		return nil
	}
	return savePointer(os.Getenv("XDG_RUNTIME_DIR"), x, y, clicked)
}

func buttonCode(name string) (uint32, error) {
	switch name {
	case "left":
		return virtualpointer.BTN_LEFT, nil
	case "right":
		return virtualpointer.BTN_RIGHT, nil
	case "middle":
		return virtualpointer.BTN_MIDDLE, nil
	default:
		return 0, ErrPointer
	}
}
