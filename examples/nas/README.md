# NAS smoke test

These are real captures from a headless Labwc session on the NAS, not rendered
mockups. `desktop.png` is the native 1280×720 screenshot; `desktop.mp4` is the
1920×1080 H.264 recording, 12 fps, normal playback speed, 3.5 seconds.

Verified on October 9, 2026:

- Linux/systemd user manager, Labwc 0.8.3 (Debian 13), grim, wtype, foot.
- Built in a disposable uncredentialed Go 1.27.1 container, not the Git devbox.
- `go test ./...`, `go vet ./...`, native build and installer tests passed.
- Installer deployed the native binary; `--hermes` registered stdio through
  the official CLI. `hermes mcp test cu` discovered all five tools.
- CLI created a headless desktop, launched foot, typed text, moved/clicked the
  pointer, captured PNG, recorded MP4, finalized recording and stopped desktop.
- A separate real MCP stdio client initialized the server, listed all five
  tools, started another headless desktop, launched foot, sent a three-action
  input batch and received a validated inline PNG. Stopping that desktop did
  not stop the first desktop.
- ffprobe confirmed H.264, 1920×1080, yuv420p, 12 fps; FFmpeg decoded the full
  MP4 without errors. Both test desktops were stopped; no cu units remain.

Build-from-source requires Go and Wayland development headers. The NAS itself
used the installer's `--binary` path after the isolated build. This does not
claim the source-build one-liner was executed on a clean NAS.

The test used the existing root Hermes account. Desktop socket separation is
not a filesystem/network sandbox; prefer an unprivileged service account for
production use with untrusted applications.
