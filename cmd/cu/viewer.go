package main

import (
	"os"
	"os/signal"
	"syscall"

	"cu/internal/viewer"

	"github.com/spf13/cobra"
)

func viewerCommand() *cobra.Command {
	var id string
	var noOpen bool
	command := &cobra.Command{Use: "view", Short: "Watch managed desktops in a separate window", Args: cobra.NoArgs}
	command.Flags().StringVar(&id, "id", "", "initial session ID")
	command.Flags().BoolVar(&noOpen, "no-open", false, "print the local viewer URL without opening a window")
	command.RunE = func(cmd *cobra.Command, _ []string) error {
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return viewer.Run(ctx, id, !noOpen, cmd.OutOrStdout())
	}
	return command
}
