#!/usr/bin/env bash
set -uo pipefail

HERE="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
ENV_FILE="${1:-$HERE/training.env}"

# shellcheck source=env.sh
source "$HERE/env.sh"
zdx_load_training_env "$ENV_FILE" || exit $?
if [[ "$ZDX_TRAINING_ENABLED" != "1" ]]; then
  exit 0
fi
zdx_require_enabled_training || exit $?

mkdir -p -- "$ZDX_SLOW_OUTPUT"
PID_FILE="$ZDX_SLOW_OUTPUT/runner.pid"
cleanup() {
  if [[ -f "$PID_FILE" ]] && [[ "$(<"$PID_FILE")" == "$$" ]]; then
    rm -f -- "$PID_FILE"
  fi
}
trap cleanup EXIT INT TERM

if [[ -e "$ZDX_SLOW_OUTPUT/STOP" ]]; then
  exit 0
fi

case "$ZDX_TRAINING_STAGE" in
  W0)
    "$ZDX_PYTHON" "$ZDX_PROJECT_ROOT/training/w0/trainer.py" \
      --release "$ZDX_W0_RELEASE" \
      --output "$ZDX_SLOW_OUTPUT"
    ;;
  W1)
    "$ZDX_PYTHON" "$ZDX_PROJECT_ROOT/training/slow_android/trainer.py" \
      --release "$ZDX_W1_RELEASE" \
      --base-model "$ZDX_W0_TRANSFORMERS" \
      --output "$ZDX_SLOW_OUTPUT" \
      --config "$ZDX_W1_CONFIG"
    ;;
esac
status=$?

printf '%s\n' "$status" >"$ZDX_SLOW_OUTPUT/last-exit-code"
date -u +'%Y-%m-%dT%H:%M:%SZ' >"$ZDX_SLOW_OUTPUT/last-exit-at"
if [[ "$status" -eq 2 ]]; then
  touch -- "$ZDX_SLOW_OUTPUT/PERMANENT_FAILURE"
fi
exit "$status"
