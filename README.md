# zdx-ban — BAN-0 v0.1

BAN-0 is a native Go research runtime testing whether the same frozen transformer becomes more robust when it preserves and evaluates multiple semantic reasoning paths instead of committing immediately to one autoregressive path. It is an external orchestration layer: it does not train or modify model weights, attention, KV caches, or inference internals, and it does not implement PPS.

## Architecture

The Ollama provider streams bounded responses behind a `context.Context`-aware interface. The BAN engine generates distinct semantic hypotheses, stores them as typed nodes in a directed graph, independently scores visible dimensions, retains leading and credible minority branches, expands leaders, performs skeptic/counterfactual challenges, invokes deterministic verifiers, backtracks after failure, selects a verified state, and atomically writes a schema `0.2` JSON trace. The graph supports multiple parents, deterministic normalized fingerprints, convergence, cycle rejection, and configured depth/node bounds. Evaluation concurrency is explicitly bounded and defaults to one.

Subsystems live under `internal/model`, `internal/ban`, `internal/memory`, `internal/trace`, `internal/benchmark`, and `internal/telemetry`. Go is the orchestration/control-plane boundary; future native inference or PPS components can sit below provider/verifier/scheduler interfaces without entering BAN-0.

## Setup and use

Install/start Ollama, then edit the strict project-level [`ban.config`](ban.config)
to select the model, endpoint, generation budgets, search bounds, and timeouts.
The complete field and override contract is in
[`CONFIGURATION.md`](CONFIGURATION.md).

```sh
go build -o ./bin/ban ./cmd/ban
./bin/ban config validate
```

Run BAN, the same-model single-generation baseline, or the initial benchmark:

```sh
./bin/ban "Why is this service gradually consuming more memory?"
./bin/ban --branches 5 --retain 2 --depth 2 "problem"
./bin/ban baseline "problem"
./bin/ban benchmark --dataset datasets/initial.json
```

Complete traces are stored in `traces/<run-id>.json`. They contain configuration, model information, runtime telemetry, every node and edge, component scores, verification results, initial/final winners, recovery fields, call/token counts, latency, and result. A temporary trace is renamed only after a complete synchronized write.

Every run also writes an observational `traces/<run-id>.candidates.jsonl`, one `internal/training.Candidate` record per graph node (`internal/cognitive.CandidatesFromBANTrace`). The `ban experiment run`/`smoke`/`memory-run`/`memory-smoke` harness records the same way into `<output>/<experiment-id>/experiment.candidates.jsonl`, since its cases run through the same `Runner.runPair`. This never affects search or selection and never auto-promotes anything: a node is classified `W1Candidate`/`W1_ELIGIBLE` only when the run's own deterministic verifier recorded an authoritative `SUPPORTED` measurement for it — the default no-constraint accept verifier leaves every node `MemoryOnly`/`RECORDED`. It is a recording of what happened, not a judgment that anything should be trained on. `./bin/ban training w1-dataset [-out DIR] <candidates.jsonl...>` extracts the `W1Candidate` subset of one or more such files into a `w1-training.jsonl` for `training/slow_android`'s release contract; see that directory's README.

W2 ("adaptive") eligibility is a stricter, repetition-gated tier above W1, not a second training stage: `./bin/ban training w2-promote [-threshold 3] [-out DIR] <candidates.jsonl...>` (`internal/training.W2PromotionCandidates`) groups `W1Candidate` records by input and, for every input independently confirmed `SUPPORTED` across at least `-threshold` distinct runs, synthesizes one derived, traceable `W2Candidate`/`W2_ELIGIBLE` record (`PromotedFrom` lists the underlying candidate IDs). It never mutates the underlying observations and never promotes anything automatically — same as W1, eligibility is not activation. No W2 trainer, release contract, or activation path exists yet.

## Memory-grounded intelligence

See [MEMORY.md](MEMORY.md) for tiered memory and epistemic safeguards, and [EXPERIMENTS.md](EXPERIMENTS.md) for Pass 5 isolated conditions, durable raw JSONL, report regeneration, inspection, and comparisons.

## What BAN Means by Correctness

See [MEASUREMENT.md](MEASUREMENT.md) for measurement-relative correctness, evidence authority and independence, the measurement hierarchy, candidate-vs-final semantics, and V0 local verification cost.

## Experimental validation

See [EXPERIMENTS.md](EXPERIMENTS.md) for the strict objective paired benchmark, dry-run, resume, reporting, telemetry, and interpretation workflow.

## Development

```sh
go test ./...
go vet ./...
go test -race ./...
go build ./...
```

Tests use provider mocks and `httptest.Server`; Ollama is not required. Generated text is untrusted, strictly decoded, size-bounded, and never executed as shell input.

Optional W1 LoRA training infrastructure lives in
[`training/slow_android`](training/slow_android/README.md). It never mutates W0,
does not train W2, and cannot register or activate an adapter. The mechanism
has been run for real end to end against a tiny synthetic fixture (see
DEVELOPMENT_STATUS.md); no real, license-reviewed corpus or production
adapter is claimed.

The W0 raw-document corpus contract and streaming validator live in
[`training/w0`](training/w0/README.md), together with a pilot
tokenizer/release builder and resumable full-weight trainer. The mechanism
has likewise been run for real end to end against a tiny synthetic fixture
(see DEVELOPMENT_STATUS.md); no trained or accepted production W0 is claimed.
Ubuntu hosts can supervise the existing W1 worker with the disabled-by-default,
cron-safe launcher in
[`training/slow_ubuntu`](training/slow_ubuntu/README.md).

## Scientific discipline and limitations

Baseline and BAN use the same configured model and relevant generation settings. The bundled dataset is a harness seed, not evidence that BAN improves accuracy. Current verification defaults to an explicit no-constraint accept verifier unless a deterministic domain verifier is injected. Semantic deduplication is normalized-text-only; telemetry uses portable Go runtime data; token counts depend on Ollama; no learned verifier, GUI, database, native foundation-model training, KV-cache manipulation, or PPS exists yet. The W0 corpus tools validate data and, as of this pass, have trained real (if toy) weights on a synthetic fixture; the W0 and W1 mechanisms have both been run for real end to end on this device (see DEVELOPMENT_STATUS.md) — but only against explicitly-synthetic fixtures, never a real corpus, so no validated production checkpoint or adapter is claimed.

The next experiment should introduce a fixed, objectively verifiable task set and compare paired seeded baseline/BAN runs, emphasizing forced recovery cases, final-selection accuracy, latency, tokens, and memory.
