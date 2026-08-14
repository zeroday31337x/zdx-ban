# zdx-ban — BAN-0 v0.1

BAN-0 is a native Go research runtime testing whether the same frozen transformer becomes more robust when it preserves and evaluates multiple semantic reasoning paths instead of committing immediately to one autoregressive path. It is an external orchestration layer: it does not train or modify model weights, attention, KV caches, or inference internals, and it does not implement PPS.

## Architecture

The Ollama provider streams bounded responses behind a `context.Context`-aware interface. The BAN engine generates distinct semantic hypotheses, stores them as typed nodes in a directed graph, independently scores visible dimensions, retains leading and credible minority branches, expands leaders, performs skeptic/counterfactual challenges, invokes deterministic verifiers, backtracks after failure, selects a verified state, and atomically writes a schema `0.1` JSON trace. The graph supports multiple parents, deterministic normalized fingerprints, convergence, cycle rejection, and configured depth/node bounds. Evaluation concurrency is explicitly bounded and defaults to one.

Subsystems live under `internal/model`, `internal/ban`, `internal/memory`, `internal/trace`, `internal/benchmark`, and `internal/telemetry`. Go is the orchestration/control-plane boundary; future native inference or PPS components can sit below provider/verifier/scheduler interfaces without entering BAN-0.

## Setup and use

Install/start Ollama and select a model:

```sh
cp .env.example .env
export BAN_MODEL=qwen2.5:1.5b
export OLLAMA_BASE_URL=http://127.0.0.1:11434
go build -o ./bin/ban ./cmd/ban
```

Run BAN, the same-model single-generation baseline, or the initial benchmark:

```sh
./bin/ban "Why is this service gradually consuming more memory?"
./bin/ban --branches 5 --retain 2 --depth 2 "problem"
./bin/ban baseline "problem"
./bin/ban benchmark --dataset datasets/initial.json
```

Complete traces are stored in `traces/<run-id>.json`. They contain configuration, model information, runtime telemetry, every node and edge, component scores, verification results, initial/final winners, recovery fields, call/token counts, latency, and result. A temporary trace is renamed only after a complete synchronized write.

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

## Scientific discipline and limitations

Baseline and BAN use the same configured model and relevant generation settings. The bundled dataset is a harness seed, not evidence that BAN improves accuracy. Current verification defaults to an explicit no-constraint accept verifier unless a deterministic domain verifier is injected. Semantic deduplication is normalized-text-only; telemetry uses portable Go runtime data; token counts depend on Ollama; no long-term memory, embeddings, learned verifier, GUI, database, model training, KV-cache manipulation, or PPS exists yet.

The next experiment should introduce a fixed, objectively verifiable task set and compare paired seeded baseline/BAN runs, emphasizing forced recovery cases, final-selection accuracy, latency, tokens, and memory.
