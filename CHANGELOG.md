# Changelog

## 0.2.0 — 2026-11-05

- W0 base switched from Qwen2.5-1.5B to official Qwen/Qwen3-0.6B (post-trained, Apache 2.0). `ban.config` model and foundation_id and `Modelfile` FROM now read `qwen3:0.6b`. The Transformers safetensors on the VPS are the weight-editing source of truth; the Q4 GGUF runs under Ollama.
- Added the W1/W2 design docs: `W2-DAILY-DESIGN.md` and `WEIGHTS-AND-MEMORY-DESIGN.md`.
- Added the October 25 memory-experiment evidence under `results/BAN-EXPERIMENT-001-20261025-*` and the trace files under `traces/`.
- Added the from-scratch W0 config at `training/w0/w0-375m-config.json`. The from-scratch path is superseded by the open-weights base and kept as history.
- Rollback: this release is one commit on main. To restore the previous base, revert this commit, or run `git checkout cc39ad1 -- Modelfile ban.config`. The pre-switch backup files live on the VPS only and are not committed.
