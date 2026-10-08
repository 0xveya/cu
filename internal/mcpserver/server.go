// Package mcpserver exposes isolated desktops through typed MCP tools.
package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"cu/internal/computer"
	"cu/internal/session"
	"cu/internal/waylandstate"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var ErrArguments = errors.New("invalid computer use arguments")

// ArgumentError identifies invalid input before any desktop action is sent.
type ArgumentError struct{ Field string }

func (e *ArgumentError) Error() string { return ErrArguments.Error() + ": " + e.Field }
func (e *ArgumentError) Unwrap() error { return ErrArguments }

type sessionInput struct {
	ID        string `json:"id,omitempty" jsonschema:"Required for start, status, stop; explicit isolated desktop ID, never the host desktop"`
	Operation string `json:"operation" jsonschema:"start, status, stop, or list"`
	Visible   bool   `json:"visible,omitempty" jsonschema:"For start: open a nested desktop window on the host"`
}
type sessionOutput struct {
	Sessions []session.Info `json:"sessions"`
}
type launchInput struct {
	ID   string   `json:"id"`
	Argv []string `json:"argv" jsonschema:"Program and separate arguments, without shell interpolation"`
}
type launchOutput struct {
	PID int `json:"pid"`
}
type observeInput struct {
	ID          string `json:"id"`
	WindowsOnly bool   `json:"windows_only,omitempty" jsonschema:"Return window titles without capturing an image"`
}
type observation struct {
	ID        string                `json:"id"`
	Width     int                   `json:"width,omitempty"`
	Height    int                   `json:"height,omitempty"`
	Completed int                   `json:"completed,omitempty"`
	Windows   []waylandstate.Window `json:"windows"`
}
type action struct {
	Points       []computer.Point `json:"points,omitempty" jsonschema:"For path: 2 to 8192 pixel points, drawn with the left button held"`
	DurationMS   int              `json:"duration_ms,omitempty" jsonschema:"Path duration in milliseconds, 0 to 5000"`
	Kind         string           `json:"kind" jsonschema:"move, click, drag, path, scroll, type, key, or wait"`
	X            int              `json:"x,omitempty"`
	Y            int              `json:"y,omitempty"`
	ToX          int              `json:"to_x,omitempty"`
	ToY          int              `json:"to_y,omitempty"`
	Button       string           `json:"button,omitempty" jsonschema:"left (default), right, or middle"`
	DeltaX       int              `json:"delta_x,omitempty" jsonschema:"Scroll pixels; positive is right"`
	DeltaY       int              `json:"delta_y,omitempty" jsonschema:"Scroll pixels; positive is down"`
	Text         string           `json:"text,omitempty"`
	Key          string           `json:"key,omitempty" jsonschema:"wtype key name or combination such as ctrl+l"`
	Milliseconds int              `json:"milliseconds,omitempty" jsonschema:"Wait duration, 0 to 2000"`
}
type actInput struct {
	ID      string   `json:"id"`
	Actions []action `json:"actions" jsonschema:"Up to 64 ordered actions. Stops on first failure; earlier actions remain applied"`
	Observe *bool    `json:"observe,omitempty" jsonschema:"Return one inline image after the batch (default true); false returns only completion count"`
}
type recordInput struct {
	ID        string `json:"id"`
	Operation string `json:"operation" jsonschema:"start, status, or stop"`
	Out       string `json:"out,omitempty" jsonschema:"New MP4 path for start"`
	FPS       int    `json:"fps,omitempty" jsonschema:"Frames per second (default 8), 1 to 30"`
	Speed     int    `json:"speed,omitempty" jsonschema:"Playback multiplier (default 1), 1 to 16"`
}

type pointerEntry struct {
	PID        int
	StartTime  uint64
	Controller *computer.UserController
}

type desktops struct {
	ctx      context.Context
	mutex    sync.Mutex
	pointers map[string]pointerEntry
}

func (d *desktops) pointer(info session.Info) (*computer.UserController, error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	entry, found := d.pointers[info.ID]
	if found && entry.PID == info.PID && entry.StartTime == info.StartTime {
		return entry.Controller, nil
	}
	if found {
		_ = entry.Controller.Close()
		delete(d.pointers, info.ID)
	}
	controller, err := computer.StartAgentPointer(d.ctx, info)
	if err != nil {
		return nil, err
	}
	d.pointers[info.ID] = pointerEntry{PID: info.PID, StartTime: info.StartTime, Controller: controller}
	return controller, nil
}

func (d *desktops) forget(id string) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	entry, found := d.pointers[id]
	if !found {
		return nil
	}
	delete(d.pointers, id)
	return entry.Controller.Close()
}

func (d *desktops) close() {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	for id, entry := range d.pointers {
		_ = entry.Controller.Close()
		delete(d.pointers, id)
	}
}

// New constructs a server with explicit desktop IDs and pointer workers owned by ctx.
func New(ctx context.Context) *mcp.Server {
	state := &desktops{ctx: ctx, pointers: make(map[string]pointerEntry)}
	context.AfterFunc(ctx, state.close)
	server := mcp.NewServer(&mcp.Implementation{Name: "cu", Version: "0.2.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "desktop_session", Description: "Start, inspect, list, or stop isolated Labwc desktops. Desktop lifecycle survives MCP connections."}, state.lifecycle)
	mcp.AddTool(server, &mcp.Tool{Name: "desktop_launch", Description: "Launch a GUI program in an isolated desktop."}, launch)
	mcp.AddTool(server, &mcp.Tool{Name: "desktop_observe", Description: "See the desktop as inline image content and window titles. Coordinates are original image pixels.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, observe)
	mcp.AddTool(server, &mcp.Tool{Name: "desktop_act", Description: "Perform ordered computer input, then return a single inline image. Batch known actions to reduce model round trips. Do not run concurrent input calls on the same desktop."}, state.act)
	mcp.AddTool(server, &mcp.Tool{Name: "desktop_record", Description: "Start, inspect, or finish a desktop MP4 with visible agent cursor."}, record)
	return server
}

// Handler serves the new stateless protocol and compatible older HTTP clients.
func Handler(server *mcp.Server) http.Handler {
	handler := mcp.NewStreamableHTTPHandler(func(_ *http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, PropagateRequestCancellation: true})
	return http.NewCrossOriginProtection().Handler(handler)
}

func (d *desktops) lifecycle(_ context.Context, _ *mcp.CallToolRequest, input sessionInput) (*mcp.CallToolResult, sessionOutput, error) {
	var info session.Info
	var err error
	switch input.Operation {
	case "list":
		items, listErr := session.List()
		return nil, sessionOutput{Sessions: items}, listErr
	case "start":
		info, err = session.Start(input.ID, input.Visible)
	case "status":
		info, err = session.Status(input.ID)
	case "stop":
		closeErr := d.forget(input.ID)
		err = errors.Join(closeErr, session.Stop(input.ID))
		info.ID = input.ID
	default:
		err = &ArgumentError{Field: "operation must be start, status, stop, or list"}
	}
	return nil, sessionOutput{Sessions: []session.Info{info}}, err
}

func launch(_ context.Context, _ *mcp.CallToolRequest, input launchInput) (*mcp.CallToolResult, launchOutput, error) {
	info, err := session.Active(input.ID)
	if err != nil {
		return nil, launchOutput{}, err
	}
	pid, err := computer.Launch(info, input.Argv)
	return nil, launchOutput{PID: pid}, err
}

func observe(ctx context.Context, _ *mcp.CallToolRequest, input observeInput) (*mcp.CallToolResult, observation, error) {
	info, err := session.Active(input.ID)
	if err != nil {
		return nil, observation{}, err
	}
	return snapshot(ctx, info, !input.WindowsOnly, 0)
}

func snapshot(ctx context.Context, info session.Info, image bool, completed int) (*mcp.CallToolResult, observation, error) {
	output := observation{ID: info.ID, Completed: completed}
	windows, err := waylandstate.List(info.Runtime, info.Display)
	if err != nil {
		return nil, output, err
	}
	output.Windows = windows
	if !image {
		return nil, output, nil
	}
	frame, err := computer.Capture(ctx, info)
	if err != nil {
		return nil, output, err
	}
	output.Width, output.Height = frame.Width, frame.Height
	windowJSON, err := json.Marshal(windows)
	if err != nil {
		return nil, output, err
	}
	result := &mcp.CallToolResult{Content: []mcp.Content{
		&mcp.TextContent{Text: fmt.Sprintf("Desktop %s: %d×%d pixels. Completed %d actions.", info.ID, frame.Width, frame.Height, completed) + " Windows: " + string(windowJSON)},
		&mcp.ImageContent{Data: frame.PNG, MIMEType: "image/png", Meta: mcp.Meta{"codex/imageDetail": "original"}, Annotations: &mcp.Annotations{Audience: []mcp.Role{"assistant"}}},
	}}
	return result, output, nil
}

func (d *desktops) act(ctx context.Context, _ *mcp.CallToolRequest, input actInput) (*mcp.CallToolResult, observation, error) {
	if len(input.Actions) == 0 || len(input.Actions) > 64 {
		return nil, observation{}, &ArgumentError{Field: "actions must contain 1 to 64 items"}
	}
	for index := range input.Actions {
		item := &input.Actions[index]
		switch item.Kind {
		case "move", "click", "drag", "path", "scroll", "type", "key":
		case "wait":
			if item.Milliseconds < 0 || item.Milliseconds > 2000 {
				return nil, observation{}, &ArgumentError{Field: "wait milliseconds must be 0 to 2000"}
			}
		default:
			return nil, observation{}, &ArgumentError{Field: "unknown action kind: " + item.Kind}
		}
	}
	info, err := session.Active(input.ID)
	if err != nil {
		return nil, observation{}, err
	}
	for index := range input.Actions {
		item := &input.Actions[index]
		err = d.perform(ctx, info, item)
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Completed %d actions; action %d (%s) failed: %s. Earlier actions remain applied; observe before retrying.", index, index+1, item.Kind, err)}}}, observation{ID: info.ID, Completed: index}, nil
		}
	}
	wantImage := input.Observe == nil || *input.Observe
	if !wantImage {
		return nil, observation{ID: info.ID, Completed: len(input.Actions), Windows: []waylandstate.Window{}}, nil
	}
	result, output, err := snapshot(ctx, info, true, len(input.Actions))
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Completed all %d actions, but observation failed: %s. Input remains applied; observe before retrying.", len(input.Actions), err)}}}, output, nil
	}
	return result, output, nil
}

func (d *desktops) perform(ctx context.Context, info session.Info, item *action) error {
	switch item.Kind {
	case "type":
		return computer.Type(ctx, info, item.Text)
	case "key":
		return computer.Key(ctx, info, item.Key)
	case "wait":
		timer := time.NewTimer(time.Duration(item.Milliseconds) * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	default:
		button := item.Button
		if button == "" {
			button = "left"
		}
		controller, err := d.pointer(info)
		if err != nil {
			return err
		}
		err = controller.Send(ctx, &computer.PointerAction{Kind: item.Kind, X: item.X, Y: item.Y, ToX: item.ToX, ToY: item.ToY, Button: button, DeltaX: item.DeltaX, DeltaY: item.DeltaY, Points: item.Points, DurationMS: item.DurationMS})
		if err != nil {
			_ = d.forget(info.ID)
		}
		return err
	}
}

func record(_ context.Context, _ *mcp.CallToolRequest, input recordInput) (*mcp.CallToolResult, computer.Recording, error) {
	info, err := session.Active(input.ID)
	if err != nil {
		return nil, computer.Recording{}, err
	}
	var value computer.Recording
	switch input.Operation {
	case "start":
		if input.Out == "" {
			return nil, value, &ArgumentError{Field: "out is required for start"}
		}
		if input.FPS == 0 {
			input.FPS = 8
		}
		if input.Speed == 0 {
			input.Speed = 1
		}
		value, err = computer.RecordStart(info, input.Out, input.FPS, input.Speed)
	case "status":
		value, err = computer.RecordStatus(info)
	case "stop":
		value, err = computer.RecordStop(info)
	default:
		err = &ArgumentError{Field: "operation must be start, status, or stop"}
	}
	return nil, value, err
}
