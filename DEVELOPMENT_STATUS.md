# Development status

## Training-candidate recording from live runs

Every `ban run` now writes an observational `traces/<run-id>.candidates.jsonl`
alongside its trace (`internal/cognitive.CandidatesFromBANTrace`, wired from
`cmd/ban/main.go`). Previously only the `ban runtime smoke` demo path (a fake
VM executor) produced `internal/training.Candidate` records, and the real
search engine's output never reached the training-candidate model at all.

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
Nothing here trains or promotes a W1 release automatically, and `W2` remains
unimplemented beyond its type-level placeholders in `internal/modelstate` and
`internal/training`.

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
