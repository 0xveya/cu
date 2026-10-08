---
name: cu-computer-use
description: Control GUI applications in isolated Linux desktops with cu MCP, including inline observations, batched mouse and keyboard input, and recordings. Use for cu desktop sessions, not the host desktop or browser DOM automation.
---

# cu computer use

Use the connected `cu` MCP tools. Every call names its desktop explicitly; MCP connections carry no selected desktop state. Pick a distinct session ID for independent agents. Never send simultaneous input batches to the same session.

- `desktop_session` starts, lists, inspects, or stops desktops. Use `visible: true` when a nested live desktop is useful.
- `desktop_launch` takes a program and separate arguments in `argv`.
- `desktop_observe` returns window titles and an inline PNG at its original dimensions. Use its pixels as coordinates. `windows_only: true` checks app titles without an image.
- `desktop_act` runs ordered `actions` and returns one inline image by default. Kinds: `move`, `click`, `drag`, `path`, `scroll`, `type`, `key`, `wait`. Pointer coordinates are `x`, `y`; drag endpoints are `to_x`, `to_y`. A `path` holds the left mouse button through `points: [{"x": 100, "y": 200}, ...]` in one connection; `duration_ms` controls pacing (0 to 5000). Use it for detailed freehand drawing instead of many tiny drags. Scroll uses `delta_x`, `delta_y` in pixels, positive right/down. Keys use names such as `Return` or `ctrl+l`.
- `desktop_record` starts, inspects, or stops an MP4. Supply `out` for start. Recording is independent of MCP connections and includes the agent cursor.

Batch a known sequence, for example click a field, type text, then press Return. Inspect after meaningful transitions instead of requesting a screenshot after every keystroke. Use `observe: false` when the result needs no visual decision. Do not save an MCP image to disk and invoke a separate image viewer; it is already image content in the tool response. When orchestration wraps tools, forward the image content block through its image helper instead of stringifying the response or printing base64.

A batch stops on its first failed action and reports how many succeeded. Earlier input remains applied. Observe before retrying; do not replay a partly completed batch blindly. A successful input call alone does not prove the intended GUI result.

Some GTK menus suppress a quick first selection after opening. Wait 550 ms after opening such a menu, then click its item; observe rather than blindly clicking twice.

Keep sessions alive while the user is watching or intends to continue. Stop sessions you created only when finished with them. Desktop isolation selects a separate Wayland socket; launched applications still run as the local user and can access that user's files and network.

If tools are unavailable, build this repository with `mise run build`, connect `bin/cu mcp` as a stdio MCP server, and restart the client to discover its tools. For stateless HTTP, run `bin/cu mcp --listen 127.0.0.1:7331` and connect to `http://127.0.0.1:7331/mcp`. See the repository README for installation.
