# cu

Computer use for Linux agents: a separate Labwc desktop with screenshots,
mouse and keyboard input, and MP4 recording. Runs headlessly or in a window
on a Wayland desktop. Supports Hermes and other MCP clients.

## Requirements

- Linux with a running systemd user manager.
- `labwc`, `grim`, `wtype`, and `ffmpeg` (all required by the installer).
- Go 1.26.5+, a C compiler, Wayland development headers, and `pkg-config`
  to build from source.
- Hermes CLI for `--hermes` installation.

On Debian / Ubuntu, if your release provides these packages:

```sh
sudo apt-get update
sudo apt-get install -y labwc grim wtype ffmpeg libwayland-dev pkg-config build-essential git
```

Install Go separately, or run `mise install` in the cloned repository.
Install the GUI apps you want to use; the example below needs `foot`.

## Install

```sh
git clone git@github.com:0xveya/cu.git && cd cu && bash scripts/install.sh --hermes
```

The clone command requires GitHub SSH access. The installer builds
`~/.local/bin/cu`, registers it with Hermes, and tests the MCP connection.
It checks dependencies but does not install system packages.
Omit `--hermes` for CLI-only installation. Add `~/.local/bin` to your `PATH`
if needed.

Start a new Hermes session or run `/reload-mcp`, then ask Hermes to start a
desktop and launch an app. Use a different desktop ID for each agent.
See the [computer-use skill](skills/cu-computer-use/SKILL.md) for workflow guidance.

## Use

```sh
cu start --id demo
cu launch --id demo -- foot
cu type --id demo --text 'hello'
cu key --id demo Return
cu screenshot --id demo --out desktop.png
cu record start --id demo --out desktop.mp4 --fps 12 --speed 1
# Interact with the desktop while recording.
cu record stop --id demo
cu stop --id demo
```

Sessions are headless by default. Use `cu start --id demo --visible` for a
window on your Wayland desktop. `cu view` lets you watch and control sessions.
See `cu --help` and each command's `--help` for options.

Recordings include an agent cursor and click rings. Headless video fits into
1080p without stretching; screenshots and input use native desktop pixels.
Stop recording before stopping the desktop to finalize the MP4.

### Headless hosts

A monitor is not required, but a systemd user manager is. Check it with:

```sh
systemctl --user show-environment
```

For unattended use, enable lingering to keep user services running after logout:

```sh
sudo loginctl enable-linger "$(id -un)"
```

Non-login services need `XDG_RUNTIME_DIR=/run/user/UID` and
`DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/UID/bus`. Replace `UID` with the
account's numeric UID; the user manager must already be running. Run Hermes
and cu as the same user. A plain Docker container without user systemd is not
sufficient.

## Other MCP clients

Run `cu mcp` as a stdio server. For example, with Codex:

```sh
codex mcp add cu -- "$HOME/.local/bin/cu" mcp
```

Tools: `desktop_session`, `desktop_launch`, `desktop_observe`, `desktop_act`,
and `desktop_record`. Observations return inline images; actions can be batched.
Desktop sessions persist across MCP connections.

## Security

A separate desktop is **not a sandbox**. Apps can access the user's files and
network. Use an unprivileged account and an appropriate sandbox for untrusted
apps. The optional HTTP MCP transport binds to loopback.

## Development

```sh
mise install
mise run build
mise run test:go
mise run lint:go
```

[Example screenshot](examples/nas/desktop.png) ·
[Example recording](examples/nas/desktop.mp4) ·
[Drawing demo](examples/horse/PROMPT.md)
