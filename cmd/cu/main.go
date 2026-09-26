package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"cu/internal/computer"
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
	case "run":
		return runCommand(args[1:])
	default:
		return 0, fmt.Errorf("unknown command %q (try cu help)", args[0])
	}
}

func runCommand(args []string) (int, error) {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: cu run --display NAME -- COMMAND [ARG...]")
		flags.PrintDefaults()
	}
	display := flags.String("display", "", "Wayland socket name in XDG_RUNTIME_DIR")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0, nil
		}
		return 0, err
	}
	if *display == "" {
		return 0, fmt.Errorf("run requires --display")
	}
	return computer.Run(context.Background(), *display, flags.Args())
}

func usage() error {
	_, err := fmt.Fprintln(os.Stdout, "cu runs commands in an isolated Wayland desktop")
	if err != nil {
		return fmt.Errorf("write help: %w", err)
	}
	_, err = fmt.Fprintln(os.Stdout, "usage: cu run --display NAME -- COMMAND [ARG...]")
	if err != nil {
		return fmt.Errorf("write help: %w", err)
	}
	return nil
}
