package computer

import (
	"strings"
	"testing"
)

func TestRecordingEncodingUsesHighQualityH264(t *testing.T) {
 args := strings.Join(recordingEncoding(), " ")
 if !strings.Contains(args, "-preset fast -crf 18") { t.Fatalf("missing quality settings: %s", args) }
 if !strings.Contains(args, "-movflags +faststart") { t.Fatal("video must support progressive playback") }
}

func TestRecordingFilterPreservesHeadlessAspectRatio(t *testing.T) {
	filter := recordingFilter(false, 1)
	for _, want := range []string{"force_original_aspect_ratio=decrease", "pad=1920:1080:(ow-iw)/2:(oh-ih)/2", "flags=lanczos"} {
		if !strings.Contains(filter, want) {
			t.Fatalf("filter %q missing %q", filter, want)
		}
	}
	if strings.Contains(filter, "drawtext") {
		t.Fatal("real-time video must not have a speed badge")
	}
}
