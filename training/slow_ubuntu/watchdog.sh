#!/usr/bin/env bash
set -euo pipefail

HERE="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
ENV_FILE="${1:-$HERE/training.env}"

# shellcheck source=env.sh
source "$HERE/env.sh"
zdx_load_training_env "$ENV_FILE"
if [[ "$ZDX_TRAINING_ENABLED" != "1" ]]; then
  exit 0
fi
zdx_require_enabled_training

mkdir -p -- "$ZDX_SLOW_OUTPUT"
exec 9>"$ZDX_SLOW_OUTPUT/watchdog.lock"
if ! flock -n 9; then
  exit 0
fi

if [[ -e "$ZDX_SLOW_OUTPUT/STOP" ]]; then
  exit 0
fi
if [[ -e "$ZDX_SLOW_OUTPUT/PERMANENT_FAILURE" ]]; then
  echo "training restart is latched after a permanent failure; inspect status and run resume.sh" >&2
  exit 0
fi

state_status="NOT_STARTED"
if [[ -f "$ZDX_SLOW_OUTPUT/state.json" ]]; then
  if ! state_status="$($ZDX_PYTHON -c 'import json,sys; value=json.load(open(sys.argv[1], encoding="utf-8")); print(value.get("status", "UNKNOWN"))' "$ZDX_SLOW_OUTPUT/state.json")"; then
    touch -- "$ZDX_SLOW_OUTPUT/PERMANENT_FAILURE"
    echo "state.json is corrupt; restart latched" >&2
    exit 2
  fi
fi
case "$state_status" in
  W0_TRAINING_COMPLETE_UNVALIDATED|TRAINING_COMPLETE_UNVALIDATED|STOPPED)
    exit 0
    ;;
esac

PID_FILE="$ZDX_SLOW_OUTPUT/runner.pid"
if [[ -f "$PID_FILE" ]]; then
  runner_pid="$(<"$PID_FILE")"
  if [[ "$runner_pid" =~ ^[1-9][0-9]*$ ]] && kill -0 "$runner_pid" 2>/dev/null; then
    command_line="$(tr '\0' ' ' <"/proc/$runner_pid/cmdline" 2>/dev/null || true)"
    if [[ "$command_line" == *"$HERE/runner.sh"* ]]; then
      exit 0
    fi
    echo "runner PID $runner_pid belongs to another process; ignoring stale PID file" >&2
  fi
  rm -f -- "$PID_FILE"
fi

nohup "$HERE/runner.sh" "$ENV_FILE" >>"$ZDX_SLOW_OUTPUT/training.log" 2>&1 </dev/null &
runner_pid=$!
temporary_pid="$PID_FILE.tmp"
printf '%s\n' "$runner_pid" >"$temporary_pid"
mv -f -- "$temporary_pid" "$PID_FILE"
echo "Ubuntu $ZDX_TRAINING_STAGE trainer started pid=$runner_pid"
