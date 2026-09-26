package computer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"cu/internal/session"
	"cu/internal/waylandstate"
)

var (
	ErrRecording        = errors.New("recording is already running")
	ErrRecordNotFound   = errors.New("no recording for this session")
	ErrRecordOutput     = errors.New("recording output already exists")
	ErrRecordIncomplete = errors.New("recording did not produce a video")
)

// Recording is the state of one session's current or last video recording.
type Recording struct {
	Path    string `json:"path"`
	FPS     int    `json:"fps"`
	Speed   int    `json:"speed"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Running bool   `json:"running"`
}

// RecordStart starts capturing a managed desktop to a new MP4 file.
func RecordStart(info session.Info, out string, fps, speed int) (Recording, error) {
	if fps < 1 || fps > 30 {
		return Recording{}, errors.New("recording FPS must be between 1 and 30")
	}
	if speed < 1 || speed > 16 {
		return Recording{}, errors.New("recording speed must be between 1 and 16")
	}
	if _, err := exec.LookPath("grim"); err != nil {
		return Recording{}, fmt.Errorf("find grim: %w", err)
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return Recording{}, fmt.Errorf("find ffmpeg: %w", err)
	}
	current, err := RecordStatus(info)
	if err == nil && current.Running {
		return Recording{}, ErrRecording
	}
	if err != nil && !errors.Is(err, ErrRecordNotFound) {
		return Recording{}, err
	}
	path, err := filepath.Abs(out)
	if err != nil {
		return Recording{}, fmt.Errorf("resolve recording path: %w", err)
	}
	_, err = os.Stat(path)
	if err == nil {
		return Recording{}, ErrRecordOutput
	}
	if !errors.Is(err, os.ErrNotExist) {
		return Recording{}, fmt.Errorf("check recording path: %w", err)
	}
	self, err := os.Executable()
	if err != nil {
		return Recording{}, fmt.Errorf("find cu executable: %w", err)
	}
	width, height := 1920, 1080
	if info.Visible {
		width, height, err = waylandstate.Size(info.Runtime, info.Display)
		if err != nil {
			return Recording{}, err
		}
		width += width % 2
		height += height % 2
	}
	recording := Recording{Path: path, FPS: fps, Speed: speed, Width: width, Height: height, Running: true}
	data, err := json.Marshal(recording)
	if err != nil {
		return Recording{}, fmt.Errorf("encode recording: %w", err)
	}
	statePath := recordingPath(info)
	err = os.WriteFile(statePath, data, 0o600)
	if err != nil {
		return Recording{}, fmt.Errorf("write recording state: %w", err)
	}
	// #nosec G204 -- self is the current cu executable; path and FPS are explicit CLI arguments.
	command := exec.CommandContext(context.Background(), "systemd-run", "--user", "--collect",
		"--unit="+session.RecordUnit(info.ID), "--slice="+session.Slice(info.ID), "--service-type=exec",
		self, "_record", "--id", info.ID, "--out", path, "--fps", fmt.Sprint(fps), "--speed", fmt.Sprint(speed))
	output, err := command.CombinedOutput()
	if err != nil {
		_ = os.Remove(statePath)
		return Recording{}, fmt.Errorf("start recording: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return recording, nil
}

// RecordStatus reports the current or last recording for a session.
func RecordStatus(info session.Info) (Recording, error) {
	data, err := os.ReadFile(recordingPath(info))
	if errors.Is(err, os.ErrNotExist) {
		return Recording{}, ErrRecordNotFound
	}
	if err != nil {
		return Recording{}, fmt.Errorf("read recording state: %w", err)
	}
	var recording Recording
	err = json.Unmarshal(data, &recording)
	if err != nil {
		return Recording{}, fmt.Errorf("decode recording state: %w", err)
	}
	running, err := recordUnitRunning(info.ID)
	if err != nil {
		return Recording{}, err
	}
	recording.Running = running
	return recording, nil
}

// RecordStop asks the recorder to finish its video and waits for its output.
func RecordStop(info session.Info) (Recording, error) {
	recording, err := RecordStatus(info)
	if err != nil {
		return Recording{}, err
	}
	if recording.Running {
		unit := session.RecordUnit(info.ID)
		// #nosec G204 -- unit is derived from a validated session ID.
		command := exec.CommandContext(context.Background(), "systemctl", "--user", "kill", "--signal=INT", "--kill-whom=main", unit)
		var output []byte
		output, err = command.CombinedOutput()
		if err != nil {
			return Recording{}, fmt.Errorf("stop recording: %w: %s", err, strings.TrimSpace(string(output)))
		}
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			var running bool
			running, err = recordUnitRunning(info.ID)
			if err != nil {
				return Recording{}, err
			}
			if !running {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	recording, err = RecordStatus(info)
	if err != nil {
		return Recording{}, err
	}
	if recording.Running {
		return Recording{}, errors.New("recording did not stop within 10 seconds")
	}
	file, err := os.Stat(recording.Path)
	if err != nil || file.Size() == 0 {
		return Recording{}, ErrRecordIncomplete
	}
	return recording, nil
}

// RecordWorker captures the desktop and closes FFmpeg cleanly on INT or TERM.
func RecordWorker(id, out string, fps, speed int) error {
	info, err := session.Active(id)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	filter := "pad=ceil(iw/2)*2:ceil(ih/2)*2"
	if !info.Visible {
		filter = "scale=1920:1080:flags=bilinear"
	}
	if speed > 1 {
		filter += ",drawtext=text='" + fmt.Sprint(speed) + "x':fontcolor=white:fontsize=38:box=1:boxcolor=black@0.75:boxborderw=10:x=w-tw-24:y=24"
	}
	// #nosec G204 -- out and fps come from the validated recording state for this managed session.
	encoder := exec.CommandContext(context.WithoutCancel(ctx), "ffmpeg", "-hide_banner", "-loglevel", "error", "-n",
		"-f", "image2pipe", "-framerate", fmt.Sprint(fps*speed), "-vcodec", "ppm", "-i", "pipe:0",
		"-vf", filter, "-c:v", "libx264", "-preset", "ultrafast",
		"-pix_fmt", "yuv420p", "-movflags", "+faststart", "-f", "mp4", out)
	input, err := encoder.StdinPipe()
	if err != nil {
		return fmt.Errorf("open encoder input: %w", err)
	}
	encoder.Stderr = os.Stderr
	if err := encoder.Start(); err != nil {
		return fmt.Errorf("start encoder: %w", err)
	}
	interval := time.Second / time.Duration(fps)
	frames := make(chan []byte, 1)
	captureErrors := make(chan error, 1)
	go captureFrames(ctx, info, interval, frames, captureErrors)
	var latest []byte
	select {
	case latest = <-frames:
	case captureErr := <-captureErrors:
		_ = input.Close()
		_ = encoder.Wait()
		return captureErr
	case <-ctx.Done():
		_ = input.Close()
		_ = encoder.Wait()
		return ctx.Err()
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var captureErr error
capture:
	for {
		select {
		case <-ctx.Done():
			break capture
		case latest = <-frames:
		case captureErr = <-captureErrors:
			break capture
		case <-ticker.C:
			if _, err := input.Write(latest); err != nil {
				captureErr = fmt.Errorf("send recording frame: %w", err)
				break capture
			}
		}
	}
	if err := input.Close(); err != nil && captureErr == nil {
		captureErr = fmt.Errorf("close encoder input: %w", err)
	}
	if err := encoder.Wait(); err != nil && captureErr == nil {
		captureErr = fmt.Errorf("finish recording: %w", err)
	}
	return captureErr
}

func captureFrames(ctx context.Context, info session.Info, interval time.Duration, frames chan []byte, failures chan error) {
	for ctx.Err() == nil {
		frame := exec.CommandContext(ctx, "grim", "-t", "ppm", "-")
		frame.Env = session.Environment(info.Runtime, info.Display)
		pixels, err := frame.Output()
		if err != nil {
			if ctx.Err() == nil {
				failures <- fmt.Errorf("capture recording frame: %w", err)
			}
			return
		}
		select {
		case frames <- pixels:
		default:
			select {
			case <-frames:
			default:
			}
			select {
			case frames <- pixels:
			case <-ctx.Done():
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

func recordingPath(info session.Info) string {
	return filepath.Join(filepath.Dir(info.Runtime), "recording.json")
}

func recordUnitRunning(id string) (bool, error) {
	// #nosec G204 -- the unit is derived from a validated session ID.
	command := exec.CommandContext(context.Background(), "systemctl", "--user", "show", session.RecordUnit(id), "--property=ActiveState", "--value")
	output, err := command.Output()
	if err != nil {
		return false, fmt.Errorf("check recording service: %w", err)
	}
	return strings.TrimSpace(string(output)) == "active", nil
}
