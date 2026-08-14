# What BAN Means by Correctness

BAN does not treat a checker returning true as universal truth. A correctness statement means that a specific claim was supported under a recorded measurement contract: an observable was measured by a stated method, compared with a stated criterion and tolerance, and classified with visible authority, independence, repeatability, provenance, uncertainty, and scope.

The authoritative outcomes are `SUPPORTED`, `CONTRADICTED`, `INCONCLUSIVE`, and `ERROR`. `UNSUPPORTED` and `NOT_MEASURED` make missing capabilities and missing observations explicit. Only `SUPPORTED` maps to the legacy compatibility field `passed=true`. Unknown, inconclusive, or failed measurement never becomes success, and a measurement subsystem error is unscored rather than counted as a reasoning error.

## Verification classes

- `BENCHMARK_VERIFIED`: agrees with a deterministic benchmark specification; this is not universal external truth.
- `FORMALLY_VERIFIED`: satisfies a deterministic formal relationship such as arithmetic, constraints, hashes, or schemas.
- `EXECUTION_VERIFIED`: confirmed by controlled compilation, tests, parsing, or invariant execution.
- `MEASUREMENT_SUPPORTED`: supported by instrumented or external observation.
- `CAUSALLY_SUPPORTED`: a controlled intervention and reversal strengthen causal attribution.
- `UNVERIFIED`: no sufficient measurement exists.

Measurement authority is recorded separately as `FORMAL`, `DETERMINISTIC_RUNTIME`, `INSTRUMENTED`, `EXPERIMENTAL`, `OBSERVATIONAL`, `MODEL_ESTIMATE`, or `UNKNOWN`. These authorities answer different questions and are not interchangeable. A model estimate cannot be labeled independent measurement.

Independence is `INDEPENDENT`, `PARTIALLY_INDEPENDENT`, `MODEL_DERIVED`, or `UNKNOWN`. The current V0 benchmark measurements are deterministic, local, independent of the evaluated model, and record zero paid inference calls. BAN never silently invokes a remote model to judge correctness.

## Measurement hierarchy

1. Level 0 — formal measurement: mathematics, declared constraints, hashes, schemas.
2. Level 1 — deterministic execution: compilers, unit tests, parsers, protocol or database invariants.
3. Level 2 — instrumented observation: memory, CPU, latency, queue depth, connections, I/O.
4. Level 3 — controlled causal experiment: intervene, observe the predicted effect, restore, and observe reversal.
5. Level 4 — probabilistic or observational evidence when stronger discrimination is unavailable.

The levels describe evidence classes, not a universal quality ranking. Formal proof differs from runtime execution; observation differs from causal evidence.

## Evidence and aggregation

Every measurement may emit explicit supporting, contradicting, or neutral evidence with source and provenance. Authoritative formal or deterministic contradiction prevents factual selection regardless of utility. Inconclusive evidence preserves the branch. Measurement errors classify the measurement subsystem, not the model.

Multiple convergent graph paths do not automatically become independent votes. Evidence carries a correlation group; correlated paths count once. Independent support can strengthen a claim, but Pass 3 deliberately avoids arbitrary averaging or invented confidence.

## Candidate versus delivered answer

Candidate and final measurements are append-only, separate events:

```text
candidate hypothesis
→ candidate measurement SUPPORTED
→ selected
→ final answer generated
→ final measurement CONTRADICTED
```

In that case BAN discovered a benchmark-supported internal branch but delivered an unsupported answer. Final benchmark-supported accuracy records failure. The trace retains both events to diagnose reasoning-versus-realization failures.

## Repeatability, tolerance, and cost

Contracts declare `DETERMINISTIC`, `REPEATABLE`, `STOCHASTIC`, or `ONE_SHOT`. Tolerance and uncertainty are present only when grounded in a real comparison or instrument; the model is never asked to invent them. Measurement records latency, heap endpoints, local measurement calls, external API calls, and paid inference calls.
