# zdx-ban — W2 Daily Weights Layer: Design Only

**Draft — 2026-10-25, design only, no training started**
**Owner:** Justen Vannocker
**Stack context:** W0 50% frozen base (identity / safety perimeter, never
changes) → W1 30% weekly → W2 20% daily. Total 100%.

## 1. What W2 is, in the confirmed design

W2 is a real, daily weights layer on top of weekly W1, not the repo's
current definition.

Current repo definition (to be reworked, not reused as-is):
W2 is only promotion-eligibility — a candidate becomes W2-eligible
after independent SUPPORTED reconfirmation in 3 separate runs
(`internal/training.W2PromotionCandidates`, `ban training w2-promote`).
There is no W2 trainer, no W2 release contract, no activation path,
and no W2 weights artifact.

Confirmed design (Justen, 2026-10-25):
W2 is daily updates, 20% of the total model, sitting on top of W1
weekly (30%) and W0 base (50%). It is the fastest, most recent layer,
and it must be cheap and reversible.

## 2. Daily cycle — what happens each day

1. **Collect:** that day's BAN runs write traces and candidate records
   (`traces/<run-id>.candidates.jsonl`, as already wired from
   `internal/cognitive.CandidatesFromBANTrace`).
2. **Filter:** keep only candidates whose deterministic verifier
   actually confirmed the selected outcome. Model-derived support
   alone never upgrades UNCERTAIN to SUPPORTED. New techniques stay
   labelled UNVERIFIED until measured.
3. **Review:** human review of the resulting dataset before any
   release assembly — the same discipline the W1 path already
   requires for `w1-training.jsonl` (the repo tool does no
   deduplication beyond exact candidate ID, and no quality, PII,
   or safety review).
4. **Train (small):** a small LoRA-style delta against the frozen
   W0 + current W1 stack, not a full-weight retrain. Same worker
   principles as the W1 slow trainer: streamed examples, gradient
   checkpointing, checkpoints at optimizer-step boundaries, pause
   on low memory (exit 75), resume from `state.json`, W0 loaded
   read-only and never modified in place.
5. **Gate:** task / safety / regression / execution-failure gates
   must pass before activation. These gates do not exist yet as
   runnable infrastructure for W1 either — they are a shared gap,
   not a W2-only gap. Process completion alone never promotes
   an adapter.
6. **Activate / rollback:** activate the new daily W2 for the next
   day, keeping the previous day's W2 intact for instant rollback.
   If a gate fails, or a checkpoint is corrupt / mismatched /
   out-of-release, reject it and freeze to the last good W2 (or
   halt, per the open freeze question — see §5), never load it
   silently.
7. **Consolidate weekly:** the weekly W1 run reviews that week's
   daily W2 candidates, keeps what survives reconfirmation,
   discards the rest, and folds the survivors into W1. Daily W2
   is therefore provisional in effect, even if it is usable the
   day it is trained — weekly consolidation is where it earns
   permanence.

## 3. Release contract (proposed, mirroring W1)

A W2 release directory must contain, at minimum:

- `w2-training.jsonl` — that day's verified instruction/output
  (or candidate) records, with provenance back to trace / run IDs.
- `w2-release.json` — `release_id`, `training_sha256`, the exact
  W0 tree hash, the exact W1 release hash it sits on top of,
  date, and gate results.
- `adapter_model.safetensors` (or equivalent delta) + config,
  with a deterministic tree hash, written via isolated temporary
  directories as the W1 worker already does.
- A `state.json` bound to release ID, dataset hash, W0 hash, and
  W1 hash, so resume cannot drift to a different release.

Preflight must verify, as W1 preflight does: release and dataset
hashes, exact W0 and W1 tree hashes, required checkpoint files
and packages, isolated output, available memory, and free disk.
Memory / disk shortages are retryable; integrity or dependency
failures are not.

## 4. Why daily W2 cannot corrupt the whole model

Three layers, already agreed with Justen:

1. **Proportions:** W2 is only 20%. Even W1 + W2 together are 50%,
   against frozen W0 at 50%. They cannot outvote W0 or become
   the model.
2. **Time separation:** W2 retrains daily, W1 weekly, on different
   schedules and different training windows. A single corruption
   event can hit at most one layer at a time, and the next retrain
   overwrites or exposes it. Corrupting both would require two
   separate time-variant logic bombs at precise detonation, with
   the second surviving the first daily corruption — assessed
   by Justen as impossible.
3. **Guards:** W0 hash-binding, corrupt-checkpoint rejection,
   unverified labelling until measured, current authoritative
   measurement overriding memory, and freeze on detection.
   Worst case is a freeze, not unsafe output, with W0 untouched
   and clean W1/W2 restorable.

Safety guards are still required even though total corruption is
assessed as impossible by the mathematics and the schedules.
Defense in depth.

## 5. Open questions — do not assume these

- **Freeze semantics:** halt entirely until clean W1/W2 are
  restored, or fall back to W0-only answers? Asked 2026-10-25,
  not yet answered. This draft assumes halt, matching the
  freeze language used so far.
- **Daily W2 promotion threshold:** is a daily W2 usable the day
  it is trained (provisional, reversible, rolled up weekly), or
  does it require the repo's 3-run reconfirmation before use?
  This draft proposes provisional-daily + weekly consolidation,
  but Justen must confirm it.
- **W0 shape — LOCKED 2026-10-25:** true from-scratch
  Llama-style W0, in the repo's Transformers format, not a
  Qwen2-1.5B-shaped interim base. The W1/W2 trainer requires an
  unquantized Transformers checkpoint as base; an Ollama GGUF
  cannot be used for PEFT training. Training still waits for
  the full memory smoke to finish, and W0 corpus/size remain open.
- **Merge / inference path:** LoRA deltas must be merged and
  converted to GGUF for Ollama / BAN use, with W0/W1/W2 version
  identities preserved through the merge. That path is not
  designed yet.
- **Memory vs weights:** daily W2 weights are not the same as
  daily episodic memory writes. Episodes are written every run;
  W2 is a daily trained delta from verified episodes. Keep the
  two distinct in code and in release notes.

## 6. What is NOT in this draft

- No training code, no trainer run, no adapter artifact.
- No full-model pretraining on free-vm-1 (deliberately excluded:
  a real 1–1.5B W0 needs ~20–30B tokens by repo planning, and
  this 2-CPU / 12 GB VM is not suitable for that).
- No change to W0, which never changes.
- No public exposure, no service, no change to the SSH-only
  firewall.

## 7. Next implementation steps, in order

1. Lock W0 shape — DONE 2026-10-25: true from-scratch Llama-style W0.
2. Confirm freeze semantics and daily-W2 threshold (§5).
3. Build W2 release contract + preflight, mirroring W1.
4. Build W2 daily trainer as a small-delta variant of the
   W1 slow trainer, with the same resume / pause / hash discipline.
5. Build gates + registration / rejection, shared with W1.
6. Build merge → GGUF + activation / rollback, with version
   preservation.
7. Measure the leverage curve (leverage vs episodes over
   repetitions) on live runs — thesis reasoning proves itself;
   implementation is what needs proving.

---

## Status update — 2026-11-01 (appended; design above unchanged)

- W0 shape LOCKED 2026-10-25: true from-scratch Llama-style W0,
  ~375M params, BF16 ~750 MB, config verified on free-vm-1
  (hidden 1024, 29 layers, intermediate 2816, 8 heads, 4 KV heads,
  vocab 32000, tied embeddings). This completes §7 step 1.
- W0 corpus LOCKED 2026-10-26: WikiText-103 + Project Gutenberg,
  300M total tokens, single epoch, 32k tokenizer.
- Phase A DONE 2026-10-26 (8,488 records in
  `~/zdx-ban/data/w0-release-300m/`).
- Phase B Step 1 (validation) FAILED 2026-10-26 on a validator
  error-list cap (100 per run × 20 attempts < 2,049 invalid
  records), then REPAIRED and CONVERGED 2026-11-01: 6,216 records,
  0 invalid (Gutenberg 3,507; WikiText 2,696; validation 6;
  test 7). See the proof-spec status update for the full
  account, including ~223 accepted lines lost in the Oct 26
  attempts by an undetermined mechanism.
- Correction to §6: the "no full-model pretraining on free-vm-1"
  line is superseded by the locked 375M / 300M-token plan.
  Training itself has NOT started and remains an open decision
  with Justen (months on 2 CPUs; must not compete with the live
  production platform on the same VM). HELD by Justen 2026-11-01:
  no training until after the platform is built for the
  submission. The full Phase B script
  auto-starts training at Step 9 if preflight passes, so it must
  never be run end-to-end without gating that step.
- W2 itself is unchanged: still design-only. No W2 trainer,
  release contract, or activation path exists in the repo yet.
- W2 open questions DECIDED 2026-11-01 by Justen (see Decision
  brief below, formerly recommendations): layer-dependent freeze;
  provisional-daily W2 with the 3-run bar at W1; intelligence unit
  = deployment instance; atomic pointer-swap activation with
  previous-day rollback.

## Status update — 2026-11-01 evening (Phase B complete through preflight)

- Phase B Steps 2–8 COMPLETE: provisional + final 32k tokenizers,
  measured 283,623,601 train tokens (under the 300M cap, zero docs
  dropped), re-validation converged attempt 1 (6,216 accepted),
  train config (69,245 steps), release `W0-375M-300M-20261026`
  (`READY_FOR_W0_PREFLIGHT`), preflight ready=true with no errors.
  STOPPED after preflight: no training started, Step 9
  hard-disabled, training start still open with Justen.
- Corpus-loss decision (Justen, 2026-11-01): the ~223-line loss
  is ACCEPTED; no Phase A regeneration; completion deferred to
  training time via `~/w0-corpus-complete.py` (toy-tested; never
  writes into the frozen release; before training starts it would
  mean a new Phase B pass, after training starts W1 data only).
- W2 decisions APPROVED by Justen 2026-11-01 (brief below,
  formerly recommendations): layer-dependent freeze,
  provisional-daily W2 with the 3-run bar at W1, unit = deployment
  instance, atomic pointer-swap activation with previous-day
  rollback.

---

## Decision brief — 2026-11-01 (APPROVED by Justen 2026-11-01)

Written by the parallel zdx-ban continuation agent while Phase B
Step 2 (provisional tokenizer) runs on free-vm-1. Justen approved
all four items 2026-11-01; they are now the W2 design decisions,
not recommendations.

### B1. Freeze semantics: make the freeze depend on WHICH layer failed

The halt-vs-fallback question as asked treats all corruption the
same. The stack gives a sharper distinction, and it matters
because W0 alone is a complete model, not half a model: the
50/30/20 shares describe influence on the combined stack, not
completeness. W0 (375M, from-scratch, frozen) can answer on its
own; W1/W2 are deltas on top of it.

- **If W0 itself fails integrity** (hash mismatch, truncated or
  out-of-release base): HALT entirely. There is no safe layer
  underneath to fall back to, and serving anything would break
  the identity/safety perimeter. This is the freeze case in its
  strictest form.
- **If only W1 and/or W2 fail** (bad adapter hash, gate failure,
  corrupt daily delta, suspect daily candidates): FALL BACK to
  W0-only answers, explicitly labelled as W0-only (no weekly or
  daily learning applied), with learning frozen until a clean
  W1/W2 is restored. Availability is preserved, the safety
  perimeter (W0) is intact, and no suspect weight influences
  any answer. Rollback to the previous day's W2 (kept intact
  by design) is the first recovery step; W0-only is the
  fallback while recovery runs.

RECOMMENDATION: adopt layer-dependent freeze (halt on W0
failure, labelled W0-only fallback on W1/W2 failure). It keeps
the "worst case is a freeze, not unsafe output" guarantee while
avoiding a total outage for the most common failure mode
(a bad daily delta). Justen to confirm.

### B2. W2 daily threshold: provisional use, with the 3-run bar moved to W1

Two readings of "daily" are in tension. If a daily candidate
needs 3 independent reconfirmations before ANY use, then in
practice most daily candidates are never used before the weekly
W1 consolidation reviews them anyway — W2 becomes a redundant
staging area for W1, not a daily layer. If daily candidates are
usable the day they are trained, W2 earns its cadence, and its
safety comes from reversibility instead of pre-use proof: keep
yesterday's W2 intact, roll back instantly on any gate or
quality signal, and let weekly W1 consolidation (with its
heavier gates and human review) decide permanence.

- **Provisional-daily (recommended):** a verified candidate
  (deterministic verifier confirmed the outcome; model-only
  support never counts) may enter that day's W2 and be used
  immediately, labelled as daily/provisional. It earns
  permanence only through weekly W1 consolidation, where the
  3-run reconfirmation bar applies.
- **3-run-before-use (not recommended for W2):** keep the
  repo's existing bar, but apply it at W1 consolidation, not
  at daily activation. Applying it at daily activation would
  delay every new technique by days and defeat the daily layer.

RECOMMENDATION: provisional-daily + weekly consolidation, with
the 3-run bar as the W1 permanence bar. Justen to confirm; this
is the draft's existing proposal (§5), now with the reasoning
for why the alternative collapses W2 into W1.

### B3. Intelligence-unit definition: the deployment boundary

"Unit" has been used for a deployment, an agent, and a device.
The stack description already constrains it: a unit runs the
same core (memory-first, branching, verified promotion,
W0/W1/W2) with its OWN memory and W1/W2 weights, sharing the
frozen W0. That is a boundary definition, not a hardware
definition:

- **A unit = one deployment instance of the BAN system** —
  one running service or agent team with its own memory store
  and its own W1/W2 releases, all bound to the same frozen W0.
  It may run on a server, on an edge device, or as one member
  of a multi-agent team, but the unit boundary is where memory
  and adaptive weights are owned, not the hardware it runs on.
- Consequences: two units never share episodic memory or W1/W2
  by default (sharing would need an explicit, provenance-tracked
  merge); W0 updates never happen per-unit (W0 is global and
  frozen); a unit's W2 rollback never affects another unit.

RECOMMENDATION: define "intelligence unit" as the deployment
instance above. Justen to confirm or correct (in particular
whether a multi-agent team counts as one unit or several).

### B4. Activation / rollback mechanics (to be built, not assumed)

For the future W2 builder, the minimum viable mechanics implied
by §2–§3: activation is an atomic pointer swap to the new
daily adapter + state.json (never an in-place overwrite); the
previous day's adapter and state.json are retained until the
next day's activation succeeds; rollback is re-pointing at the
previous day's adapter (no retraining); a failed gate or hash
check at activation time leaves the previous day's W2 live and
records a freeze/rollback audit event. These mechanics are
design only until built and tested against the W1 worker's
existing resume/hash discipline.
