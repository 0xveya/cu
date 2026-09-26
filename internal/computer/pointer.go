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
	Kind   string `json:"kind"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	ToX    int    `json:"to_x"`
	ToY    int    `json:"to_y"`
	Button string `json:"button"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

var ErrPointer = errors.New("invalid pointer action")

// Pointer sends an action to the session through a fresh virtual pointer connection.
func Pointer(ctx context.Context, info session.Info, action *PointerAction) error {
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
	data, err := json.Marshal(action)
	if err != nil {
		return fmt.Errorf("encode pointer action: %w", err)
	}
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find cu executable: %w", err)
	}
	// #nosec G204 -- self is the current cu executable; the action is validated data.
	command := exec.CommandContext(ctx, self, "_pointer", string(data))
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
	if err := movePointer(device, action.X, action.Y, action.Width, action.Height); err != nil {
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
		return dragPointer(device, &action)
	default:
		return ErrPointer
	}
}

func (action *PointerAction) validate() error {
	if action.Width <= 0 || action.Height <= 0 || action.X < 0 || action.Y < 0 ||
		action.X >= action.Width || action.Y >= action.Height ||
		uint64(action.Width) > uint64(^uint32(0)) || uint64(action.Height) > uint64(^uint32(0)) {
		return ErrPointer
	}
	switch action.Kind {
	case "move":
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
	// #nosec G115 -- the validated coordinates and extents fit in uint32.
	if err := device.MotionAbsolute(time.Now(), uint32(x), uint32(y), uint32(width), uint32(height)); err != nil {
		return err
	}
	return device.Frame()
}

func clickPointer(device *virtualpointer.VirtualPointer, name string) error {
	button, err := buttonCode(name)
	if err != nil {
		return err
	}
	if err := device.Button(time.Now(), button, virtualpointer.ButtonStatePressed); err != nil {
		return err
	}
	if err := device.Frame(); err != nil {
		return err
	}
	time.Sleep(30 * time.Millisecond)
	if err := device.Button(time.Now(), button, virtualpointer.ButtonStateReleased); err != nil {
		return err
	}
	return device.Frame()
}

func dragPointer(device *virtualpointer.VirtualPointer, action *PointerAction) error {
	if err := device.Button(time.Now(), virtualpointer.BTN_LEFT, virtualpointer.ButtonStatePressed); err != nil {
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
		time.Sleep(20 * time.Millisecond)
	}
	if err := device.Button(time.Now(), virtualpointer.BTN_LEFT, virtualpointer.ButtonStateReleased); err != nil {
		return err
	}
	return device.Frame()
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
