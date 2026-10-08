package computer

import (
	"context"
	"fmt"
	"image"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"cu/internal/session"
)

// Launch starts a graphical program and returns its PID without waiting for it.
func Launch(info session.Info, argv []string) (int, error) {
	if len(argv) == 0 || argv[0] == "" {
		return 0, ErrCommand
	}
	return session.StartApp(info, argv)
}

// Screenshot captures the desktop to a PNG and reports its pixel dimensions.
func Screenshot(ctx context.Context, info session.Info, out string) (path string, width, height int, err error) {
	path, err = filepath.Abs(out)
	if err != nil {
		return "", 0, 0, fmt.Errorf("resolve screenshot path: %w", err)
	}
	err = tool(ctx, info, nil, "grim", "-t", "png", path)
	if err != nil {
		return "", 0, 0, err
	}
	// #nosec G304 -- path is the output file explicitly selected by the caller.
	file, err := os.Open(path)
	if err != nil {
		return "", 0, 0, fmt.Errorf("open screenshot: %w", err)
	}
	defer func() { _ = file.Close() }()
	config, _, err := image.DecodeConfig(file)
	if err != nil {
		return "", 0, 0, fmt.Errorf("decode screenshot: %w", err)
	}
	return path, config.Width, config.Height, nil
}

// Type sends text through Labwc's virtual keyboard protocol.
func Type(ctx context.Context, info session.Info, value string) error {
	return tool(ctx, info, strings.NewReader(value), "wtype", "-s", "30", "-", "-s", "30")
}

// Key presses one named key, optionally with modifiers such as ctrl+shift+l.
func Key(ctx context.Context, info session.Info, combination string) error {
	parts := strings.Split(combination, "+")
	if len(parts) == 0 || parts[len(parts)-1] == "" {
		return fmt.Errorf("invalid key %q", combination)
	}
	args := make([]string, 0, len(parts)*4+4)
	// A fresh virtual keyboard needs a short settle before named key events.
	args = append(args, "-s", "30")
	for _, modifier := range parts[:len(parts)-1] {
		switch modifier {
		case "ctrl", "shift", "alt", "logo", "altgr":
			args = append(args, "-M", modifier)
		default:
			return fmt.Errorf("unsupported key modifier %q", modifier)
		}
	}
	args = append(args, "-k", parts[len(parts)-1])
	for index := len(parts) - 2; index >= 0; index-- {
		args = append(args, "-m", parts[index])
	}
	args = append(args, "-s", "30")
	return tool(ctx, info, nil, "wtype", args...)
}

func tool(ctx context.Context, info session.Info, stdin *strings.Reader, name string, args ...string) error {
	// #nosec G204 -- callers pass only the fixed grim and wtype program names.
	command := exec.CommandContext(ctx, name, args...)
	command.Env = session.Environment(info.Runtime, info.Display)
	if stdin != nil {
		command.Stdin = stdin
	}
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(string(output)))
	}
	return nil
}
