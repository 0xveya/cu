package computer

import "fmt"

func recordingEncoding() []string {
 return []string{"-c:v", "libx264", "-preset", "fast", "-crf", "18", "-pix_fmt", "yuv420p", "-movflags", "+faststart"}
}

// recordingFilter keeps screenshot coordinates undistorted in a 1080p video.
func recordingFilter(visible bool, speed int) string {
	filter := "pad=ceil(iw/2)*2:ceil(ih/2)*2"
	if !visible {
		filter = "scale=1920:1080:force_original_aspect_ratio=decrease:flags=lanczos,pad=1920:1080:(ow-iw)/2:(oh-ih)/2:color=0x101827,setsar=1"
	}
	if speed > 1 {
		filter += ",drawtext=text='" + fmt.Sprint(speed) + "x':fontcolor=white:fontsize=38:box=1:boxcolor=black@0.75:boxborderw=10:x=w-tw-24:y=24"
	}
	return filter
}
