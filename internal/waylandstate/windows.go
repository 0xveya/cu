// Package waylandstate reads public output and toplevel metadata from a Wayland compositor.
package waylandstate

/*
#cgo pkg-config: wayland-client
#include <stdlib.h>
#include "windowlist.h"
*/
import "C" //nolint:gocritic // cgo is a pseudo-package, not a duplicate import.

import (
	"errors"
	"path/filepath"
	"unsafe" //nolint:gocritic // required to pass the C string to free.
)

var (
	ErrConnect     = errors.New("read Wayland window list")
	ErrUnavailable = errors.New("wayland window list protocol unavailable")
)

// Window contains the public app ID and title of a mapped toplevel.
type Window struct {
	AppID string `json:"app_id"`
	Title string `json:"title"`
}

// List connects only to the supplied runtime socket.
func List(runtime, display string) ([]Window, error) {
	path := C.CString(filepath.Join(runtime, display))
	defer C.free(unsafe.Pointer(path))
	var head *C.struct_cu_window
	status := C.cu_windowlist(path, &head) //nolint:gocritic // cgo call arguments are different values.
	if status == 2 {
		return nil, ErrUnavailable
	}
	if status != 0 {
		return nil, ErrConnect
	}
	defer C.cu_windowlist_free(head)
	items := make([]Window, 0)
	for current := head; current != nil; current = current.next {
		items = append(items, Window{
			AppID: C.GoString(current.app_id),
			Title: C.GoString(current.title),
		})
	}
	return items, nil
}

// Size returns the current output dimensions in physical screenshot pixels.
func Size(runtime, display string) (width, height int, err error) {
	path := C.CString(filepath.Join(runtime, display))
	defer C.free(unsafe.Pointer(path))
	var cwidth C.int
	var cheight C.int
	status := C.cu_screen_size(path, &cwidth, &cheight) //nolint:gocritic // cgo call arguments are distinct pointers.
	if status != 0 {
		return 0, 0, ErrConnect
	}
	return int(cwidth), int(cheight), nil
}
