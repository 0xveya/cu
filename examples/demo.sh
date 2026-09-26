#!/bin/sh
set -eu

demo_id="cu-demo-$$"
demo_out="${CU_DEMO_OUT:-${TMPDIR:-/tmp}/cu-demo.png}"
cu_bin="$(dirname "$0")/../bin/cu"

"$cu_bin" start --id "$demo_id"
trap '"$cu_bin" stop --id "$demo_id" >/dev/null' EXIT

"$cu_bin" launch --id "$demo_id" -- foot --config=/dev/null --title='CU demo' /bin/sh
sleep 2
"$cu_bin" type --id "$demo_id" --text 'echo hello-from-cu'
"$cu_bin" key --id "$demo_id" Return
sleep 1
"$cu_bin" screenshot --id "$demo_id" --out "$demo_out"
