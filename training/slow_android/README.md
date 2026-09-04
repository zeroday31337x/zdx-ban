# Slow Android W1 trainer

Status: optional W1 training infrastructure is implemented. The mechanism has
been run end to end for real on an 8 GiB-class Ubuntu VPS — real preflight,
a real tiny LoRA adapter trained for real optimizer steps against a real
bound W0 checkpoint, bit-identical deterministic resume, correct
missing-checkpoint rejection, exit 75 on low memory, and real compatibility
inference loading the adapter against W0 — but only against an
explicitly-`synthetic: true` throwaway fixture (4 training pairs), not a
real corpus or reviewed release. No real device training, accepted adapter,
or production W1 is claimed. Project status and required validation are
tracked in [../../DEVELOPMENT_STATUS.md](../../DEVELOPMENT_STATUS.md) and
[../../TODO.md](../../TODO.md).

This worker favors bounded memory and resumability over throughput. It streams
one example at a time, trains only a small LoRA adapter, accumulates gradients,
uses gradient checkpointing, checkpoints at optimizer-step boundaries, pauses
on low memory, and resumes from `state.json`. `torch_dtype: auto` preserves the
foundation checkpoint dtype instead of expanding it to float32.

It does not make a 1.5B model fit solely by streaming examples. W0, LoRA
parameters, gradients, activations, and optimizer state still require RAM or
swap. W0 must be a local, immutable, unquantized Transformers checkpoint; an
Ollama GGUF cannot be used for PEFT training.

## Release contract

The release directory must contain:

- `w1-training.jsonl`, with non-empty `instruction` and `output` strings;
- `w1-release.json`, with `release_id`, `training_sha256`, and a W0 SHA-256.

`./bin/ban training w1-dataset [-out DIR] <candidates.jsonl...>` produces
`w1-training.jsonl` from one or more `traces/<run-id>.candidates.jsonl` files
(each written automatically by `ban run`; see
`internal/cognitive.CandidatesFromBANTrace` and the root README). It only
extracts candidates whose `Target` is `W1_CANDIDATE` — the run's own
deterministic verifier must have actually confirmed the selected reasoning
outcome; the default no-constraint accept verifier never produces one. This
tool performs no deduplication beyond exact candidate ID, no quality, PII, or
safety review, and does not write `w1-release.json` — a human must still
review the resulting dataset and assemble the rest of the release by hand.

The W0 hash may be named `w0_sha256`, `base_model_sha256`, or supplied as
`foundation.artifact_hash`. It is the deterministic tree hash printed by:

```bash
python training/slow_android/trainer.py \
  --base-model /path/to/W0-transformers \
  --hash-base-model
```

## Preflight

```bash
python training/slow_android/trainer.py \
  --release /path/to/W1-release \
  --base-model /path/to/W0-transformers \
  --output training/slow-output \
  --preflight
```

Preflight verifies release and dataset hashes, the exact W0 tree hash, required
checkpoint files and Python packages, isolated output, available memory, and
free disk. Memory and disk shortages are retryable; permanent integrity or
dependency failures are not.

## Background mode

```bash
export ZDX_W1_RELEASE=/path/to/W1-release
export ZDX_W0_TRANSFORMERS=/path/to/W0-transformers
export ZDX_SLOW_OUTPUT=$PWD/training/slow-output
training/slow_android/start-background.sh
```

Inspect with `python training/slow_android/status.py "$ZDX_SLOW_OUTPUT"` and
request a checkpointed stop with `training/slow_android/stop.sh`.

`TRAINING_COMPLETE_UNVALIDATED` is not acceptance. Registration remains blocked
until deterministic, safety, regression, compatibility, resume, and execution-
failure gates pass.

## Safety boundaries

- W0 is loaded read-only and is never modified in place.
- This worker trains W1 only; it neither trains nor activates W2.
- Resume state is bound to release ID, dataset hash, and W0 hash.
- Checkpoint and final-adapter writes use isolated temporary directories and
  record deterministic tree hashes; corrupt resume checkpoints are rejected.
- W0 is hashed again before completion so an in-run mutation cannot produce an
  apparently valid adapter.
- A failed or interrupted run must use an output directory dedicated to its
  release.
- Process completion never promotes or registers an adapter.
