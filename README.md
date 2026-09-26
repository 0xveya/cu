# cu

`cu` is a Go CLI for an isolated Labwc desktop. Each session has its own
Wayland socket under `$XDG_RUNTIME_DIR/cu/<id>/`. The user systemd manager keeps
Labwc and launched apps alive between CLI calls.

The first version supports desktop lifecycle, command execution, app launch,
PNG screenshots, text input, named key presses, and pointer movement, clicks,
and drags. It requires Linux with a running user systemd manager, `labwc`,
`grim`, and `wtype` on `PATH`. Recording also requires `ffmpeg`. The demo
also uses `foot`.

Building requires the `wayland-client` development package and `pkg-config`.

## Try it

Build with `mise run build`, then give your agent the
[horse demo prompt](examples/horse/PROMPT.md). It asks the agent to use `cu`
to arrange browser windows and draw a horse in Calc, inspecting the desktop
after each significant action. The task needs Chromium and LibreOffice Calc
as well as the runtime dependencies above.

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

Scroll input and MCP adapters are not implemented yet.
