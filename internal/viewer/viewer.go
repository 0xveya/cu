// Package viewer serves a local desktop viewer with optional user input.
package viewer

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"cu/internal/computer"
	"cu/internal/session"
)

//go:embed index.html
var page string

// ErrViewer identifies a failure to serve or open the session viewer.
var ErrViewer = errors.New("session viewer")

// OpError preserves the failed viewer operation and its underlying cause.
type OpError struct {
	Operation string
	Cause     error
}

func (e *OpError) Error() string   { return e.Operation + ": session viewer: " + e.Cause.Error() }
func (e *OpError) Unwrap() []error { return []error{ErrViewer, e.Cause} }

// Run serves on loopback until cancellation or the app window closes.
func Run(ctx context.Context, id string, open bool, output io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if id != "" {
		_, err := session.Status(id)
		if err != nil {
			return err
		}
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return &OpError{Operation: "listen", Cause: err}
	}
	defer func() { _ = listener.Close() }()
	prefix := "/" + rand.Text() + "/"
	state := &viewer{ctx: ctx, controllers: make(map[string]*computer.UserController)}
	defer func() { cancel(); state.close() }()
	server := &http.Server{Handler: state.handler(prefix), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	defer func() { _ = server.Close() }()
	serveResult := make(chan error, 1)
	go func() { serveResult <- server.Serve(listener) }()
	url := "http://" + listener.Addr().String() + prefix
	if id != "" {
		url += "#" + id
	}
	_, err = io.WriteString(output, url+"\n")
	if err != nil {
		return &OpError{Operation: "write URL", Cause: err}
	}
	var closed <-chan error
	var cleanup func()
	if open {
		closed, cleanup, err = openWindow(ctx, url)
		if err != nil {
			return err
		}
		defer cleanup()
	}
	select {
	case <-ctx.Done():
		return nil
	case err = <-closed:
		if err != nil && ctx.Err() == nil {
			return &OpError{Operation: "window exited", Cause: err}
		}
		return nil
	case err = <-serveResult:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return &OpError{Operation: "serve", Cause: err}
	}
}

type viewer struct {
	ctx         context.Context
	mutex       sync.Mutex
	controllers map[string]*computer.UserController
}

func (v *viewer) close() {
	v.mutex.Lock()
	defer v.mutex.Unlock()
	for id, controller := range v.controllers {
		_ = controller.Close()
		delete(v.controllers, id)
	}
}

func (v *viewer) handler(prefix string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+prefix, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != prefix {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src 'self' blob:")
		_, _ = io.WriteString(w, page)
	})
	mux.HandleFunc("GET "+prefix+"sessions", func(w http.ResponseWriter, _ *http.Request) {
		items, err := session.List()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(items)
	})
	mux.HandleFunc("GET "+prefix+"frame/{id}", func(w http.ResponseWriter, r *http.Request) {
		info, err := session.Active(r.PathValue("id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		frame, err := computer.CaptureWithCursor(ctx, info)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		// #nosec G705 -- the response is a captured PNG with image/png and nosniff headers.
		_, _ = w.Write(frame.PNG)
	})
	mux.HandleFunc("POST "+prefix+"input/{id}", v.input)
	mux.HandleFunc("POST "+prefix+"attach/{id}", v.attach)
	protected := http.NewCrossOriginProtection().Handler(mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		protected.ServeHTTP(w, r)
	})
}

func openWindow(ctx context.Context, url string) (closed <-chan error, cleanup func(), err error) {
	for _, name := range []string{"chromium", "chromium-browser", "google-chrome"} {
		path, lookupErr := exec.LookPath(name)
		if lookupErr != nil {
			continue
		}
		profile, profileErr := os.MkdirTemp("", "cu-view-")
		if profileErr != nil {
			return nil, nil, &OpError{Operation: "create window profile", Cause: profileErr}
		}
		// #nosec G204 -- path is a known browser and the URL is our loopback listener.
		command := exec.CommandContext(ctx, path, "--app="+url, "--user-data-dir="+profile, "--no-first-run", "--no-default-browser-check", "--disable-sync", "--window-size=1200,800", "--class=cu-view")
		err = command.Start()
		if err != nil {
			_ = os.RemoveAll(profile)
			return nil, nil, &OpError{Operation: "open window", Cause: err}
		}
		done := make(chan error, 1)
		finished := make(chan struct{})
		go func() {
			waitErr := command.Wait()
			_ = os.RemoveAll(profile)
			done <- waitErr
			close(finished)
		}()
		return done, func() {
			if command.Process != nil {
				_ = command.Process.Kill()
			}
			<-finished
		}, nil
	}
	// #nosec G204 -- the URL is generated from our loopback listener.
	command := exec.CommandContext(ctx, "xdg-open", url)
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, nil, &OpError{Operation: "open browser " + strings.TrimSpace(string(output)), Cause: err}
	}
	return nil, func() {}, nil
}

type inputRequest struct {
	Kind    string                  `json:"kind"`
	Pointer *computer.PointerAction `json:"pointer,omitempty"`
	Text    string                  `json:"text,omitempty"`
	Key     string                  `json:"key,omitempty"`
}

func (v *viewer) input(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "expected application/json", http.StatusUnsupportedMediaType)
		return
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10))
	decoder.DisallowUnknownFields()
	var request inputRequest
	err := decoder.Decode(&request)
	if err != nil {
		http.Error(w, "invalid input: "+err.Error(), http.StatusBadRequest)
		return
	}
	var extra json.RawMessage
	err = decoder.Decode(&extra)
	if !errors.Is(err, io.EOF) {
		http.Error(w, "expected one input", http.StatusBadRequest)
		return
	}
	info, err := session.Active(r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	v.mutex.Lock()
	defer v.mutex.Unlock()
	controller, found := v.controllers[info.ID]
	if !found {
		http.Error(w, "desktop is not attached", http.StatusConflict)
		return
	}
	switch request.Kind {
	case "pointer":
		if request.Pointer == nil {
			http.Error(w, "missing pointer action", http.StatusBadRequest)
			return
		}
		err = controller.Send(ctx, request.Pointer)
	case "text":
		if request.Text == "" || len(request.Text) > 16384 {
			http.Error(w, "invalid text", http.StatusBadRequest)
			return
		}
		err = computer.Type(ctx, info, request.Text)
	case "key":
		if request.Key == "" || len(request.Key) > 128 {
			http.Error(w, "invalid key", http.StatusBadRequest)
			return
		}
		err = computer.Key(ctx, info, request.Key)
	default:
		http.Error(w, "unknown input kind", http.StatusBadRequest)
		return
	}
	if err != nil {
		_ = controller.Close()
		delete(v.controllers, info.ID)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type attachRequest struct {
	Attached bool `json:"attached"`
}

func (v *viewer) attach(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "expected application/json", http.StatusUnsupportedMediaType)
		return
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	decoder.DisallowUnknownFields()
	var request attachRequest
	err := decoder.Decode(&request)
	if err != nil {
		http.Error(w, "invalid attach request", http.StatusBadRequest)
		return
	}
	var extra json.RawMessage
	err = decoder.Decode(&extra)
	if !errors.Is(err, io.EOF) {
		http.Error(w, "expected one attach request", http.StatusBadRequest)
		return
	}
	id := r.PathValue("id")
	err = nil
	v.mutex.Lock()
	defer v.mutex.Unlock()
	if !request.Attached {
		controller, found := v.controllers[id]
		if found {
			err = controller.Close()
			delete(v.controllers, id)
		}
		if err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if _, found := v.controllers[id]; found {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	info, err := session.Active(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	controller, err := computer.StartUserPointer(v.ctx, info)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	v.controllers[id] = controller
	w.WriteHeader(http.StatusNoContent)
}
