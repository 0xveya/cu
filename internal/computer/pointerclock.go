package computer

import (
	"errors"
	"time"

	virtualpointer "github.com/bnema/wayland-virtual-input-go/virtual_pointer"
	"golang.org/x/sys/unix"
)

// ErrPointerClock identifies a failed read of the compositor's monotonic clock.
var ErrPointerClock = errors.New("pointer clock")

// PointerClockError preserves the cause of a failed protocol timestamp read.
type PointerClockError struct{ Cause error }

// Error describes the clock operation that failed.
func (e *PointerClockError) Error() string { return "read pointer clock: " + e.Cause.Error() }

// Unwrap exposes the stable category and syscall cause.
func (e *PointerClockError) Unwrap() []error { return []error{ErrPointerClock, e.Cause} }

func pointerTime() (time.Time, error) {
	var timestamp unix.Timespec
	err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &timestamp)
	if err != nil {
		return time.Time{}, &PointerClockError{Cause: err}
	}
	// The library serializes UnixNano; encode compositor uptime into that field.
	return time.Unix(0, timestamp.Nano()), nil
}

func pointerButton(device *virtualpointer.VirtualPointer, button uint32, state virtualpointer.ButtonState) error {
	timestamp, err := pointerTime()
	if err != nil {
		return err
	}
	return device.Button(timestamp, button, state)
}

func pointerAxis(device *virtualpointer.VirtualPointer, axis virtualpointer.Axis, value float64) error {
	timestamp, err := pointerTime()
	if err != nil {
		return err
	}
	return device.Axis(timestamp, axis, value)
}
