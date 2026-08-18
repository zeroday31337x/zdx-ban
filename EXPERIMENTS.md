# BAN Experimental Validation Pass 2

The paired experiment compares one normal generation with the unchanged BAN-0 search using the same frozen provider, prompt, temperature, seed, per-call generation limit, timeout policy, and host. Expected answers are never included in model prompts. Primary accuracy includes only cases accepted by a registered deterministic verifier.

## Datasets

- `datasets/ban-experiment-001-smoke.jsonl`: 20 cases, four per category.
- `datasets/ban-experiment-001.jsonl`: 100 cases, twenty per category.

Both are generated from inspectable deterministic definitions in `cmd/datasetgen/main.go`, use independent dataset versions, and are SHA-256 hashed when loaded. Categories are arithmetic/constraints, logic/deduction, structured transformation, coding/debugging, and forced recovery.

## Memory ablation commands

```sh
./bin/ban experiment memory-smoke --dry-run
./bin/ban experiment memory-smoke --repetitions 1 --live
./bin/ban experiment memory-run --repetitions 3 --live
./bin/ban experiment memory-report results/<id>/memory-results.jsonl results/<id>/pass5-report.md
./bin/ban experiment memory-inspect results/<id>/memory-results.jsonl <case-or-run-id>
./bin/ban experiment memory-compare results/<id>/memory-results.jsonl BAN_COLD BAN_MEMORY
./bin/ban memory inspect
./bin/ban memory stats
./bin/ban memory reset --confirm
```

See [MEMORY.md](MEMORY.md) for intervention semantics and leakage controls.

## Pass 5 empirical memory validation

Pass 5 emits experiment schema `0.4` while retaining BAN trace schema `0.3` and memory schema `0.1`. It persists one raw JSONL observation per experiment/run/condition/case/attempt, from which reports and comparisons are regenerated.

Stores are isolated by condition and repetition. Live execution requires `--live`; tests use deterministic fake providers. Memory effects derive from observable retrieval, measurement, recovery, and condition deltas, never private chain-of-thought. `UNKNOWN`, `UNSUPPORTED`, `INCONCLUSIVE`, `CONTRADICTED`, `FAILED`, and `SUPPORTED` remain distinct.

The 20/100-case datasets cover useful transfer, irrelevant rejection, stale override, misleading recovery, novelty preservation, repeated-problem efficiency, failure-memory leverage, and conflicting observations. Expected answers remain verifier-only.

Better memory performance does not prove that remembered information is true. It demonstrates that historical information improved measured behavior under the recorded experimental contract.

A harmful-memory recovery result is valuable. BAN should be judged not only by whether it retrieves useful information, but by whether it can escape incorrect historical assumptions when current evidence disagrees.

## Commands

```sh
./bin/ban experiment smoke --dry-run
./bin/ban experiment smoke --repetitions 1 --output results
./bin/ban experiment run --dataset datasets/ban-experiment-001.jsonl --repetitions 3
./bin/ban experiment run --resume BAN-EXPERIMENT-001-...
./bin/ban experiment report results/<experiment-id>/summary.json
```

Dry-run validates schema, IDs, duplicate prompts, objective verifier support, configuration, and output location without contacting Ollama. Completed pairs are atomically checkpointed; resume skips them and rejects dataset or configuration drift. Ctrl-C cancels active requests, writes a valid checkpoint, and permits later resume.

Each result directory contains `manifest.json`, `paired-results.jsonl`, `summary.json`, and `report.md`. The manifest records the dataset hash/version, Git commit or `unavailable`, model/provider, fixed BAN configuration, temperature, seed, token limit, timeout, Go/OS/architecture, and timestamps.

## Verification and interpretation

Pass 3 emits experiment schema 0.3 and BAN trace schema 0.3; memory schema is 0.1. Pass 2 schema 0.1 files remain ordinary JSON and are not reinterpreted. Current objective verifiers are normalized exact output, numeric output with tolerance, explicit safe numeric constraints, and structured JSON field invariants. There is no permissive fallback in experimental mode. Outcomes distinguish incorrect answers, malformed output, unsupported/misconfigured verification, provider errors, timeouts, and interruption.

“Recovery rate” is successful BAN final verification among cases where BAN’s initial top branch fails the objective verifier. Baseline has no structurally comparable initial branch, so the report does not fabricate a conditional baseline recovery probability. McNemar’s test compares final paired binary correctness only.

Telemetry samples Go runtime memory immediately before and after each side. “Peak” is the larger endpoint sample, not a continuously sampled true peak; this avoids materially perturbing inference. Token fields are omitted when Ollama supplies no token counts.

The architecture-default comparison intentionally permits BAN’s additional calls. It measures both verified accuracy and cost; it is not an equal-token-budget experiment. No claim of improvement is valid until live paired results support it.
