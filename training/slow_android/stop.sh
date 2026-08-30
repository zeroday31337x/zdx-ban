#!/data/data/com.termux/files/usr/bin/bash
set -eu

: "${ZDX_SLOW_OUTPUT:?set ZDX_SLOW_OUTPUT}"
mkdir -p -- "$ZDX_SLOW_OUTPUT"
touch -- "$ZDX_SLOW_OUTPUT/STOP"
echo "stop requested; trainer will checkpoint at the next safe boundary"
