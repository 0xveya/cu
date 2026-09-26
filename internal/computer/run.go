// Package computer contains operations on an isolated Wayland desktop.
package computer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"cu/internal/session"
)

var (
	ErrDisplay = errors.New("invalid Wayland display")
	ErrCommand = errors.New("invalid command")
)

// Run starts argv on display with the caller's standard streams and returns its exit code.
func Run(ctx context.Context, display string, argv []string) (int, error) {
	if err := checkDisplay(display); err != nil {
		return 0, err
	}
	return run(ctx, append(os.Environ(), "WAYLAND_DISPLAY="+display), argv)
}

// RunSession runs argv on a managed desktop and returns its exit code.
func RunSession(ctx context.Context, info session.Info, argv []string) (int, error) {
	return run(ctx, session.Environment(info.Runtime, info.Display), argv)
}

func run(ctx context.Context, environment, argv []string) (int, error) {
	if len(argv) == 0 || argv[0] == "" {
		return 0, ErrCommand
	}

	// #nosec G204 -- the command is explicitly supplied by the cu run caller.
	command := exec.CommandContext(ctx, argv[0], argv[1:]...)
	command.Env = environment
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode(), nil
		}
		return 0, fmt.Errorf("run %q: %w", argv[0], err)
	}
	return 0, nil
}

func checkDisplay(display string) error {
	if display == "" || display == "." || display == ".." || strings.ContainsRune(display, '/') {
		return fmt.Errorf("%w: name must be a socket name in XDG_RUNTIME_DIR", ErrDisplay)
	}
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		return fmt.Errorf("%w: XDG_RUNTIME_DIR is unset", ErrDisplay)
	}
	// #nosec G703 -- display is checked to be one socket name without path separators.
	info, err := os.Stat(filepath.Join(runtimeDir, display))
	if err != nil {
		return fmt.Errorf("%w: %s: %w", ErrDisplay, display, err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("%w: %s is not a socket", ErrDisplay, display)
	}
	return nil
}
