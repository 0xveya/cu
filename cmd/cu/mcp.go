package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cu/internal/mcpserver"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

var errMCP = errors.New("serve MCP")

type mcpError struct {
	Operation string
	Cause     error
}

func (e *mcpError) Error() string   { return e.Operation + ": " + e.Cause.Error() }
func (e *mcpError) Unwrap() []error { return []error{errMCP, e.Cause} }

func mcpCommand() *cobra.Command {
	var listen string
	command := &cobra.Command{Use: "mcp", Short: "Serve computer use tools over stdio or stateless HTTP", Args: cobra.NoArgs}
	command.Flags().StringVar(&listen, "listen", "", "optional loopback HTTP address, e.g. 127.0.0.1:7331")
	command.RunE = func(cmd *cobra.Command, _ []string) error {
		ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		server := mcpserver.New(ctx)
		if listen == "" {
			return server.Run(ctx, &mcp.StdioTransport{})
		}
		host, _, err := net.SplitHostPort(listen)
		if err != nil {
			return &mcpError{Operation: "parse listen address", Cause: err}
		}
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return &mcpserver.ArgumentError{Field: "listen must use a loopback IP address"}
		}
		listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", listen)
		if err != nil {
			return &mcpError{Operation: "listen", Cause: err}
		}
		mux := http.NewServeMux()
		mux.Handle("/mcp", mcpserver.Handler(server))
		httpServer := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
		done := make(chan error, 1)
		go func() { done <- httpServer.Serve(listener) }()
		_, err = os.Stderr.WriteString("cu MCP: http://" + listener.Addr().String() + "/mcp\n")
		if err != nil {
			_ = httpServer.Close()
			return &mcpError{Operation: "report MCP address", Cause: err}
		}
		select {
		case err = <-done:
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err = httpServer.Shutdown(shutdownCtx)
			if err != nil {
				_ = httpServer.Close()
			}
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return &mcpError{Operation: "serve HTTP", Cause: err}
		}
		return nil
	}
	return command
}
