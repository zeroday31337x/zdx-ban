#!/usr/bin/env bash
set -euo pipefail

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
crontab "$filtered"
echo "removed ZDX training watchdog cron block"
