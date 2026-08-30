#!/usr/bin/env bash
set -euo pipefail

HERE="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
ENV_FILE="${1:-$HERE/training.env}"
# shellcheck source=env.sh
source "$HERE/env.sh"
zdx_load_training_env "$ENV_FILE"
zdx_require_enabled_training
: "${ZDX_SLOW_OUTPUT:?set ZDX_SLOW_OUTPUT}"
mkdir -p -- "$ZDX_SLOW_OUTPUT"
rm -f -- "$ZDX_SLOW_OUTPUT/STOP" "$ZDX_SLOW_OUTPUT/PERMANENT_FAILURE"
"$HERE/watchdog.sh" "$ENV_FILE"
