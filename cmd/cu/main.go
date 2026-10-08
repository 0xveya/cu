package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"cu/internal/computer"
	"cu/internal/session"
	"cu/internal/waylandstate"

	"github.com/spf13/cobra"
)

func main() {
	code, err := execute(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "cu:", err)
		os.Exit(1)
	}
	os.Exit(code)
}

func execute(args []string) (int, error) {
	code := 0
	root := &cobra.Command{Use: "cu", Short: "Control an isolated Labwc desktop", SilenceUsage: true, SilenceErrors: true}
	root.SetArgs(args)
	for _, name := range []string{"start", "status", "stop"} {
		root.AddCommand(lifecycleCommand(name))
	}
	for _, name := range []string{"run", "launch"} {
		root.AddCommand(processCommand(name, &code))
	}
	for _, name := range []string{"screenshot", "type", "key"} {
		root.AddCommand(inputCommand(name))
	}
	for _, name := range []string{"move", "click", "drag", "scroll"} {
		root.AddCommand(pointerCommand(name))
	}
	root.AddCommand(windowsCommand(), recordCommand(), pointerWorkerCommand(), recordWorkerCommand(), mcpCommand(), viewerCommand(), pointerStreamWorkerCommand(true), pointerStreamWorkerCommand(false))
	return code, root.ExecuteContext(context.Background())
}

func required(command *cobra.Command, name string) {
	if err := command.MarkFlagRequired(name); err != nil {
		panic(err)
	}
}

func lifecycleCommand(name string) *cobra.Command {
	var id string
	var visible bool
	description := map[string]string{"start": "Start a Labwc desktop", "status": "Show a desktop's status", "stop": "Stop a desktop"}
	command := &cobra.Command{Use: name, Short: description[name], Args: cobra.NoArgs}
	command.Flags().StringVar(&id, "id", "", "session ID")
	required(command, "id")
	if name == "start" {
		command.Flags().BoolVar(&visible, "visible", false, "show the desktop in a host Wayland window")
	}
	command.RunE = func(_ *cobra.Command, _ []string) error {
		switch name {
		case "start":
			info, err := session.Start(id, visible)
			return output(info, err)
		case "status":
			info, err := session.Status(id)
			return output(info, err)
		default:
			if err := session.Stop(id); err != nil {
				return err
			}
			return output(struct {
				ID      string `json:"id"`
				Stopped bool   `json:"stopped"`
			}{id, true}, nil)
		}
	}
	return command
}

func processCommand(name string, code *int) *cobra.Command {
	var id, display string
	description := map[string]string{"run": "Run a command and wait", "launch": "Launch an app in the desktop"}
	command := &cobra.Command{Use: name + " --id ID -- COMMAND [ARG...]", Short: description[name], Args: cobra.MinimumNArgs(1)}
	command.Flags().StringVar(&id, "id", "", "managed session ID")
	if name == "run" {
		command.Flags().StringVar(&display, "display", "", "existing Wayland socket name")
	}
	command.RunE = func(cmd *cobra.Command, argv []string) error {
		if (id == "") == (display == "") || (name == "launch" && display != "") {
			return errors.New("select exactly one managed session with --id (or --display for run)")
		}
		if display != "" {
			var err error
			*code, err = computer.Run(cmd.Context(), display, argv)
			return err
		}
		info, err := session.Active(id)
		if err != nil {
			return err
		}
		if name == "run" {
			*code, err = computer.RunSession(cmd.Context(), info, argv)
			return err
		}
		pid, err := computer.Launch(info, argv)
		return output(struct {
			ID  string `json:"id"`
			PID int    `json:"pid"`
		}{id, pid}, err)
	}
	return command
}

func inputCommand(name string) *cobra.Command {
	var id, out, value string
	description := map[string]string{"screenshot": "Save a PNG of the desktop", "type": "Type text into the desktop", "key": "Press a named key or combination"}
	command := &cobra.Command{Use: name, Short: description[name], Args: cobra.NoArgs}
	command.Flags().StringVar(&id, "id", "", "managed session ID")
	required(command, "id")
	switch name {
	case "screenshot":
		command.Flags().StringVar(&out, "out", "", "PNG output path")
		required(command, "out")
	case "type":
		command.Flags().StringVar(&value, "text", "", "text to type")
		required(command, "text")
	case "key":
		command.Use = "key KEY"
		command.Args = cobra.ExactArgs(1)
	}
	command.RunE = func(cmd *cobra.Command, args []string) error {
		info, err := session.Active(id)
		if err != nil {
			return err
		}
		switch name {
		case "screenshot":
			path, width, height, err := computer.Screenshot(cmd.Context(), info, out)
			return output(struct {
				Path   string `json:"path"`
				Width  int    `json:"width"`
				Height int    `json:"height"`
			}{path, width, height}, err)
		case "type":
			return computer.Type(cmd.Context(), info, value)
		default:
			return computer.Key(cmd.Context(), info, args[0])
		}
	}
	return command
}

func pointerCommand(name string) *cobra.Command {
	var id, button, from, to string
	var x, y, deltaX, deltaY int
	description := map[string]string{"move": "Move the pointer", "click": "Click at a screenshot coordinate", "drag": "Drag between screenshot coordinates", "scroll": "Scroll at a screenshot coordinate"}
	command := &cobra.Command{Use: name, Short: description[name], Args: cobra.NoArgs}
	command.Flags().StringVar(&id, "id", "", "managed session ID")
	required(command, "id")
	if name == "drag" {
		command.Flags().StringVar(&from, "from", "", "start coordinate X,Y")
		command.Flags().StringVar(&to, "to", "", "end coordinate X,Y")
		required(command, "from")
		required(command, "to")
	} else {
		command.Flags().IntVar(&x, "x", -1, "horizontal screenshot coordinate")
		command.Flags().IntVar(&y, "y", -1, "vertical screenshot coordinate")
		required(command, "x")
		required(command, "y")
	}
	if name == "scroll" {
		command.Flags().IntVar(&deltaX, "delta-x", 0, "horizontal scroll pixels; positive is right")
		command.Flags().IntVar(&deltaY, "delta-y", 0, "vertical scroll pixels; positive is down")
	}
	if name == "click" {
		command.Flags().StringVar(&button, "button", "left", "left, right, or middle")
	}
	command.RunE = func(cmd *cobra.Command, _ []string) error {
		action := computer.PointerAction{Kind: name, X: x, Y: y, Button: button, DeltaX: deltaX, DeltaY: deltaY}
		if name == "drag" {
			var err error
			action.X, action.Y, err = point(from)
			if err != nil {
				return err
			}
			action.ToX, action.ToY, err = point(to)
			if err != nil {
				return err
			}
		}
		info, err := session.Active(id)
		if err != nil {
			return err
		}
		return computer.Pointer(cmd.Context(), info, &action)
	}
	return command
}

func windowsCommand() *cobra.Command {
	var id string
	command := &cobra.Command{Use: "windows", Short: "List visible windows as JSON", Args: cobra.NoArgs}
	command.Flags().StringVar(&id, "id", "", "managed session ID")
	required(command, "id")
	command.RunE = func(_ *cobra.Command, _ []string) error {
		info, err := session.Active(id)
		if err != nil {
			return err
		}
		items, err := waylandstate.List(info.Runtime, info.Display)
		return output(items, err)
	}
	return command
}

func recordCommand() *cobra.Command {
	root := &cobra.Command{Use: "record", Short: "Record the isolated desktop to MP4"}
	for _, name := range []string{"start", "status", "stop"} {
		var id, out string
		var fps, speed int
		description := map[string]string{"start": "Start a new MP4 recording", "status": "Show recording status", "stop": "Finish and save the recording"}
		command := &cobra.Command{Use: name, Short: description[name], Args: cobra.NoArgs}
		command.Flags().StringVar(&id, "id", "", "managed session ID")
		required(command, "id")
		if name == "start" {
			command.Flags().StringVar(&out, "out", "", "new MP4 output path")
			command.Flags().IntVar(&fps, "fps", 4, "frames per second, 1 to 30")
			command.Flags().IntVar(&speed, "speed", 4, "playback speed, 1 to 16 times real time")
			required(command, "out")
		}
		command.RunE = func(_ *cobra.Command, _ []string) error {
			info, err := session.Active(id)
			if err != nil {
				return err
			}
			var value computer.Recording
			switch name {
			case "start":
				value, err = computer.RecordStart(info, out, fps, speed)
			case "status":
				value, err = computer.RecordStatus(info)
			default:
				value, err = computer.RecordStop(info)
			}
			return output(value, err)
		}
		root.AddCommand(command)
	}
	return root
}

func pointerWorkerCommand() *cobra.Command {
	return &cobra.Command{Use: "_pointer ACTION", Hidden: true, Args: cobra.ExactArgs(1), RunE: func(_ *cobra.Command, args []string) error {
		if os.Getenv("WAYLAND_DISPLAY") != "wayland-0" || filepath.Base(filepath.Dir(filepath.Dir(os.Getenv("XDG_RUNTIME_DIR")))) != "cu" {
			return computer.ErrPointer
		}
		encoded := args[0]
		if encoded == "-" {
			data, err := io.ReadAll(io.LimitReader(os.Stdin, 2<<20))
			if err != nil {
				return err
			}
			encoded = string(data)
		}
		return computer.PointerWorker(encoded)
	}}
}

func recordWorkerCommand() *cobra.Command {
	var id, out string
	var fps, speed int
	command := &cobra.Command{Use: "_record", Hidden: true, Args: cobra.NoArgs}
	command.Flags().StringVar(&id, "id", "", "managed session ID")
	command.Flags().StringVar(&out, "out", "", "MP4 output path")
	command.Flags().IntVar(&fps, "fps", 0, "frames per second")
	command.Flags().IntVar(&speed, "speed", 0, "playback speed")
	command.RunE = func(_ *cobra.Command, _ []string) error {
		if id == "" || out == "" || fps < 1 || fps > 30 || speed < 1 || speed > 16 {
			return errors.New("invalid recording worker arguments")
		}
		return computer.RecordWorker(id, out, fps, speed)
	}
	return command
}

func point(value string) (x, y int, err error) {
	left, right, found := strings.Cut(value, ",")
	if !found {
		return 0, 0, errors.New("coordinate must be X,Y")
	}
	x, err = strconv.Atoi(left)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid x coordinate: %w", err)
	}
	y, err = strconv.Atoi(right)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid y coordinate: %w", err)
	}
	return x, y, nil
}

func output(value any, err error) error {
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(value); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}
