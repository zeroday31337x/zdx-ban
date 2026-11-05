# zdx-ban — Memory as Substrate: Full Model Proof Design

**Working spec — 2026-10-25**
**Author / thesis owner:** Justen Vannocker, *Architecting Intelligence*
**Implementation:** zdx-ban (BAN-0 v0.1), on Oracle free-vm-1
**Status:** DESIGN ONLY. No production W0, W1, or W2 weights exist yet.
BAN currently runs on Ollama `qwen2.5:1.5b` as a placeholder base.
This document records Justen's clarified design so future work does not
drift from it. It does not claim the AI implementation is already proven.

## 1. Thesis to prove

Memory is the substrate of intelligence.

Correction, per Justen 2026-10-25: this is not even an unproved thesis
at its root. It is the basis for human learning and intelligence —
humans learn, remember, recall, and build on prior experience; that is
how intelligence works in people. What zdx-ban must prove is that this
same substrate principle works in an artificial model: memory-first,
branching, verified promotion, and compounding advantage. Do not frame
the root principle as merely speculative; the open work is the AI
implementation, not the human principle itself.

A further claim, per Justen 2026-10-25: the memory advantage develops
exponentially with more memories, not linearly — it compounds as
episodes consolidate into reusable patterns that help many future cases.
That compounding curve is to be measured longitudinally, not from a
single case.

Testable form, as clarified by Justen:

> Using memory FIRST, before any action or response, improves model
> reasoning.

Memory-first is not retrieval bolted on after generation. The order is:

1. Retrieve working memory for the problem, bounded by record count,
   character count, age, and tier.
2. Label it as prior experience, not current evidence:
   `PRIOR EXPERIENCE — MEMORY GUIDANCE, NOT CURRENT EVIDENCE`
3. Branch, challenge, and verify against that memory.
4. Act / respond only from a branch that survives verification.
5. Record the outcome as a new episode, so the next run starts from
   a richer memory.

A second claim follows from that order: branching responses, with new
techniques explicitly labelled unverified, allow new knowledge to
develop safely — by surviving measurement, not by being repeated
confidently.

## 2. Memory tiers

From the repo's Pass 4–5 memory design:

- **Tier 1 — Foundation / structural:** stable rules and invariants.
  Explicit, historically traceable mutation only.
- **Tier 2 — Learned / domain:** conservatively consolidated reusable
  patterns, with contributing episode IDs and measurement provenance.
- **Tier 3 — Episodic / experiential:** actual run histories, including
  failed branches, recovery, candidate and final measurements, costs,
  and outcomes.
- **Working memory:** a bounded, removable subset deterministically
  retrieved for one problem, enforced before prompt construction.

Current limits, stated honestly in the repo: lexical retrieval only
(token overlap, category, tags, strategy type, tier, recency),
small local JSONL stores, deterministic threshold consolidation.
No embeddings, learned retrieval, neural memory, remote database,
KV-cache state, or PPS integration yet.

## 3. Weight stack — W0 / W1 / W2

Confirmed sequence (Justen, 2026-10-25):

| Layer | Role | Cadence | Share |
|---|---|---|---|
| W0 | Base weights. Identity and safety perimeter. | Never changes | 50% |
| W1 | Weekly updates. Consolidated custom layer. | Weekly | 30% |
| W2 | Daily updates. Fastest, most recent layer on top of W1. | Daily | 20% |
| **Total** | | | **100%** |

Rules:

- **W0 never changes.** It is who the model is, and what it will and
  won't do. Getting W0 right is a one-shot decision; later improvement
  must come through W1 and W2, inside the perimeter W0 sets.
- **W0 is loaded read-only and never modified in place.** Every W1
  release is hash-bound to a specific W0 tree hash. A changed or
  mismatched base is rejected.
- **W1 weekly** is where heavier gates, human review, and consolidation
  happen.
- **W2 daily** must be cheap and reversible — a small adapter / delta,
  not a full retrain — so it can be rolled back the next day.
- Inference stack, in order: W0 base, then W1 weekly, then W2 daily
  on top.

### 3.1 Why total corruption is impossible, in this design

Three independent layers, not one claim:

1. **Mathematics / proportions.** W1 + W2 together are 50%. They can
   never outvote the frozen 50% W0 on their own, and they can never
   *become* the model.
2. **Time / schedules.** W1 and W2 are not updated together.
   A single corruption event can hit at most one layer at a time,
   and the next retrain overwrites or exposes it before it can align
   with the other layer. Corrupting the whole adaptive stack would
   require two separate, time-variant logic bombs — one for the
   weekly cycle, one for the daily cycle — detonating at precisely
   the right overlap, with the second still working after the first
   daily training corruption. Justen's assessment: impossible.
3. **Guards.** Hash-binding to W0, rejection of corrupt / truncated /
   out-of-release checkpoints, verification before promotion, and
   current authoritative measurement overriding memory.

Worst case, therefore, is not unsafe output. It is a **freeze** —
a safe halt, detectable and recoverable, with W0 untouched underneath
and clean W1/W2 restorable on top of it.

Safety guards are still required even though the mathematics makes
total corruption impossible. The math sets the bound; the guards
detect corruption, enforce the bound, and trigger the freeze.
Defense in depth, not reliance on any single layer.

> Open design tension, recorded not resolved: the repo currently
> defines W2 only as a promotion-eligibility bar (a candidate becomes
> W2-eligible after independent SUPPORTED reconfirmation in 3
> separate runs), with no W2 trainer, release contract, or activation
> path. That definition must be reworked into real daily W2 weights.
> Open question: is daily W2 provisional by default (reversible,
> rolled up into weekly W1 only after reconfirmation), or does it
> require the same 3-run threshold before it can be used at all?
> This spec does not decide that; it flags it as the next design
> decision.

## 4. Promotion ladder — how new knowledge earns its place

New techniques start labelled, and only measurement moves them:

- **UNVERIFIED / UNCERTAIN:** a new branch, technique, or memory.
  Model-derived support alone can never upgrade it to SUPPORTED.
- **W1 candidate:** one authoritative, deterministic verification
  of the selected reasoning outcome (in the repo, via
  `ban training w1-dataset` from trace candidate records whose
  target is `W1_CANDIDATE`).
- **W2 eligible / daily layer:** repeated independent verification
  across separate runs, and/or that day's verified candidates,
  subject to the open question in §3.1.
- **Consolidated (W1 weekly):** deterministic Tier 3 → Tier 2
  consolidation, with complete provenance, distinct correlation
  groups, and no contradicted record for the pattern. No LLM
  decides what deserves promotion.
- **Contradicted / superseded:** current authoritative measurement
  overrides memory. The historical record is marked or superseded,
  its original content preserved, not silently deleted.
  `INCONCLUSIVE`, `NOT_MEASURED`, and measurement `ERROR` preserve
  memory, because they are insufficient contrary evidence.

Correlated memories are not independent observations. Multiple
convergent graph paths do not automatically become independent votes;
evidence carries a correlation group, and correlated paths count once.

## 5. Proof experiments — what would count as proof

The repo already contains the ablation framework (Pass 4–5).
Live results are required before claiming the hypothesis is
experimentally supported. The repo is explicit: better memory
performance does not prove remembered information is true; it
demonstrates that historical information improved measured behaviour
under the recorded experimental contract.

Conditions, same model / prompt / temperature / seed / limits:

- `BASELINE`: ordinary single-path generation.
- `BAN_COLD`: BAN without persistent memory.
- `BAN_MEMORY`: BAN with relevant prior strategy memories.
- `BAN_MISLEADING_MEMORY`: BAN with stale or misleading strategies.

Datasets in repo: 20-case smoke and 100-case sets
(`datasets/ban-memory-experiment-001*.jsonl`), plus a versioned
foundation memory (`datasets/foundation-memory-v1.jsonl`).
Categories include useful transfer, irrelevant rejection, stale
override, misleading recovery, novelty preservation, repeated-problem
efficiency, failure-memory leverage, and conflicting observations.

Metrics to report, not just accuracy:

- Verified accuracy, by condition and by category. Only cases
  accepted by a registered deterministic verifier count as primary
  accuracy.
- Recovery rate: successful BAN final verification where BAN's
  initial top branch failed the objective verifier.
- Memory leverage: cold nodes / model calls minus memory-condition
  nodes / model calls. Positive leverage means memory reached the
  measured outcome with less BAN work; it does not itself establish
  correctness.
- Leverage curve (folded into leverage, per Justen 2026-10-25):
  report leverage not as a single number, but as a curve over
  growing memory — leverage vs episodes / records / repetitions,
  with memory-write writable and online learning carried forward
  across prompts. Per Justen, the advantage develops exponentially
  with more memories, because context and passed memories help
  future branches, and consolidated patterns help many future cases
  at once. The transfer logic, in his terms, cannot fail in
  principle: context already helps a model, and memory is simply
  persistent context from prior runs, so a future branch starting
  with relevant prior context cannot be worse off than a cold
  branch, provided retrieval is relevant and verification /
  override still apply. The curve to measure is therefore:
  initial leverage (memory-001 live run: 2 branches / 2 calls
  avoided) → leverage after N episodes → leverage after
  consolidation to Tier 2. A flat or linear curve would contradict
  the exponential claim; a superlinear curve, with accuracy held
  or improved and misleading-memory recovery intact, would support
  it. Report tokens alongside calls, since memory-001 used fewer
  calls but more tokens than cold.
- Misleading-memory recovery: can BAN escape incorrect historical
  assumptions when current evidence disagrees? A harmful-memory
  recovery result is valuable, not a failure to hide.
- Cost: nodes, model calls, tokens, latency, and graph cost per
  condition. The architecture-default comparison intentionally
  permits BAN's additional calls; it is not an equal-token-budget
  experiment, so cost must be reported alongside accuracy.
- Outcomes must stay distinct: `SUPPORTED`, `CONTRADICTED`,
  `INCONCLUSIVE`, `UNSUPPORTED`, `NOT_MEASURED`, `FAILED`, `ERROR`.
  Unknown, inconclusive, or failed measurement never becomes success.

Feasibility note (2026-10-25, checked on free-vm-1):
`./bin/ban experiment memory-smoke --dry-run` validates 20 cases,
4 conditions, up to ~1,100 provider calls, with a 5-hour bounded
upper run time on this 2-CPU VM. A full live smoke is therefore
not a quick turn; a single-case live run (`-case-id`, `-limit 1`)
is the feasible first live data point, with the full smoke run
as a background job.

## 6. End uses

- **Intelligence units:** each unit runs the same core —
  memory-first, branching, verified promotion, W0/W1/W2 stack —
  with its own memory and W1/W2 weights, sharing the frozen W0
  identity / safety perimeter. (Definition of "unit" — separate
  deployment, agent, or device — to be confirmed by Justen.)
- **Data-synthesizing platform — datasynthesis.zerodrivex.com:**
  BAN traces and verified candidates are provenance-tracked
  synthetic data, not just generated text: what was tried, what
  failed, what was measured, what got promoted, and under which
  W0/W1/W2 versions. That feed supports:
  - **Predictions:** memory-first + branched + labelled uncertainty,
    not a single confident answer.
  - **Data creation:** verified branches become training datasets
    (the repo's `ban training w1-dataset` extraction is the seed
    of this), with human review still required before release
    assembly.
  - **New knowledge:** promoted tiers and consolidated W1/W2
    weights become the next base for reasoning, closing the loop
    from episode → candidate → weight → memory.

## 7. Implementation gaps, in order

1. **This spec** — locked design, open questions flagged.
2. **Real W0.** One-shot, frozen, hash-bound. Repo pipeline exists
   (corpus validator, BPE tokenizer builder, immutable release,
   Llama-style trainer, ~50M pilot template), proven only on a
   synthetic toy (2-layer / hidden-32, 8 records). A real 1–1.5B
   W0 needs ~20–30B tokens by the repo's planning numbers, and
   the repo states this 2-CPU / 12 GB VM is not suitable for
   professional 1B–1.5B pretraining with Adam. Decision needed:
   small pilot W0, an interim HuggingFace Transformers base
   (note: the W1 trainer requires an unquantized Transformers
   checkpoint — an Ollama GGUF cannot be used for PEFT training),
   or GPU training elsewhere.
3. **W1 weekly trainer + gates + registration.** Trainer exists
   (LoRA, streamed, resumable, exit 75 on low memory), proven only
   on 4 synthetic examples. Missing: task / safety / regression /
   execution-failure gates, and an adapter registration / rejection
   mechanism. Process completion alone never promotes an adapter.
4. **W2 daily trainer + activation.** Does not exist in the repo
   beyond eligibility. Needs a release contract analogous to W1's,
   a cheap daily training path, a merge path, and an activation /
   rollback path.
5. **Merge + quantize for inference.** LoRA adapters must be merged
   into the base and converted to GGUF for Ollama / BAN use, with
   versioned W0/W1/W2 identities preserved through the merge.
6. **Memory architecture upgrade.** Beyond lexical JSONL retrieval,
   as decided by Justen — embeddings, learned retrieval, or neural
   memory are options, not assumptions in this spec.
7. **Live ablation.** Single-case live run first, then full
   memory-smoke, then the 100-case set, with reports regenerated
   from raw JSONL observations.
8. **Datasynthesis integration.** Export format for traces and
   verified candidates, with W0/W1/W2 version, memory snapshot hash,
   dataset hash, and verifier identity on every record.

## 8. Open questions for Justen

- Freeze on corruption: halt entirely until clean W1/W2 are
  restored, or fall back to W0-only answers? (Asked 2026-10-25,
  not yet answered; this spec assumes halt.)
- W2 daily promotion threshold: provisional daily layer, or
  3-run reconfirmation before use? (See §3.1.)
- Base architecture: LOCKED 2026-10-25 by Justen — true
  from-scratch Llama-style W0, in the repo's Transformers format,
  not a Qwen2-1.5B-shaped interim base. Corpus and size decisions
  for that W0 remain open.
- Intelligence units: what exactly is a unit?
- Real corpus: what licensed, provenance-preserving corpus is
  approved for W0? No real corpus is claimed in the repo, and
  safety review currently has no gate — it needs a real classifier
  or human review pipeline, not a keyword denylist.

---
*Sources checked on free-vm-1, 2026-10-25: `~/zdx-ban/MEMORY.md`,
`EXPERIMENTS.md`, `MEASUREMENT.md`, `TODO.md`, `DEVELOPMENT_STATUS.md`,
`training/w0/README.md`, `training/slow_android/README.md`,
`ban.config`, and `./bin/ban experiment memory-smoke --dry-run`.*

---

## Status update — 2026-11-01 (appended; design above unchanged)

**Locked decisions since this spec was written (2026-10-25):**

- W0 shape LOCKED 2026-10-25: true from-scratch Llama-style W0, repo
  Transformers format, ~375M params, BF16 ~750 MB, edge-runnable.
  Verified config (calculation, then confirmed present on free-vm-1):
  hidden 1024, 29 layers, intermediate 2816, 8 heads, 4 KV heads,
  vocab 32000, tied embeddings = 374,926,336 params. Config:
  `~/zdx-ban/training/w0/w0-375m-config.json`. Do not use the
  superseded 24-layer untied draft.
- W0 corpus LOCKED 2026-10-26: WikiText-103 + Project Gutenberg,
  300M total tokens, single epoch, 32k tokenizer. Not FineWeb-Edu,
  not TinyStories alone. This answers the open corpus question in
  §8 and the "decision needed" in §7 item 2: the decision is the
  375M from-scratch W0 trained on free-vm-1 only, as large as the
  VPS allows. No GPU training elsewhere, no paid upgrade.

**Phase A — DONE 2026-10-26:** WikiText-103 + Gutenberg assembled in
`~/zdx-ban/data/w0-release-300m/` — 8,488 records (2,698 WikiText
docs, 5,777 Gutenberg docs, 6 validation, 7 test), ~260.6M words
(word counts, not token counts).

**Phase B Step 1 (validation) — FAILED 2026-10-26, REPAIRED and
CONVERGED 2026-11-01:**

- Oct 26 failure: `~/w0-phase-b.py` Step 1 raised "validation did
  not converge" after 20 attempts. Root cause, proven from
  `~/w0-phase-b.log`: the validator caps its error list at
  `--max-errors` (default 100), so each attempt removed at most 100
  lines — at most 2,000 removals — but attempt 1 found 2,049 invalid
  records. Convergence was mathematically impossible under those
  parameters. The script deleted lines in place (1,998 Gutenberg +
  2 WikiText over 20 attempts).
- Invalid records were overwhelmingly Gutenberg header/boilerplate
  tripping the email PII rule (all matched addresses `.org` —
  Project Gutenberg credit lines, not personal data), plus a small
  number of canonical-hash mismatches. The PII rule was NOT
  weakened; removal is the pipeline's designed behavior.
- Anomaly, recorded honestly: across attempts 5–8 the accepted count
  fell 6,439 → 6,216, meaning ~223 lines the validator had counted
  as accepted were deleted. The mechanism could not be determined
  from surviving evidence. It stopped after attempt 8 (accepted
  rock-stable at 6,216 across 12+ later validations), and a fresh
  full-list validation on 2026-11-01 verified all 277 remaining
  flagged lines as genuinely invalid with exact line numbers and
  zero duplicates, so the repair loop is robust to it regardless.
  Those ~223 lines are not recoverable from the release directory
  (no backup existed); both source datasets are in the VM's
  HuggingFace cache, so a Phase A regeneration could recover them,
  but that is a separate decision, not done unilaterally. The
  corpus still sits in the 300M-token cap region after validation
  (~301.5M chars/4 over accepted records), so the loss does not
  block the token cap.
- Nov 1 repair (in place): `~/w0-phase-b.py` now passes
  `--max-errors 100000`, allows 50 attempts, and aligns line
  handling to newline splitting (the validator numbers lines by
  newline iteration). Original script backed up as
  `~/w0-phase-b.py.bak-20261101`; corpus backed up before the
  repair run to `/tmp/w0-corpus-pre-repair-20261101/`. A Step-1-only
  driver (`~/w0-step1-repair.py`) re-validated to convergence
  without touching later steps.
- Converged state, 2026-11-01: **6,216 records, 0 invalid, 0 PII**
  (Gutenberg 3,507; WikiText 2,696; validation 6; test 7).
  Report: `/tmp/w0-validation-repair.json`.

**NOT yet run (do not assume):** Phase B Steps 2–8 (provisional
tokenizer, token measurement + truncation to the 300M cap,
re-validation, final tokenizer, train config, release preparation
`W0-375M-300M-20261026`, preflight) and any training. WARNING:
the full `~/w0-phase-b.py` as written auto-starts training at
Step 9 if preflight passes. Full W0 training start is an open
decision with Justen (months on 2 CPUs; must not compete with the
live production platform on the same VM) — never run the script
end-to-end without gating Step 9.

**Open questions unchanged:** freeze semantics (halt entirely vs
W0-only fallback); W2 daily threshold (provisional-daily + weekly
consolidation vs 3-run reconfirmation before use); what exactly an
"intelligence unit" is. W2 remains design-only (see the W2 design
doc); no W2 trainer, release contract, or activation path exists
in the repo yet.

---

## Status update — 2026-11-01, continued (accept decision + Phase B progress)

**DECISION by Justen, 2026-11-01 (via the parent agent): ACCEPT
the ~223-line Phase B corpus loss. Do NOT regenerate Phase A
from the HuggingFace cache.** The converged 6,216-record corpus
is the corpus Phase B proceeds with. The accept-vs-regenerate
question is now CLOSED as accept; it is no longer an open
decision. The loss itself (~223 accepted lines deleted across
the Oct 26 attempts 5–8 by an undetermined mechanism) stays
recorded as a known imperfection of the release, not hidden.

**Completion during training (pipeline set up, NOT run):** the
missing lines are not abandoned — they are deferred to training
time, so they do not block Phase B now. The mechanism, built
2026-11-01 and toy-tested (not run on real corpora):

- Tool: `~/w0-corpus-complete.py` (local copy in
  `~/workspace/zdx-ban-phase-b-drivers/`). It diffs a FRESH
  Phase A regeneration (separate directory, produced only when
  training is actually being prepared, with Justen's training
  go-ahead) against the frozen release by stable record `id`,
  extracts records present in the regeneration but absent from
  the release, and validates the extracted set with the repo
  validator (full error list) — refusing unless valid with
  0 invalid. Same `id` with a different `content_sha256` is a
  conflict: refused for human review, never guessed. It NEVER
  writes into the release directory.
- Toy test (2026-11-01, on the VM): diff extracted exactly the
  1 missing record; the validator gate was demonstrated twice
  (first toy failed on the validator's 200-character minimum
  and was refused with no output; corrected toy validated
  clean); the conflict path was demonstrated (same id, changed
  hash → refused, exit 2, no output).
- Downstream branches, decided at training time with Justen's
  go-ahead, NOT by this tool: (a) if training has NOT started,
  the completion set may be folded into a NEW Phase B pass
  (Steps 2–8 re-run on the completed corpus — new tokenizer
  binding, new validation, new or formally amended release id;
  the old release is kept, never overwritten), because the
  tokenizer and manifest are hash-bound to the exact corpus;
  (b) if training HAS started, W0 is frozen and completed lines
  go to W1 candidate data ONLY, never into W0.

**This decision is NOT an authorization to start training.**
Training start remains an open decision with Justen. Step 9
stays gated off (see below); no training has started.

**Phase B progress, 2026-11-01 evening:** Step 1 converged
(6,216 records). Step 9 in `~/w0-phase-b.py` HARD-DISABLED
(repaired script backed up as
`~/w0-phase-b.py.bak-repaired-20261101`; script compiles;
no `Popen` remains) — even an end-to-end run now stops at
Step 9 with an explicit refusal. Converged corpus backed up to
`~/w0-corpus-post-step1-20261101/` (sha256s match the Step 1
repair report). Step 2 DONE and verified: provisional 32k
byte-level BPE tokenizer at `/tmp/w0-tokenizer-prov`
(status `TOKENIZER_FROZEN`, vocab exactly 32000, pad 0 /
bos 1 / eos 2 / unk 3, metadata shard hashes match all four
corpus files). Step 3 DONE (measured, 2026-11-01):
WikiText train 116,131,728 tokens + Gutenberg kept
167,491,873 tokens = **283,623,601 total train tokens
(provisional tokenizer), 16,376,399 tokens UNDER the 300M
cap — so ZERO Gutenberg docs were dropped** (3,507 kept,
unused file empty). The chars/4 estimate (~301.5M) had
overstated the real BPE count; the cap is met with headroom.
(One benign warning during counting: a transformers notice
that a 42,789-token doc exceeds the tokenizer's 1024
model_max_length — counting does not truncate, so the
measurement stands.) Step 4 DONE (re-validation, 2026-11-01):
converged on attempt 1 — valid, 6,216 records, 6,216 accepted,
0 invalid, 0 PII, 0 duplicate ids, 0 duplicate contents; all
four file hashes identical to the Step 1 repair report
(Gutenberg hash unchanged at `56947d34…`, confirming Step 3
dropped nothing and rewrote the file byte-identically).
Final validation report: `/tmp/w0-validation-final.json`.
Step 5 (final tokenizer inside the release dir) DONE:
final tokenizer frozen in
`~/zdx-ban/data/w0-release-300m/tokenizer/` (vocab 32000),
and its recount is IDENTICAL to the provisional measurement
(WikiText 116,131,728 + Gutenberg 167,491,873 =
283,623,601) — an exact cross-check of Step 3. Note: the
parent exec's SSH session dropped mid-run (exit 255,
transport artifact); the VM-side job SURVIVED and was NOT
restarted (no duplicate run). Step 6 DONE: train config
from the final count — 69,245 optimizer steps
(ceil(283,623,601 / 4096)), warmup 3,462, 1 epoch, seq 256,
lr 3e-4, fp32, grad checkpointing, 2 threads, seed 20260826
(`/tmp/w0-train-config-375m.json`). Step 7 DONE: release
`W0-375M-300M-20261026` prepared (status
`READY_FOR_W0_PREFLIGHT`; manifest `w0-release.json` with
model-config, train-config, and tokenizer artifacts; corpus
shard hashes identical to Step 1/Step 4). Step 8 DONE:
preflight reports **ready=true, errors=[]**, profile
`w0-375m-cpu`, available memory 10,413 MB, free disk
19,774 MB. **STOPPED HERE BY DESIGN. No training started:**
no `~/zdx-w0-output` dir, no `~/w0-train.log`, no training
process running, and Step 9 in `~/w0-phase-b.py` remains
hard-disabled. Phase B Steps 2–8 are COMPLETE; training
start remains an open decision with Justen.
