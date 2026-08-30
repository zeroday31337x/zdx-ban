#!/usr/bin/env bash

zdx_load_training_env() {
  local env_file=$1
  if [[ ! -f "$env_file" ]]; then
    echo "training environment file is missing: $env_file" >&2
    return 2
  fi
  # The environment file is operator-controlled shell syntax. Never point this
  # at an untrusted or writable-by-others file.
  set -a
  # shellcheck disable=SC1090
  source "$env_file"
  set +a
  : "${ZDX_TRAINING_ENABLED:=0}"
  : "${ZDX_TRAINING_STAGE:=W1}"
}

zdx_require_enabled_training() {
  if [[ "$ZDX_TRAINING_ENABLED" != "1" ]]; then
    return 3
  fi
  : "${ZDX_PROJECT_ROOT:?set ZDX_PROJECT_ROOT}"
  : "${ZDX_PYTHON:?set ZDX_PYTHON}"
  : "${ZDX_SLOW_OUTPUT:?set ZDX_SLOW_OUTPUT}"
  if [[ ! -x "$ZDX_PYTHON" ]]; then
    echo "configured Python is not executable: $ZDX_PYTHON" >&2
    return 2
  fi
  case "$ZDX_TRAINING_STAGE" in
    W0)
      : "${ZDX_W0_RELEASE:?set ZDX_W0_RELEASE}"
      if [[ ! -f "$ZDX_PROJECT_ROOT/training/w0/trainer.py" ]]; then
        echo "W0 trainer not found below ZDX_PROJECT_ROOT" >&2
        return 2
      fi
      ;;
    W1)
      : "${ZDX_W1_RELEASE:?set ZDX_W1_RELEASE}"
      : "${ZDX_W0_TRANSFORMERS:?set ZDX_W0_TRANSFORMERS}"
      : "${ZDX_W1_CONFIG:=$ZDX_PROJECT_ROOT/training/slow_android/config.json}"
      if [[ ! -f "$ZDX_PROJECT_ROOT/training/slow_android/trainer.py" ]]; then
        echo "W1 trainer not found below ZDX_PROJECT_ROOT" >&2
        return 2
      fi
      ;;
    *)
      echo "unsupported training stage: $ZDX_TRAINING_STAGE (expected W0 or W1)" >&2
      return 2
      ;;
  esac
}
