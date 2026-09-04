# Validation TODO

The W1 worker is not production-validated until all of the following are
recorded against an immutable W0 and reviewed W1 release. Several are now
DONE for the *mechanism*, against a tiny synthetic fixture, not a real
corpus/release — see DEVELOPMENT_STATUS.md ("First real device-specific W1
LoRA training run") for exact results and caveats:

- DONE (mechanism only): Run preflight on the target device with the real
  local W0 checkpoint.
- DONE (mechanism only, 4-example/4-optimizer-step, not literally one):
  Complete a one-example, one-optimizer-step smoke.
- DONE (mechanism only): Interrupt after a checkpoint and prove deterministic
  resume behavior — proven bit-identical for resume-from-checkpoint;
  independent *fresh* runs are NOT reproducible against each other because
  `training/slow_android`'s config has no `seed` field (unlike `training/w0`'s)
  and LoRA init is unseeded — a real, newly-discovered gap.
- DONE (mechanism only, startup-precondition variant): Force the low-memory
  path and prove exit 75. NOT done: safe supervisor restart (needs
  `training/slow_ubuntu`'s watchdog, not exercised).
- NOT done: request a graceful stop with pending gradients and verify the
  checkpoint (the tested config's `gradient_accumulation_steps: 1` means
  there is never a partial-accumulation window; a plain STOP-with-no-pending-
  gradients was proven for W0 but not re-tested for W1).
- DONE (mechanism only, missing/corrupted-checkpoint case): Reject truncated,
  malformed, cross-release, and path-escaping state files.
- DONE (mechanism only): Verify W0 files are byte-identical before and after
  training.
- DONE (mechanism only): Load the final LoRA against the bound W0 and run
  compatibility inference — real `PeftModel.from_pretrained` + `generate()`
  completed without error; the generated text has no capability claim
  attached (24k-parameter toy model, 4 training examples).
- NOT done: run deterministic task, safety, regression, and
  execution-failure gates (no such gate/harness exists yet regardless of
  hardware).
- NOT done: reject an unvalidated adapter at registration (no registration
  mechanism exists yet).
- Partially measured: wall-clock (~5.5s for the tiny smoke), disk (~1.2MB for
  the whole fixture run). RAM/swap pressure and thermals were not
  systematically measured; this was a sub-second-per-step toy run, not
  representative of real training load.

W2 eligibility semantics are now decided and implemented (see
DEVELOPMENT_STATUS.md: a repetition-gated promotion tier over W1 candidates,
`internal/training.W2PromotionCandidates` / `training w2-promote`). Actual W2
training, a W2 release contract, validation, rollback, and activation remain
separate, unimplemented work — eligibility is not an artifact.

## W0 prerequisites

- Select a parameter budget that fits measured training compute; do not infer a
  1B-1.5B full-pretraining capability from streamed dataset loading.
- Build, review, and freeze a provenance-preserving corpus release using the
  `training/w0` contract. `training/w0/validate_dataset.py` now gates
  near-duplicate text (opt-in `--near-duplicate`), contamination against a
  real held-out benchmark corpus (opt-in `--benchmark-corpus`, distinguished
  in the report from ordinary in-corpus near-duplicates), high-precision
  PII/secrets (on by default), an operator-supplied license allow-list
  (opt-in `--license-allow`), per-source/per-source-type caps (opt-in
  `--max-records-per-source[-type]`), a coarse language/script structural
  check (opt-in `--language-script-check`), and degenerate-text quality
  heuristics (opt-in `--max-token-repetition-ratio`/`--min-alpha-ratio`) —
  all unit-tested but not yet run against a real corpus. Safety review has no
  gate at all (a keyword denylist would be false confidence, not protection)
  and still needs a real classifier or human review pipeline. Real factual
  quality, topic classification, and human audits of random/high-risk samples
  also remain separate, unimplemented pipeline stages.
- Run the tokenizer builder against a representative approved corpus and audit
  vocabulary coverage, special-token IDs, normalization, and artifact hashes.
- Review the pilot architecture and training policy against measured hardware;
  create a new immutable release rather than editing an existing release.
- Complete a real W0 preflight and tiny-corpus overfit using the atomic model,
  optimizer, scheduler, RNG, release, tokenizer, and exact cursor checkpoints.
  DONE for the mechanism, not the corpus: run against an 8-record,
  explicitly-`synthetic: true` fixture (not a real, license-reviewed corpus —
  this session could not source or fabricate one) with a 2-layer/hidden-32
  toy architecture. Real preflight, tokenizer build, release binding, and 2
  real optimizer steps all completed on this host in under 5s. See
  DEVELOPMENT_STATUS.md for exact results.
- Complete tiny-model overfit, interrupted-resume, checkpoint-corruption,
  deterministic replay, loss-curve, held-out perplexity, safety, and downstream
  evaluation gates before scaling. Interrupted-resume, checkpoint-corruption
  rejection, and deterministic replay are DONE for the same tiny fixture —
  a resumed run reproduced bit-for-bit identical final weights versus an
  uninterrupted run, and a resume pointed at a missing checkpoint was
  correctly rejected. Tiny-model overfit (driving loss to near-zero on the
  fixture), loss-curve/held-out perplexity (no `validation` split exists in
  the fixture), safety, and downstream evaluation gates remain undone.
- Measure realistic accelerator hours, RAM/VRAM, storage, checkpoint bandwidth,
  and total token throughput before committing to the final corpus size.

## Ubuntu watchdog validation

- Populate `training/slow_ubuntu/training.env` with a dedicated virtualenv,
  select W0 or W1, and use immutable real release paths while disabled.
- Run W1 preflight, then verify one enabled foreground/watchdog start.
- Kill the runner after a checkpoint and verify cron resumes from that exact
  optimizer step without duplicating a runner.
- Force exit 75 and prove retry; force exit 2 and prove the permanent-failure
  latch prevents a restart loop.
- Verify STOP, resume, final completion, stale PID, corrupt state, log rotation,
  host reboot, and cron-daemon startup behavior on the intended Ubuntu VPS.
