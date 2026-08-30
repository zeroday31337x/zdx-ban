#!/data/data/com.termux/files/usr/bin/bash
set -eu

: "${ZDX_W1_RELEASE:?set ZDX_W1_RELEASE}"
: "${ZDX_W0_TRANSFORMERS:?set ZDX_W0_TRANSFORMERS}"
: "${ZDX_SLOW_OUTPUT:?set ZDX_SLOW_OUTPUT}"

HERE="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
mkdir -p -- "$ZDX_SLOW_OUTPUT"

PID_FILE="$ZDX_SLOW_OUTPUT/supervisor.pid"
if [ -f "$PID_FILE" ]; then
  old_pid="$(cat "$PID_FILE" 2>/dev/null || true)"
  if [ -n "$old_pid" ] && kill -0 "$old_pid" 2>/dev/null; then
    echo "slow Android trainer already running pid=$old_pid" >&2
    exit 1
  fi
fi

rm -f -- "$ZDX_SLOW_OUTPUT/STOP"
nohup "$HERE/supervisor.sh" >>"$ZDX_SLOW_OUTPUT/training.log" 2>&1 &
trainer_pid=$!
temporary_pid="$PID_FILE.tmp"
printf '%s\n' "$trainer_pid" >"$temporary_pid"
mv -f -- "$temporary_pid" "$PID_FILE"
echo "slow Android trainer started pid=$trainer_pid"
