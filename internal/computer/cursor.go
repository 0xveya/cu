package computer

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"cu/internal/session"
)

var (
	// ErrCursorState indicates a failed cursor state read or write.
	ErrCursorState = errors.New("cursor state")
	// ErrCursorFrame indicates a malformed frame that cannot receive the cursor overlay.
	ErrCursorFrame = errors.New("invalid recording frame")
)

// CursorError preserves the operation, stable category, and underlying cause.
type CursorError struct {
	Operation string
	Kind      error
	Cause     error
}

// Error describes the failed operation.
func (e *CursorError) Error() string {
	message := e.Operation + ": " + e.Kind.Error()
	if e.Cause != nil {
		message += ": " + e.Cause.Error()
	}
	return message
}

// Unwrap exposes the category and cause to errors.Is and errors.As.
func (e *CursorError) Unwrap() []error {
	if e.Cause == nil {
		return []error{e.Kind}
	}
	return []error{e.Kind, e.Cause}
}

type pointerState struct {
	X       int   `json:"x"`
	Y       int   `json:"y"`
	Clicked int64 `json:"clicked_at,omitempty"`
}

func savePointer(runtime string, x, y int, clicked bool) error {
	state := pointerState{X: x, Y: y}
	if clicked {
		state.Clicked = time.Now().UnixMilli()
	}
	data, err := json.Marshal(state)
	if err != nil {
		return &CursorError{Operation: "encode pointer", Kind: ErrCursorState, Cause: err}
	}
	file, err := os.CreateTemp(runtime, ".pointer-*")
	if err != nil {
		return &CursorError{Operation: "create pointer state", Kind: ErrCursorState, Cause: err}
	}
	defer func() {
		// #nosec G703 -- CreateTemp returned this path beneath the session runtime directory.
		_ = os.Remove(file.Name())
	}()
	_, err = file.Write(data)
	closeErr := file.Close()
	if err != nil {
		return &CursorError{Operation: "write pointer state", Kind: ErrCursorState, Cause: err}
	}
	if closeErr != nil {
		return &CursorError{Operation: "close pointer state", Kind: ErrCursorState, Cause: closeErr}
	}
	// #nosec G703 -- both paths are beneath the managed session runtime selected by the worker.
	err = os.Rename(file.Name(), filepath.Join(runtime, "pointer.json"))
	if err != nil {
		return &CursorError{Operation: "replace pointer state", Kind: ErrCursorState, Cause: err}
	}
	return nil
}

func overlayCursor(info session.Info, frame []byte) error {
	state, exists, err := readPointer(info)
	if err != nil || !exists {
		return err
	}
	pixels, width, height, err := ppmPixels(frame)
	if err != nil {
		return err
	}
	if state.X < 0 || state.Y < 0 || state.X >= width || state.Y >= height {
		return nil
	}
	paintCursor(pixels, width, height, width*3, 3, state, time.Now())
	return nil
}

func readPointer(info session.Info) (pointerState, bool, error) {
	data, err := os.ReadFile(filepath.Join(info.Runtime, "pointer.json"))
	if errors.Is(err, os.ErrNotExist) {
		return pointerState{}, false, nil
	}
	if err != nil {
		return pointerState{}, false, &CursorError{Operation: "read pointer state", Kind: ErrCursorState, Cause: err}
	}
	var state pointerState
	err = json.Unmarshal(data, &state)
	if err != nil {
		return pointerState{}, false, &CursorError{Operation: "decode pointer state", Kind: ErrCursorState, Cause: err}
	}
	return state, true, nil
}

func ppmPixels(frame []byte) (pixels []byte, width, height int, err error) {
	offset := 0
	tokens := make([]string, 0, 4)
	for range 4 {
		for offset < len(frame) {
			if ppmSpace(frame[offset]) {
				offset++
				continue
			}
			if frame[offset] == '#' {
				end := bytes.IndexByte(frame[offset:], '\n')
				if end < 0 {
					return nil, 0, 0, &CursorError{Operation: "decode PPM", Kind: ErrCursorFrame}
				}
				offset += end + 1
				continue
			}
			break
		}
		start := offset
		for offset < len(frame) && !ppmSpace(frame[offset]) {
			offset++
		}
		tokens = append(tokens, string(frame[start:offset]))
	}
	var widthErr, heightErr error
	width, widthErr = strconv.Atoi(tokens[1])
	height, heightErr = strconv.Atoi(tokens[2])
	if tokens[0] != "P6" || tokens[3] != "255" || widthErr != nil || heightErr != nil ||
		width <= 0 || height <= 0 || offset >= len(frame) {
		return nil, 0, 0, &CursorError{Operation: "decode PPM", Kind: ErrCursorFrame}
	}
	if frame[offset] == '\r' && offset+1 < len(frame) && frame[offset+1] == '\n' {
		offset++
	}
	pixels = frame[offset+1:]
	if width > len(pixels)/3 || height > len(pixels)/(width*3) || len(pixels) != width*height*3 {
		return nil, 0, 0, &CursorError{Operation: "decode PPM pixels", Kind: ErrCursorFrame}
	}
	return pixels, width, height, nil
}

func ppmSpace(value byte) bool {
	return value == ' ' || value == '\n' || value == '\r' || value == '\t'
}

var arrow = [...]image.Point{{0, 0}, {0, 29}, {8, 22}, {15, 36}, {21, 33}, {14, 19}, {25, 18}}

func arrowContains(x, y int) bool {
	inside := false
	previous := arrow[len(arrow)-1]
	for _, point := range arrow {
		if (point.Y > y) != (previous.Y > y) &&
			x < (previous.X-point.X)*(y-point.Y)/(previous.Y-point.Y)+point.X {
			inside = !inside
		}
		previous = point
	}
	return inside
}

func paintCursor(pixels []byte, width, height, stride, channels int, state pointerState, now time.Time) {
	age := now.UnixMilli() - state.Clicked
	radius := 12 + int(min(int64(600), max(int64(0), age)))*16/600
	ring := state.Clicked > 0 && age >= 0 && age < 600
	for y := max(0, state.Y-30); y < min(height, state.Y+42); y++ {
		for x := max(0, state.X-30); x < min(width, state.X+32); x++ {
			dx, dy := x-state.X, y-state.Y
			start := y*stride + x*channels
			pixel := pixels[start : start+3]
			distance := dx*dx + dy*dy
			if ring && distance >= (radius-2)*(radius-2) && distance <= radius*radius {
				blendPixel(pixel, 255, 138, 200, int(600-age)*210/600)
			}
			if arrowContains(dx, dy) {
				blendPixel(pixel, 255, 138, 200, 255)
				continue
			}
			if arrowContains(dx-1, dy) || arrowContains(dx+1, dy) || arrowContains(dx, dy-1) || arrowContains(dx, dy+1) {
				blendPixel(pixel, 255, 255, 255, 255)
				continue
			}
			if arrowContains(dx-2, dy-3) || arrowContains(dx-3, dy-3) {
				blendPixel(pixel, 0, 0, 0, 170)
			}
		}
	}
}

func blendPixel(pixel []byte, red, green, blue, alpha int) {
	for index, component := range [...]int{red, green, blue} {
		// #nosec G115 -- a weighted average of two bytes remains in [0, 255].
		pixel[index] = byte((int(pixel[index])*(255-alpha) + component*alpha) / 255)
	}
}
