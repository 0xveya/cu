package computer

import (
	"strings"
	"testing"
)

func TestRecordingEncodingCapsAcceleratedPlaybackFrameRate(t *testing.T) {
	args := strings.Join(recordingEncoding(), " ")
	if !strings.Contains(args, "-r 30 -fps_mode cfr") {
		t.Fatalf("accelerated input up to 480 FPS must be capped for browser decoders: %s", args)
	}
}
