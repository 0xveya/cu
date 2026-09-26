# cu

`cu` is a Go CLI for a headless Labwc desktop. Each session has its own Wayland
socket under `$XDG_RUNTIME_DIR/cu/<id>/`. The user systemd manager keeps Labwc
and launched apps alive between CLI calls.

The first version supports desktop lifecycle, command execution, app launch,
PNG screenshots, text input, and named key presses. It requires Linux with a
running user systemd manager, `labwc`, `grim`, and `wtype` on `PATH`. The demo
also uses `foot`.

## Try it

```sh
mise run demo
```

The demo starts a desktop, opens Foot, types `echo hello-from-cu`, saves a
screenshot to `/tmp/cu-demo.png`, and stops the desktop. Set `CU_DEMO_OUT` to
choose another screenshot path.

## Commands

```sh
mise run build

bin/cu start --id agent-1
bin/cu status --id agent-1
bin/cu launch --id agent-1 -- foot
bin/cu run --id agent-1 -- env
bin/cu type --id agent-1 --text 'hello'
bin/cu key --id agent-1 Return
bin/cu key --id agent-1 ctrl+l
bin/cu screenshot --id agent-1 --out /tmp/agent-1.png
bin/cu stop --id agent-1
```

`run` waits for the command and returns its exit status. `launch` returns after
the program starts. The original direct wrapper still works with `cu run
--display NAME -- COMMAND` for an existing socket in the caller's
`XDG_RUNTIME_DIR`.

Session commands return JSON. `type` and `key` return no output on success.
If Labwc fails to start, `cu` prints the exact `journalctl` command for its
service. Screenshots and input always require a managed session ID, so they
cannot target the host display by accident.

## Development

```sh
mise run fmt:go
mise run lint:go
mise run test:go
mise run build
```

Mouse input, window listing, observation, and MCP adapters are not implemented
yet.
