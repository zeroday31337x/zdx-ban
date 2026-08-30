#!/usr/bin/env bash
set -euo pipefail

HERE="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
ENV_FILE="${1:-$HERE/training.env}"
# shellcheck source=env.sh
source "$HERE/env.sh"
zdx_load_training_env "$ENV_FILE"
: "${ZDX_SLOW_OUTPUT:?set ZDX_SLOW_OUTPUT}"
mkdir -p -- "$ZDX_SLOW_OUTPUT"
touch -- "$ZDX_SLOW_OUTPUT/STOP"
echo "stop requested; $ZDX_TRAINING_STAGE will checkpoint at its next safe boundary"
