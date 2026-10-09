#!/usr/bin/env bash
set -euo pipefail
usage() {
  printf '%s\n' 'Usage: bash scripts/install.sh [--binary PATH] [--hermes]' 'Build and install cu to ${CU_INSTALL_DIR:-$HOME/.local/bin}.' '--binary PATH  Install an already built, trusted native binary.' '--hermes       Register the stdio MCP server using the Hermes CLI.' 'Requires Linux, labwc, grim, wtype, ffmpeg and a running user systemd manager.'
}
binary=''
hermes=false
while (($#)); do
  case "$1" in
    --help|-h) usage; exit 0 ;;
    --binary) [[ $# -ge 2 ]] || { usage >&2; exit 2; }; binary=$2; shift 2 ;;
    --hermes) hermes=true; shift ;;
    *) usage >&2; exit 2 ;;
  esac
done
[[ $(uname -s) == Linux ]] || { printf '%s\n' 'cu requires Linux.' >&2; exit 1; }
for dependency in labwc grim wtype ffmpeg systemctl systemd-run; do
  command -v "$dependency" >/dev/null || { printf 'Missing dependency: %s (see README)\n' "$dependency" >&2; exit 1; }
done
[[ -n ${XDG_RUNTIME_DIR:-} && -d $XDG_RUNTIME_DIR ]] || { printf '%s\n' 'XDG_RUNTIME_DIR is unset or missing; log in through systemd/PAM (see README).' >&2; exit 1; }
systemctl --user show-environment >/dev/null || { printf '%s\n' 'User systemd manager unavailable; see README SSH/linger setup.' >&2; exit 1; }
if [[ -z $binary ]]; then
  for dependency in go pkg-config; do
    command -v "$dependency" >/dev/null || { printf 'Missing build dependency: %s\n' "$dependency" >&2; exit 1; }
  done
  pkg-config --exists wayland-client || { printf '%s\n' 'Install libwayland-dev before building.' >&2; exit 1; }
  root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
  (cd "$root" && go build -trimpath -o bin/cu ./cmd/cu)
  binary=$root/bin/cu
fi
[[ -f $binary && -x $binary ]] || { printf '%s\n' 'Binary must be an executable file.' >&2; exit 1; }
destination=${CU_INSTALL_DIR:-$HOME/.local/bin}
mkdir -p "$destination"
install -m 755 "$binary" "$destination/cu"
"$destination/cu" --help >/dev/null
printf 'Installed: %s/cu\n' "$destination"
if $hermes; then
  command -v hermes >/dev/null || { printf '%s\n' 'Hermes CLI is missing; binary installed but MCP not registered.' >&2; exit 1; }
  hermes mcp add cu --command "$destination/cu" --args mcp
  hermes mcp test cu
fi
printf '%s\n' 'Use a new Hermes session (or /reload-mcp) after registration.' 'Ensure the install directory is on PATH. No daemon or public port is required.'
