# cu

`cu` is a Go CLI for an isolated Labwc desktop. Each session has its own
Wayland socket under `$XDG_RUNTIME_DIR/cu/<id>/`. The user systemd manager keeps
Labwc and launched apps alive between CLI calls.

It supports desktop lifecycle, command execution, app launch, inline MCP
images, text input, named key presses, pointer paths, clicks, drags, and scrolling.
It requires Linux with a running user systemd manager, `labwc`, `grim`, and
`wtype` on `PATH`. Recording also requires `ffmpeg`.

Building requires the `wayland-client` development package and `pkg-config`.

## Try it

Build with `mise run build`, then give your agent the
[horse demo prompt](examples/horse/PROMPT.md). It asks the agent to use `cu`
to draw a horse with the pen tool and colored cells in LibreOffice Calc,
inspecting the desktop after significant actions. The workbook, screenshots,
and recordings are in `examples/horse/`.

## Commands

```sh
mise run build

bin/cu start --id agent-1 --visible
bin/cu status --id agent-1
bin/cu launch --id agent-1 -- foot
bin/cu run --id agent-1 -- env
bin/cu type --id agent-1 --text 'hello'
bin/cu key --id agent-1 Return
bin/cu key --id agent-1 ctrl+l
bin/cu move --id agent-1 --x 500 --y 400
bin/cu click --id agent-1 --x 500 --y 400
bin/cu click --id agent-1 --x 500 --y 400 --button right
bin/cu drag --id agent-1 --from 500,200 --to 800,400
bin/cu scroll --id agent-1 --x 500 --y 400 --delta-y 120
bin/cu view --id agent-1
bin/cu windows --id agent-1
bin/cu screenshot --id agent-1 --out /tmp/agent-1.png
bin/cu record start --id agent-1 --out /tmp/agent-1.mp4
bin/cu record status --id agent-1
bin/cu record stop --id agent-1
bin/cu stop --id agent-1
```

`run` waits for the command and returns its exit status. `launch` returns after
the program starts. The original direct wrapper still works with `cu run
--display NAME -- COMMAND` for an existing socket in the caller's
`XDG_RUNTIME_DIR`.

Session commands return JSON. `windows` returns a compact JSON array of visible
app IDs and titles, so an agent can check which apps are open before requesting
a screenshot. Input commands return no output on success.
Recording captures only the named Labwc desktop, including its apps, and
returns the MP4 path as JSON. It keeps recording while other `cu` commands run.
Recording playback defaults to 4× real time. Use `--speed 1` for real time;
videos faster than real time show their speed in the top-right corner.
Visible sessions record at their current desktop size; headless recordings
default to 1920×1080. `record status` reports the output dimensions.
For a browser that fills the recording, focus it and use `cu key --id agent-1
F11` before recording.
If Labwc fails to start, `cu` prints the exact `journalctl` command for its
service. Screenshots and input always require a managed session ID, so they
cannot target the host display by accident.

`start --visible` opens the nested Labwc desktop as a window on your current
Wayland desktop. It returns immediately, so you can keep watching the window
while another terminal or agent uses `cu launch`, `cu type`, and the other
commands with that ID. Run `cu stop --id agent-1` to close it. Omit `--visible`
to start a headless session instead; visibility is chosen when the session
starts.

## Development

```sh
mise run fmt:go
mise run lint:go
mise run test:go
mise run build
```

## Codex computer use

```sh
mise run build
codex mcp add cu -- /absolute/path/to/cu/bin/cu mcp
mkdir -p ~/.agents/skills
ln -s /absolute/path/to/cu/skills/cu-computer-use ~/.agents/skills/cu-computer-use
```

Restart Codex to discover the tools and skill. The default stdio server is
launched by the client. For stateless Streamable HTTP instead, keep this command
running and register its URL:

```sh
bin/cu mcp --listen 127.0.0.1:7331
codex mcp add cu --url http://127.0.0.1:7331/mcp
```

The official MCP Go SDK handles protocol `2026-07-28`, including handshake-free
requests with per-request metadata, and older clients. HTTP is loopback-only,
uses JSON responses, and keeps no MCP session IDs. Managed desktop sessions
remain independent of transport connections. Each desktop keeps one agent
pointer device alive across calls, so menu interactions do not recreate
the input device at every click.

`desktop_session` manages desktops, `desktop_launch` opens apps,
`desktop_observe` returns window titles and an original-size inline PNG,
`desktop_act` runs up to 64 ordered input actions and returns one inline image,
and `desktop_record` manages MP4 capture. No screenshot file or separate image
loading call is needed. `windows_only: true` skips the image in an observation;
`observe: false` skips it after input. Input batches stop at their first error
and report completed actions, so observe before retrying a partial batch.

For freehand drawing, a `path` action holds the left button along up to 8192
`points: [{"x": 100, "y": 200}, ...]` in one pointer connection. Set
`duration_ms` from 0 to 5000 to control pacing. Coordinates remain unscaled
screenshot pixels. Calls always name an isolated session; independent agents
should use distinct IDs and never issue concurrent input on the same desktop.

`bin/cu view` lists sessions in a separate Chromium app window and shows the
selected desktop, including headless sessions. It uses the Go standard library
and an installed browser. `--no-open` prints a local URL instead. The viewer
shows only a session picker, an Attach control, and the desktop.
Attach forwards your mouse and keyboard to the selected session; your normal
cursor stays separate from the pink agent marker. Frames refresh sequentially
and pause when the window is hidden. The temporary Chromium profile is removed
when the window closes. Without Chromium, it opens the default browser;
stop the CLI with Ctrl-C when finished.

Recordings add a visible agent cursor and click ring, including motion through
paths and drags. For smooth real-time playback, use `--fps 12 --speed 1`.

Desktop isolation selects a different Wayland socket. Applications still run
as your user with access to your files and network.
