package computer

import (
	"testing"
	"time"
)

func TestCaptureDelayDoesNotAddCaptureLatency(t *testing.T) {
	if got := captureDelay(50*time.Millisecond, 20*time.Millisecond); got != 30*time.Millisecond {
		t.Fatalf("capture cadence drift: %s", got)
	}
	if got := captureDelay(50*time.Millisecond, 80*time.Millisecond); got != 0 {
		t.Fatalf("slow capture must not sleep again: %s", got)
	}
}
