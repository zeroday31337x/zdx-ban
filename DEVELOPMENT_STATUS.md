# Development status

## First real device-specific W0 training run (tiny synthetic smoke)

This repository's stated status has always been "no successful device-specific
... training run is claimed." That changed for a narrow, honestly-scoped
case: this Ubuntu VPS matches `training/w0`'s own described target profile
(8 GiB class, `nproc`=4), so a PyTorch/transformers/safetensors/tokenizers
venv was installed (`python3 -m venv`, `pip install torch --index-url
https://download.pytorch.org/whl/cpu`, then `transformers`/`safetensors`/
`tokenizers` from PyPI) and the *real* pipeline was run end to end against an
explicitly-labeled **synthetic fixture** corpus (8 short sentences,
`synthetic: true`, `generator`/`generation_prompt_hash` set honestly
describing it as a throwaway smoke fixture — not a claim of a real,
license-reviewed corpus, which this session could not source or fabricate).

What was proven, for real, not just unit-tested:

- `validate_dataset.py` (with `--near-duplicate`, `--language-script-check`,
  the new gates from this session) accepted the real fixture corpus.
- `build_tokenizer.py` trained and froze a real byte-level BPE tokenizer.
- `prepare_release.py` bound a real immutable `w0-release.json`.
- `trainer.py --preflight` passed against real host memory/disk/package
  detection.
- A real 2-tiny-layer, hidden-size-32 model trained for 2 real optimizer
  steps (`gradient_accumulation_steps: 1`) in under 5 seconds wall-clock,
  reaching `W0_TRAINING_COMPLETE_UNVALIDATED` with a real `model.safetensors`.
- **Deterministic resume, proven bit-identical**: the run was repeated,
  interrupted by deleting checkpoint-2 and reconstructing the authoritative
  post-checkpoint-1 `state.json` (checkpoint + tree hash, exactly as
  `save_checkpoint` would have left it), then resumed. The resumed run's
  final `model.safetensors` SHA-256 was **byte-for-byte identical** to the
  uninterrupted run's.
- **Corrupted/missing-checkpoint rejection**: pointing `state.json` at a
  checkpoint directory that no longer exists correctly failed with
  `"W0 resume checkpoint is missing or outside output"` rather than silently
  proceeding.
- **Low-memory pause, exit 75**: a second release with
  `min_available_memory_mb: 999999999` correctly refused to start and
  exited with the documented `PAUSE_EXIT` code (75) — this exercised the
  startup-precondition check, not the mid-training periodic re-check (which
  would require actually exhausting real memory on a shared host, which
  this session deliberately avoided).
- **Graceful STOP**: creating an `output/STOP` file before running caused a
  clean stop at `checkpoint-00000000` with status `STOPPED` and exit 0.
- **Release immutability**: the corpus shard's SHA-256 inside
  `w0-release.json` matched the corpus file's real hash after training
  completed — nothing in the release was mutated by the trainer.

What this does **not** establish: no claim about a real corpus, real model
capability, real safety, or anything beyond "the mechanism itself works as
designed on this hardware." `training/slow_ubuntu`'s watchdog was not
exercised. The venv used for the first pass was deleted by an operator error
while cleaning up fixture artifacts; it was recreated (same recipe, plus
`peft`) for the W1 pass below and is currently at `/root/zdx-ban-w0-venv` —
kept intentionally this time.

## First real device-specific W1 LoRA training run (tiny synthetic smoke)

Same honesty constraints as the W0 pass above: an explicitly-`synthetic:
true` fixture, not a real corpus. The tiny W0 checkpoint from the pass above
was rebuilt fresh (2-layer, hidden-32 toy `LlamaForCausalLM`) to serve as the
`--base-model`, its tree hash computed via `trainer.py --hash-base-model`,
and a tiny `w1-release.json`/`w1-training.jsonl` (4 instruction/output pairs)
built and bound to that exact hash — exercising the real release-binding
contract in `training/slow_android/README.md`, not a shortcut around it.

What was proven, for real:

- Real W1 preflight passed, with `dataset_sha256`/`base_model_sha256`
  correctly reported and matching.
- A real LoRA adapter (rank 2, `q_proj`/`v_proj`, `peft`) trained for 4 real
  optimizer steps (one per training pair, `gradient_accumulation_steps: 1`)
  in ~5.5s wall-clock, reaching `TRAINING_COMPLETE_UNVALIDATED` with a real
  `adapter_model.safetensors`.
- **A real, non-obvious finding**: two independent fresh runs from the same
  release produced *different* final adapter weights. `training/slow_android`
  has no `seed` field in its config schema (unlike `training/w0`'s trainer,
  which does) and LoRA weight initialization is unseeded, so exact
  reproducibility of a *fresh* run is not currently possible — only
  checkpoint-*resumption* determinism is. This is a real, previously-unknown
  gap worth a `seed` field if bit-exact fresh-run reproducibility is wanted.
- **Deterministic resume, proven bit-identical** despite the above: since
  `PeftModel.from_pretrained` loads the checkpoint's exact saved weights on
  resume (the random-init step is bypassed entirely), and this config had no
  dropout and the trainer does no data shuffling, resuming a specific run
  from its own checkpoint-1 (after reconstructing the authoritative
  post-checkpoint-1 `state.json`, deleting checkpoints 2-4 and
  `adapter-final`) reproduced that *same run's own* final adapter SHA-256
  exactly.
- **Corrupted/missing-checkpoint rejection**: same result as W0 —
  `"resume checkpoint is missing or outside output"`.
- **Low-memory pause, exit 75**: same result as W0, via the real `train()`
  path (not the separate `--preflight` CLI mode, which by design always maps
  to exit 0/2 regardless of retryability — only the actual training-attempt
  path distinguishes retryable-vs-permanent via `PAUSE_EXIT`).
- **Real compatibility inference** (the TODO item neither pass had done
  yet): loaded the trained adapter against the bound W0 checkpoint via
  `PeftModel.from_pretrained` and ran a real `model.generate()` forward pass
  — it completed without error. The generated text itself is meaningless (a
  24k-parameter toy model trained on 4 examples has no real capability); the
  point was proving the load/compatibility mechanism works, which it did.
- **Release immutability**: both the W0 corpus shard hash and the W1
  training-data hash matched their release manifests after training.

Not exercised: `training/slow_ubuntu`'s watchdog, the STOP-with-pending-
gradients variant (this config's `gradient_accumulation_steps: 1` means
there's never a partial-accumulation window to interrupt), and anything
downstream of a trained adapter (task/safety/regression/execution-failure
gates, adapter registration/rejection) — none of that exists as runnable
infrastructure yet regardless of hardware.

## Training-candidate recording from live runs

Every `ban run` now writes an observational `traces/<run-id>.candidates.jsonl`
alongside its trace (`internal/cognitive.CandidatesFromBANTrace`, wired from
`cmd/ban/main.go`). Previously only the `ban runtime smoke` demo path (a fake
VM executor) produced `internal/training.Candidate` records, and the real
search engine's output never reached the training-candidate model at all.

`internal/experiment.Runner.runPair` — the function both `ban experiment
run`/`smoke` and `ban experiment memory-run`/`memory-smoke` funnel through —
now does the same for every pair, success or failure, and
`persist`/`persistMemory` write the flattened result to
`<output>/<experiment-id>/experiment.candidates.jsonl`. This is the more
consequential path in practice: `memory-run`/`memory-smoke` are what actually
inject a real deterministic verifier (via `BranchVerifier` /
`ban.ApplyMeasurement`) and produced every `results/*-MEMORY` directory in
this repo, so it is the path most likely to yield `W1Candidate`-eligible
records. Confirmed with a real deterministic (Formal, numeric) verifier in
`TestRunPairRecordsTrainingCandidatesForVerifiedWinner` and the extended
`TestDeterministicFourConditionRunPersistsRawRows`, both in
`internal/experiment`.

This closes the "real run -> candidate" gap. The "candidate -> trainable W1
dataset" gap is now also closed at the conversion-tool level:
`./bin/ban training w1-dataset [-out DIR] <candidates.jsonl...>`
(`internal/training.WriteW1Dataset`) extracts `{"instruction", "output"}`
pairs from one or more recorded candidate files into `w1-training.jsonl`,
filtered to `Target == W1Candidate`. It still does not write `w1-release.json`
or bind a W0 hash — a human must assemble the rest of the release and review
the dataset before training. Eligibility classification stays deliberately
conservative — a node is `W1Candidate`/`W1_ELIGIBLE` only when a real
deterministic verifier recorded an authoritative `SUPPORTED` measurement for
it; the default accept verifier leaves every node `MemoryOnly`/`RECORDED`, so
an ordinary `ban run` never produces anything `w1-dataset` will pick up.
Nothing here trains or promotes a W1 release automatically.

## W2 semantics: a repetition-gated eligibility tier, not a second training stage

W2 ("adaptive") was previously unimplemented beyond type-level placeholders
(`modelstate.Identity.Adaptive`, and `W2Candidate`/`W2Eligible`/`W2Validated`
in `internal/training`). Its design is now decided and implemented at the
eligibility level: W2 is **not** a second neural training stage, an
adapter-on-adapter merge, or anything architecturally different from W1. It
is a stricter *promotion bar* over the same underlying observations — a
candidate's input is W2-eligible once it has been independently reconfirmed
`SUPPORTED` across multiple separate runs, not just the single authoritative
verification that already makes it W1-eligible.

`internal/training.W2PromotionCandidates(candidates, threshold, now)` groups
`W1Candidate` records by `Input`, counts distinct `SourceRun`s, and for every
input meeting `threshold` synthesizes one derived `Candidate` with
`Target: W2Candidate`, `ValidationState: W2Eligible`, a real
`RepetitionCount`, and a new `PromotedFrom []string` field tracing back to
every underlying per-run candidate ID. It never mutates the original
per-run records (they remain immutable observational history) and never sets
`ValidationState: Promoted` — same "eligible, not promoted" discipline as W1.
Exposed via `./bin/ban training w2-promote [-threshold 3] [-out DIR]
<candidates.jsonl...>`; default threshold is 3, chosen as a reasonable
"more than twice is not a fluke" starting point, not an empirically
validated value.

This closes the "what does W2 mean" design gap, not the "train/register/
activate a W2 artifact" gap: there is still no W2 trainer, no W2 release
contract analogous to `training/slow_android`'s W1 contract, and no
activation path. `modelstate.Identity.Adaptive` still just carries identity
metadata for whatever eventually fills that role.

## W1 slow-training worker

The optional RAM-aware W1 LoRA worker is implemented under
`training/slow_android`. Its configuration, release validation, W0 binding,
streamed dataset loading, hashed checkpoints, stop/pause handling, and resume-state
validation have dependency-light unit coverage.

No successful device-specific PyTorch/Transformers/PEFT training run is claimed.
No produced adapter has passed the project's validation and registration gates.
W2 training and activation remain unavailable.

The Go runtime continues to use an external frozen inference provider. Merely
creating an adapter does not alter the active model state.

## W0 corpus and Ubuntu supervision

The W0 raw-document JSONL contract, machine-readable schema, streaming
structural validator, byte-level BPE tokenizer builder, immutable release
assembler, approximately 50M-parameter pilot architecture, and resumable
full-weight causal trainer are implemented under `training/w0`. W0 checkpoints
bind the corpus, tokenizer, architecture, and training policy and persist model,
optimizer, scheduler, RNG, and exact packed-corpus cursor state.

This infrastructure has dependency-light unit coverage only. No tokenizer has
been trained from a real approved corpus, no device-specific full-weight smoke
has completed, and no trained or accepted W0 weights are claimed. Structural
validation does not establish source rights, near-duplicate cleanliness,
factual quality, safety, or fitness for training.

An Ubuntu-native, disabled-by-default cron watchdog is implemented under
`training/slow_ubuntu` for isolated W0 or W1 stages. It avoids duplicate runners, respects stop
and completion state, retries transient pauses/crashes, and latches permanent
failures. No live training is enabled until an operator supplies real immutable
release paths, passes preflight, and explicitly sets `ZDX_TRAINING_ENABLED=1`.
