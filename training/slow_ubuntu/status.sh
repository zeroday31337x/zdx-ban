#!/usr/bin/env bash
set -euo pipefail

HERE="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
ENV_FILE="${1:-$HERE/training.env}"
# shellcheck source=env.sh
source "$HERE/env.sh"
zdx_load_training_env "$ENV_FILE"

if [[ -z "${ZDX_SLOW_OUTPUT:-}" ]]; then
  echo "ZDX_SLOW_OUTPUT is not configured" >&2
  exit 2
fi

echo "enabled=$ZDX_TRAINING_ENABLED stage=$ZDX_TRAINING_STAGE output=$ZDX_SLOW_OUTPUT"
if [[ -f "$ZDX_SLOW_OUTPUT/runner.pid" ]]; then
  runner_pid="$(<"$ZDX_SLOW_OUTPUT/runner.pid")"
  if [[ "$runner_pid" =~ ^[1-9][0-9]*$ ]] && kill -0 "$runner_pid" 2>/dev/null; then
    echo "runner=RUNNING pid=$runner_pid"
  else
    echo "runner=STALE_PID pid=$runner_pid"
  fi
else
  echo "runner=NOT_RUNNING"
fi
[[ -e "$ZDX_SLOW_OUTPUT/STOP" ]] && echo "stop_requested=true" || echo "stop_requested=false"
[[ -e "$ZDX_SLOW_OUTPUT/PERMANENT_FAILURE" ]] && echo "restart_latched=true" || echo "restart_latched=false"
if [[ -f "$ZDX_SLOW_OUTPUT/last-exit-code" ]]; then
  echo "last_exit_code=$(<"$ZDX_SLOW_OUTPUT/last-exit-code")"
fi
if [[ -f "$ZDX_SLOW_OUTPUT/last-exit-at" ]]; then
  echo "last_exit_at=$(<"$ZDX_SLOW_OUTPUT/last-exit-at")"
fi
if [[ -f "$ZDX_SLOW_OUTPUT/state.json" ]]; then
  "$ZDX_PYTHON" "$ZDX_PROJECT_ROOT/training/slow_android/status.py" "$ZDX_SLOW_OUTPUT"
else
  echo '{"status":"NOT_STARTED"}'
fi
