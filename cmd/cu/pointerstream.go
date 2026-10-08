package main

import (
	"os"
	"path/filepath"

	"cu/internal/computer"

	"github.com/spf13/cobra"
)

func pointerStreamWorkerCommand(user bool) *cobra.Command {
	name := "_agent_pointer"
	if user {
		name = "_user_pointer"
	}
	return &cobra.Command{Use: name, Hidden: true, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if os.Getenv("WAYLAND_DISPLAY") != "wayland-0" || filepath.Base(filepath.Dir(filepath.Dir(os.Getenv("XDG_RUNTIME_DIR")))) != "cu" {
			return computer.ErrPointer
		}
		if user {
			return computer.UserPointerWorker(cmd.Context(), os.Stdin, os.Stdout)
		}
		return computer.AgentPointerWorker(cmd.Context(), os.Stdin, os.Stdout)
	}}
}
