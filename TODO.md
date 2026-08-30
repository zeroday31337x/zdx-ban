# Validation TODO

The W1 worker is not production-validated until all of the following are
recorded against an immutable W0 and reviewed W1 release:

- Run preflight on the target device with the real local W0 checkpoint.
- Complete a one-example, one-optimizer-step smoke.
- Interrupt after a checkpoint and prove deterministic resume behavior.
- Force the low-memory path and prove exit 75 plus safe supervisor restart.
- Request a graceful stop with pending gradients and verify the checkpoint.
- Reject truncated, malformed, cross-release, and path-escaping state files.
- Verify W0 files are byte-identical before and after training.
- Load the final LoRA against the bound W0 and run compatibility inference.
- Run deterministic task, safety, regression, and execution-failure gates.
- Reject an unvalidated adapter at registration; accept only after every gate.
- Measure disk growth, RAM/swap pressure, thermals, and wall-clock time on the
  intended idle device.

W2 design, training, validation, rollback, and activation remain separate work.

## W0 prerequisites

- Select a parameter budget that fits measured training compute; do not infer a
  1B-1.5B full-pretraining capability from streamed dataset loading.
- Build, review, and freeze a provenance-preserving corpus release using the
  `training/w0` contract; add near-duplicate, PII/secret, license, safety,
  quality, and benchmark-contamination gates beyond the structural validator.
- Run the tokenizer builder against a representative approved corpus and audit
  vocabulary coverage, special-token IDs, normalization, and artifact hashes.
- Review the pilot architecture and training policy against measured hardware;
  create a new immutable release rather than editing an existing release.
- Complete a real W0 preflight and tiny-corpus overfit using the atomic model,
  optimizer, scheduler, RNG, release, tokenizer, and exact cursor checkpoints.
- Complete tiny-model overfit, interrupted-resume, checkpoint-corruption,
  deterministic replay, loss-curve, held-out perplexity, safety, and downstream
  evaluation gates before scaling.
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
