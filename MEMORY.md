# BAN Pass 4 — Memory-Grounded Intelligence

Pass 4 tests a limited form of the Architecting Intelligence hypothesis: retained, provenance-aware structure and experience may improve how BAN generates, prioritizes, recovers, and avoids reasoning paths. Memory is guidance about prior cases. It is never independent evidence that the present claim is correct.

## Four memory scopes

1. **Foundation / structural memory** stores explicit stable rules and invariants as Tier 1 records. Mutation is explicit and historically traceable.
2. **Learned / domain memory** stores conservatively consolidated reusable patterns as Tier 2 records. Every record retains contributing episode IDs and measurement provenance.
3. **Experiential / episodic memory** stores actual Tier 3 run histories, including failed branches, recovery, candidate and final measurements, costs, and outcomes.
4. **Working memory** is a bounded, removable subset deterministically retrieved for one problem. Limits cover record count, approximate characters, age, and included tiers.

Persistent records use memory schema `0.1`. They contain stable IDs, tier, kind, content, status, timestamps, source, experiment/case/trace/branch provenance, measurement IDs and authority, correlation group, and supersession links.

## Storage and retrieval

Tests use an in-memory store. Experiments may use checksummed append-style JSONL. Every line declares its schema and contains a SHA-256 checksum; malformed, truncated, mismatched, or unknown-schema data fails closed. Rewrites use an atomic same-directory rename. A deterministic hash of ID-sorted records identifies a memory snapshot.

Retrieval uses no embeddings or model judgment. Its visible score combines normalized token overlap, category, tags, strategy type, tier, recency, and supported/failure relevance. Ties are broken by stable record ID. Every retrieved item records why it was selected. Working-memory bounds are enforced before prompt construction.

Retrieved context is labeled:

```text
PRIOR EXPERIENCE — MEMORY GUIDANCE, NOT CURRENT EVIDENCE
```

Expected evaluation answers, duplicate prompts, reused case IDs, and suspicious normalized equivalents are rejected by memory dataset validation. Leakage detection is necessarily lexical and cannot identify every semantic paraphrase.

## Memory versus evidence

Historical formal measurement remains authoritative about its historical case. It is not automatically current evidence. Retrieval never increments the independent current-evidence count. Model-derived assertions remain model-derived after storage or repetition, and deterministic consolidation excludes them.

Convergent or repeated records carry correlation groups. Correlated memories are not treated as independent observations.

## Episodic learning and consolidation

Eligible completed runs append episodes only after branch and final-answer measurements are known. Episodes preserve successful and failed strategies, speculative branch history, recovery, measurement authority, provenance, latency, model calls, and token counts.

Tier 3 to Tier 2 consolidation is deterministic. A strategy requires a configurable number of supported episodes from distinct correlation groups, complete provenance, and no contradicted record for that pattern. No LLM decides what deserves promotion.

## When Memory Conflicts With Reality

Current authoritative measurement overrides memory.

- A consistent supported measurement retains the historical record and may support later consolidation.
- An authoritative contradiction marks or supersedes the memory while preserving its original content and update event.
- `INCONCLUSIVE`, `NOT_MEASURED`, and measurement `ERROR` preserve memory because they provide insufficient contrary evidence.
- A desirable remembered strategy cannot make a factually contradicted branch selectable.

## Controlled ablation

The memory experiment holds model, provider, temperature, seed, generation limits, timeout, BAN graph limits, dataset, and measurement contracts constant across:

- `BASELINE`: ordinary single-path generation.
- `BAN_COLD`: BAN without persistent memory.
- `BAN_MEMORY`: BAN with relevant prior strategy memories.
- `BAN_MISLEADING_MEMORY`: BAN with stale or misleading strategies.

The memory-on/off comparison is a controlled intervention within this benchmark, not universal causal proof. Reports include supported outcomes, retrieval/harm/contradiction events, episodes, consolidation, graph cost, and documented “memory leverage”: cold nodes/model calls minus memory-condition nodes/model calls. Positive leverage means memory reached its measured outcome with less BAN work; it does not itself establish correctness.

## Isolation and reproducibility

Experiments support fresh seeded stores, reset, read-only or writable policy, bounded configuration, initial/final snapshot hashes, and resume rejection on dataset, configuration, or initial-memory drift. Manifests and memory summaries record Git state, dataset hashes, schemas, provider/model settings, and intervention configuration.

## Limitations

Pass 4 uses lexical retrieval and leakage detection, small local JSONL stores, endpoint resource telemetry, and deterministic threshold consolidation. It has no embeddings, learned retrieval, neural memory, remote database, paid verification, KV-cache state, or PPS integration. Live results are required before claiming the memory hypothesis is experimentally supported.
