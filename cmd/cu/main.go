package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"cu/internal/computer"
	"cu/internal/session"
)

func main() {
	code, err := run(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "cu:", err)
		os.Exit(1)
	}
	os.Exit(code)
}

func run(args []string) (int, error) {
	if len(args) == 0 {
		return 0, usage()
	}
	switch args[0] {
	case "help", "-h", "--help":
		return 0, usage()
	case "start", "status", "stop":
		return lifecycle(args[0], args[1:])
	case "run", "launch":
		return command(args[0], args[1:])
	case "screenshot", "type", "key":
		return input(args[0], args[1:])
	case "move", "click", "drag":
		return mouse(args[0], args[1:])
	case "_pointer":
		if len(args) != 2 || os.Getenv("WAYLAND_DISPLAY") != "wayland-0" ||
			filepath.Base(filepath.Dir(filepath.Dir(os.Getenv("XDG_RUNTIME_DIR")))) != "cu" {
			return 0, computer.ErrPointer
		}
		return 0, computer.PointerWorker(args[1])
	default:
		return 0, fmt.Errorf("unknown command %q (try cu help)", args[0])
	}
}

func mouse(name string, args []string) (int, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	id := flags.String("id", "", "managed session ID")
	x := flags.Int("x", -1, "horizontal screenshot coordinate")
	y := flags.Int("y", -1, "vertical screenshot coordinate")
	button := flags.String("button", "left", "left, right, or middle (click)")
	from := flags.String("from", "", "start coordinate X,Y (drag)")
	to := flags.String("to", "", "end coordinate X,Y (drag)")
	if err := flags.Parse(args); err != nil {
		return flagError(err)
	}
	if *id == "" || flags.NArg() != 0 {
		return 0, fmt.Errorf("usage: cu %s --id ID [coordinates]", name)
	}
	action := computer.PointerAction{Kind: name, X: *x, Y: *y, Button: *button}
	if name == "drag" {
		var err error
		action.X, action.Y, err = point(*from)
		if err != nil {
			return 0, err
		}
		action.ToX, action.ToY, err = point(*to)
		if err != nil {
			return 0, err
		}
	}
	info, err := session.Active(*id)
	if err != nil {
		return 0, err
	}
	return 0, computer.Pointer(context.Background(), info, &action)
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

func lifecycle(name string, args []string) (int, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	id := flags.String("id", "", "session ID")
	var visible *bool
	if name == "start" {
		visible = flags.Bool("visible", false, "show the desktop as a window on the host Wayland display")
	}
	if err := flags.Parse(args); err != nil {
		return flagError(err)
	}
	if *id == "" || flags.NArg() != 0 {
		if name == "start" {
			return 0, errors.New("usage: cu start --id ID [--visible]")
		}
		return 0, fmt.Errorf("usage: cu %s --id ID", name)
	}
	switch name {
	case "start":
		info, err := session.Start(*id, *visible)
		return 0, output(info, err)
	case "status":
		info, err := session.Status(*id)
		return 0, output(info, err)
	default:
		if err := session.Stop(*id); err != nil {
			return 0, err
		}
		return 0, output(struct {
			ID      string `json:"id"`
			Stopped bool   `json:"stopped"`
		}{*id, true}, nil)
	}
}

func command(name string, args []string) (int, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	id := flags.String("id", "", "managed session ID")
	display := flags.String("display", "", "existing Wayland socket name (run only)")
	if err := flags.Parse(args); err != nil {
		return flagError(err)
	}
	argv := flags.Args()
	if len(argv) == 0 || (*id == "" && *display == "") || (*id != "" && *display != "") || (name == "launch" && *display != "") {
		return 0, fmt.Errorf("usage: cu %s --id ID -- COMMAND [ARG...]", name)
	}
	ctx := context.Background()
	if *display != "" {
		return computer.Run(ctx, *display, argv)
	}
	info, err := session.Active(*id)
	if err != nil {
		return 0, err
	}
	if name == "run" {
		return computer.RunSession(ctx, info, argv)
	}
	pid, err := computer.Launch(info, argv)
	return 0, output(struct {
		ID  string `json:"id"`
		PID int    `json:"pid"`
	}{*id, pid}, err)
}

func input(name string, args []string) (int, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	id := flags.String("id", "", "managed session ID")
	out := flags.String("out", "", "PNG output path (screenshot)")
	value := flags.String("text", "", "text to type (type)")
	if err := flags.Parse(args); err != nil {
		return flagError(err)
	}
	if *id == "" {
		return 0, fmt.Errorf("%s requires --id ID", name)
	}
	info, err := session.Active(*id)
	if err != nil {
		return 0, err
	}
	ctx := context.Background()
	switch name {
	case "screenshot":
		if *out == "" || flags.NArg() != 0 {
			return 0, errors.New("usage: cu screenshot --id ID --out FILE.png")
		}
		path, width, height, err := computer.Screenshot(ctx, info, *out)
		return 0, output(struct {
			Path   string `json:"path"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		}{path, width, height}, err)
	case "type":
		if flags.NArg() != 0 {
			return 0, errors.New("usage: cu type --id ID --text TEXT")
		}
		return 0, computer.Type(ctx, info, *value)
	default:
		if flags.NArg() != 1 {
			return 0, errors.New("usage: cu key --id ID KEY (example: ctrl+l)")
		}
		return 0, computer.Key(ctx, info, flags.Arg(0))
	}
}

func flagError(err error) (int, error) {
	if errors.Is(err, flag.ErrHelp) {
		return 0, nil
	}
	return 0, err
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

func usage() error {
	_, err := fmt.Fprintln(os.Stdout, "cu controls an isolated Labwc desktop")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(os.Stdout, "commands: start, status, stop, run, launch, screenshot, type, key, move, click, drag")
	return err
}
