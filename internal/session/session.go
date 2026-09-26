// Package session manages headless Labwc desktops in the user's runtime directory.
package session

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const display = "wayland-0"

var (
	ErrID       = errors.New("invalid session ID")
	ErrExists   = errors.New("session already exists")
	ErrNotFound = errors.New("session not found")
	ErrStopped  = errors.New("session is not running")
)

// Info is the persistent identity of a Labwc process and its Wayland socket.
type Info struct {
	ID        string `json:"id"`
	PID       int    `json:"pid"`
	StartTime uint64 `json:"start_time"`
	Display   string `json:"display"`
	Runtime   string `json:"runtime_dir"`
	Running   bool   `json:"running"`
	Visible   bool   `json:"visible"`
}

// Start creates a desktop. Visible sessions open a window on the host Wayland display.
func Start(id string, visible bool) (Info, error) {
	dir, err := sessionDir(id)
	if err != nil {
		return Info{}, err
	}
	backend := "headless"
	parentDisplay := ""
	if visible {
		parentDisplay, err = hostDisplay()
		if err != nil {
			return Info{}, err
		}
		backend = "wayland"
	}
	err = os.MkdirAll(filepath.Dir(dir), 0o700)
	if err != nil {
		return Info{}, fmt.Errorf("create session root: %w", err)
	}
	err = os.Mkdir(dir, 0o700)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return Info{}, ErrExists
		}
		return Info{}, fmt.Errorf("create session %s: %w", id, err)
	}
	ready := false
	defer func() {
		if !ready {
			_ = os.RemoveAll(dir)
		}
	}()
	runtimeDir := filepath.Join(dir, "runtime")
	configDir := filepath.Join(dir, "config")
	for _, path := range []string{runtimeDir, configDir} {
		err = os.Mkdir(path, 0o700)
		if err != nil {
			return Info{}, fmt.Errorf("create %s: %w", path, err)
		}
	}
	unit := Unit(id)
	// #nosec G204 -- all variable arguments are paths beneath the validated session directory.
	command := exec.CommandContext(context.Background(), "systemd-run", "--user", "--collect",
		"--unit="+unit, "--slice="+Slice(id), "--service-type=exec",
		"--setenv=XDG_RUNTIME_DIR="+runtimeDir, "--setenv=WAYLAND_DISPLAY="+parentDisplay,
		"--setenv=WLR_BACKENDS="+backend, "--setenv=WLR_LIBINPUT_NO_DEVICES=1",
		"labwc", "-C", configDir)
	output, err := command.CombinedOutput()
	if err != nil {
		return Info{}, fmt.Errorf("start Labwc: %w: %s", err, strings.TrimSpace(string(output)))
	}
	defer func() {
		if !ready {
			_ = stopUnit(unit)
		}
	}()

	info := Info{ID: id, Display: display, Runtime: runtimeDir, Running: true, Visible: visible}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		info.StartTime = 0
		info.PID, err = unitPID(unit)
		if err == nil && info.PID > 0 {
			info.StartTime, err = processStartTime(info.PID)
		}
		if err == nil && info.StartTime > 0 && socketReady(info) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if info.StartTime == 0 || !socketReady(info) {
		return Info{}, fmt.Errorf("labwc did not create its socket; inspect journalctl --user -u %s", unit)
	}
	data, err := json.Marshal(info)
	if err != nil {
		return Info{}, fmt.Errorf("encode session: %w", err)
	}
	err = os.WriteFile(filepath.Join(dir, "state.json"), data, 0o600)
	if err != nil {
		return Info{}, fmt.Errorf("write session: %w", err)
	}
	ready = true
	return info, nil
}

func hostDisplay() (string, error) {
	name := os.Getenv("WAYLAND_DISPLAY")
	if name == "" {
		return "", errors.New("WAYLAND_DISPLAY is unset; start --visible from your host Wayland desktop")
	}
	if !filepath.IsAbs(name) {
		name = filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), name)
	}
	// #nosec G703 -- this is the host display path explicitly supplied by the caller's environment.
	info, err := os.Stat(name)
	if err != nil {
		return "", fmt.Errorf("host Wayland display: %w", err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return "", errors.New("host Wayland display is not a socket")
	}
	return name, nil
}

// Status reports the stored session and checks its process and socket.
func Status(id string) (Info, error) {
	dir, err := sessionDir(id)
	if err != nil {
		return Info{}, err
	}
	// #nosec G304 -- dir is beneath XDG_RUNTIME_DIR/cu and id is a validated name.
	data, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Info{}, ErrNotFound
		}
		return Info{}, fmt.Errorf("read session %s: %w", id, err)
	}
	var info Info
	err = json.Unmarshal(data, &info)
	if err != nil {
		return Info{}, fmt.Errorf("decode session %s: %w", id, err)
	}
	if info.ID != id || info.Runtime != filepath.Join(dir, "runtime") || info.Display != display {
		return Info{}, fmt.Errorf("session %s has invalid state", id)
	}
	startTime, err := processStartTime(info.PID)
	info.Running = err == nil && startTime == info.StartTime && socketReady(info)
	return info, nil
}

// Active returns a session only while its original process and socket exist.
func Active(id string) (Info, error) {
	info, err := Status(id)
	if err != nil {
		return Info{}, err
	}
	if !info.Running {
		return Info{}, ErrStopped
	}
	return info, nil
}

// Stop terminates Labwc and removes its runtime files.
func Stop(id string) error {
	info, err := Status(id)
	if err != nil {
		return err
	}
	if info.Running {
		err = stopUnit(Unit(id))
		if err != nil {
			return err
		}
		deadline := time.Now().Add(3 * time.Second)
		var startTime uint64
		for time.Now().Before(deadline) {
			startTime, err = processStartTime(info.PID)
			if err != nil || startTime != info.StartTime || !socketReady(info) {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if socketReady(info) {
			return fmt.Errorf("labwc did not stop; session files remain at %s", filepath.Dir(info.Runtime))
		}
	}
	err = stopUnit(Slice(id))
	if err != nil {
		return err
	}
	err = os.RemoveAll(filepath.Dir(info.Runtime))
	if err != nil {
		return fmt.Errorf("remove session %s: %w", id, err)
	}
	return nil
}

// Unit is the systemd service name for a session's compositor.
func Unit(id string) string {
	return "cu_" + hex.EncodeToString([]byte(id)) + "_labwc.service"
}

// RecordUnit is the transient service that records one session.
func RecordUnit(id string) string {
	return "cu_" + hex.EncodeToString([]byte(id)) + "_record.service"
}

// Slice groups the compositor and launched applications for one session.
func Slice(id string) string {
	return "cu_" + hex.EncodeToString([]byte(id)) + ".slice"
}

// StartApp starts argv as a user service in the session's systemd slice.
func StartApp(info Info, argv []string) (int, error) {
	unit := "cu_" + hex.EncodeToString([]byte(info.ID)) + "_app_" + strconv.FormatInt(time.Now().UnixNano(), 10) + ".service"
	args := make([]string, 0, 7+len(argv))
	args = append(args,
		"--user", "--collect", "--unit="+unit, "--slice="+Slice(info.ID), "--service-type=exec",
		"--setenv=XDG_RUNTIME_DIR="+info.Runtime, "--setenv=WAYLAND_DISPLAY="+info.Display,
	)
	args = append(args, argv...)
	// #nosec G204 -- argv is explicitly supplied by the cu launch caller.
	command := exec.CommandContext(context.Background(), "systemd-run", args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("launch app: %w: %s", err, strings.TrimSpace(string(output)))
	}
	pid, err := unitPID(unit)
	if err != nil {
		return 0, fmt.Errorf("get app PID: %w", err)
	}
	if pid == 0 {
		return 0, fmt.Errorf("app exited during launch; inspect journalctl --user -u %s", unit)
	}
	return pid, nil
}

func unitPID(unit string) (int, error) {
	// #nosec G204 -- unit names are derived from validated session IDs.
	command := exec.CommandContext(context.Background(), "systemctl", "--user", "show", unit, "--property=MainPID", "--value")
	output, err := command.Output()
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(output)))
}

func stopUnit(unit string) error {
	// #nosec G204 -- unit names are derived from validated session IDs.
	command := exec.CommandContext(context.Background(), "systemctl", "--user", "stop", unit)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("stop %s: %w: %s", unit, err, strings.TrimSpace(string(output)))
	}
	return nil
}

// Environment selects the session socket without inheriting the host display.
func Environment(runtimeDir, displayName string) []string {
	result := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "WAYLAND_DISPLAY=") || strings.HasPrefix(entry, "XDG_RUNTIME_DIR=") ||
			strings.HasPrefix(entry, "WLR_BACKENDS=") || strings.HasPrefix(entry, "WLR_LIBINPUT_NO_DEVICES=") {
			continue
		}
		result = append(result, entry)
	}
	result = append(result, "XDG_RUNTIME_DIR="+runtimeDir)
	if displayName != "" {
		result = append(result, "WAYLAND_DISPLAY="+displayName)
	}
	return result
}

func sessionDir(id string) (string, error) {
	if id == "" || len(id) > 64 {
		return "", ErrID
	}
	for _, char := range id {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-' || char == '_' {
			continue
		}
		return "", ErrID
	}
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		return "", errors.New("XDG_RUNTIME_DIR is unset")
	}
	return filepath.Join(runtimeDir, "cu", id), nil
}

func processStartTime(pid int) (uint64, error) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, err
	}
	closing := strings.LastIndexByte(string(data), ')')
	if closing < 0 {
		return 0, errors.New("invalid process stat")
	}
	fields := strings.Fields(string(data[closing+1:]))
	if len(fields) < 20 {
		return 0, errors.New("short process stat")
	}
	return strconv.ParseUint(fields[19], 10, 64)
}

func socketReady(info Info) bool {
	dialer := net.Dialer{Timeout: 200 * time.Millisecond}
	connection, err := dialer.DialContext(context.Background(), "unix", filepath.Join(info.Runtime, info.Display))
	if err != nil {
		return false
	}
	return connection.Close() == nil
}
