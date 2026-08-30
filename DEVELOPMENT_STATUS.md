# Development status

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
