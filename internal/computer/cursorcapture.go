package computer

import (
	"bytes"
	"context"
	"image"
	"image/draw"
	"image/png"
	"time"

	"cu/internal/session"
)

// CaptureWithCursor captures a viewer frame with the agent's pink pointer marker.
func CaptureWithCursor(ctx context.Context, info session.Info) (Frame, error) {
	frame, err := Capture(ctx, info)
	if err != nil {
		return Frame{}, err
	}
	state, exists, err := readPointer(info)
	if err != nil {
		return Frame{}, err
	}
	if !exists || state.X < 0 || state.Y < 0 || state.X >= frame.Width || state.Y >= frame.Height {
		return frame, nil
	}
	decoded, err := png.Decode(bytes.NewReader(frame.PNG))
	if err != nil {
		return Frame{}, &CaptureError{Operation: "decode viewer frame", Cause: err}
	}
	var pixels []byte
	var stride int
	switch canvas := decoded.(type) {
	case *image.RGBA:
		pixels, stride = canvas.Pix, canvas.Stride
	case *image.NRGBA:
		pixels, stride = canvas.Pix, canvas.Stride
	default:
		rgba := image.NewRGBA(decoded.Bounds())
		draw.Draw(rgba, rgba.Bounds(), decoded, decoded.Bounds().Min, draw.Src)
		decoded = rgba
		pixels, stride = rgba.Pix, rgba.Stride
	}
	paintCursor(pixels, frame.Width, frame.Height, stride, 4, state, time.Now())
	var output bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	err = encoder.Encode(&output, decoded)
	if err != nil {
		return Frame{}, &CaptureError{Operation: "encode viewer frame", Cause: err}
	}
	frame.PNG = output.Bytes()
	return frame, nil
}
