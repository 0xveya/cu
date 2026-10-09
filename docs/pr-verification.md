# Change verification

Use this checklist before proposing a change. This does not create or post a PR.

1. Reproduce the reported behavior with a focused test. Run it and retain its failing output before editing implementation.
2. Make the smallest fix, run the focused test, then `go test ./...` and `go vet ./...` in a disposable environment without credentials.
3. Build `go build -o cu ./cmd/cu`. Exercise the built binary on an isolated Linux Wayland desktop through MCP; verify real screenshots, not only returned PIDs.
4. For recording changes, exercise speed 1 and an accelerated recording. Inspect with `ffprobe -v error -show_streams -show_format demo.mp4` and decode every frame with `ffmpeg -v error -xerror -i demo.mp4 -f null -`.
5. Check H.264, yuv420p, even dimensions, constant frame pacing at no more than 30 FPS, and a `moov` atom before `mdat`. Confirm accelerated output visibly says `4x speed` (or the selected multiplier). Replay in a browser; successful FFmpeg decoding alone does not prove browser playback.
6. Review the diff for unrelated edits and secrets. Include test commands, actual results, screenshots/video paths, and unresolved limitations in the proposed review description.
7. Commit and push only the requested branch. Verify the remote commit hash after pushing. Post a PR only when explicitly requested.

## Recording tradeoffs

Recording uses real captured frames with frame duplication/drop to maintain 30 FPS output; it does not synthesize movement or interpolate screenshots. Higher capture FPS improves cursor sampling but increases CPU usage. Screenshot capture time counts toward the capture period instead of being added to it. Accelerated input rates can exceed browser decoder limits, so the encoded output is capped independently of capture FPS and playback speed.
