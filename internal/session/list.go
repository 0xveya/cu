package session

import (
	"errors"
	"os"
	"path/filepath"
)

// ErrList identifies a failure to enumerate managed desktops.
var ErrList = errors.New("list sessions")

// ListError preserves the failed path and underlying session error.
type ListError struct {
	Path  string
	Cause error
}

func (e *ListError) Error() string   { return "list sessions: " + e.Path + ": " + e.Cause.Error() }
func (e *ListError) Unwrap() []error { return []error{ErrList, e.Cause} }

// List returns managed desktops in ID order, including stopped sessions.
func List() ([]Info, error) {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		return nil, &ListError{Cause: errors.New("XDG_RUNTIME_DIR is unset")}
	}
	root := filepath.Join(runtimeDir, "cu")
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return []Info{}, nil
	}
	if err != nil {
		return nil, &ListError{Path: root, Cause: err}
	}
	items := make([]Info, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, statusErr := Status(entry.Name())
		if errors.Is(statusErr, ErrID) || errors.Is(statusErr, ErrNotFound) {
			continue
		}
		if statusErr != nil {
			return nil, &ListError{Path: entry.Name(), Cause: statusErr}
		}
		items = append(items, info)
	}
	return items, nil
}
