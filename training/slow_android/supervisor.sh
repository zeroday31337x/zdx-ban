#!/data/data/com.termux/files/usr/bin/bash
set -u

: "${ZDX_W1_RELEASE:?set ZDX_W1_RELEASE}"
: "${ZDX_W0_TRANSFORMERS:?set ZDX_W0_TRANSFORMERS}"
: "${ZDX_SLOW_OUTPUT:?set ZDX_SLOW_OUTPUT}"

HERE="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
PYTHON_BIN="${PYTHON_BIN:-python}"
WAIT_SECONDS="$($PYTHON_BIN -c 'import json,sys; print(int(json.load(open(sys.argv[1]))["restart_wait_seconds"]))' "$HERE/config.json")"

while true; do
  if [ -f "$ZDX_SLOW_OUTPUT/STOP" ]; then
    exit 0
  fi
  "$PYTHON_BIN" "$HERE/trainer.py" \
    --release "$ZDX_W1_RELEASE" \
    --base-model "$ZDX_W0_TRANSFORMERS" \
    --output "$ZDX_SLOW_OUTPUT" \
    --config "$HERE/config.json"
  status=$?
  if [ "$status" -eq 0 ]; then
    exit 0
  fi
  if [ "$status" -ne 75 ]; then
    exit "$status"
  fi
  sleep "$WAIT_SECONDS"
done
