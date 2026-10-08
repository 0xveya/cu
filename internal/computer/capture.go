package computer

import (
	"bytes"
	"context"
	"errors"
	"image"
	"os/exec"

	"cu/internal/session"
)

// Frame is an unscaled desktop PNG with its coordinate space.
type Frame struct {
	PNG    []byte
	Width  int
	Height int
}

var ErrCapture = errors.New("capture desktop")

// CaptureError preserves the failed capture operation and its cause.
type CaptureError struct {
	Operation string
	Cause     error
}

func (e *CaptureError) Error() string   { return e.Operation + ": " + e.Cause.Error() }
func (e *CaptureError) Unwrap() []error { return []error{ErrCapture, e.Cause} }

// Capture returns a desktop image directly, without a temporary file.
func Capture(ctx context.Context, info session.Info) (Frame, error) {
	command := exec.CommandContext(ctx, "grim", "-t", "png", "-l", "1", "-")
	command.Env = session.Environment(info.Runtime, info.Display)
	var diagnostic bytes.Buffer
	command.Stderr = &diagnostic
	pixels, err := command.Output()
	if err != nil {
		return Frame{}, &CaptureError{Operation: "grim " + diagnostic.String(), Cause: err}
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(pixels))
	if err != nil {
		return Frame{}, &CaptureError{Operation: "decode desktop", Cause: err}
	}
	return Frame{PNG: pixels, Width: config.Width, Height: config.Height}, nil
}
