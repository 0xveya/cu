# cu

`cu` is a Go CLI for running programs inside an isolated Wayland desktop. The
first scaffold includes the command wrapper. Session lifecycle, computer input,
screenshots, and MCP adapters are future work.

## Development

```sh
mise run fmt:go
mise run lint:go
mise run test:go
mise run build
```

## Run a command

```sh
mise run build
bin/cu run --display wayland-agent-1 -- firefox
```

The display must already exist. `cu run` executes the command directly, passes
through its standard input and output, and returns its exit status. It never
uses the host's `WAYLAND_DISPLAY` implicitly.
