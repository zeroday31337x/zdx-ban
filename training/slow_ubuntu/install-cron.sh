#!/usr/bin/env bash
set -euo pipefail

HERE="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
ENV_FILE="${1:-$HERE/training.env}"
if [[ ! -f "$ENV_FILE" ]]; then
  echo "training environment file is missing: $ENV_FILE" >&2
  exit 2
fi
ENV_FILE="$(realpath -- "$ENV_FILE")"
if ! command -v crontab >/dev/null 2>&1; then
  echo "crontab is not installed" >&2
  exit 2
fi

BEGIN_MARKER="# BEGIN ZDX TRAINING WATCHDOG"
END_MARKER="# END ZDX TRAINING WATCHDOG"
temporary="$(mktemp)"
filtered="$(mktemp)"
cleanup() {
  rm -f -- "$temporary" "$filtered"
}
trap cleanup EXIT

crontab -l >"$temporary" 2>/dev/null || true
awk -v begin="$BEGIN_MARKER" -v end="$END_MARKER" '
  $0 == begin { skipping = 1; next }
  $0 == end { skipping = 0; next }
  !skipping { print }
' "$temporary" >"$filtered"
{
  cat "$filtered"
  echo "$BEGIN_MARKER"
  printf '*/5 * * * * %q %q >>%q 2>&1\n' "$HERE/watchdog.sh" "$ENV_FILE" "$HERE/cron.log"
  echo "$END_MARKER"
} >"$temporary"
crontab "$temporary"
echo "installed five-minute ZDX watchdog for $ENV_FILE"
echo "the job remains inert while ZDX_TRAINING_ENABLED=0"
