# W0 corpus contract

W0 is a causal language model trained from randomly initialized weights. It is
not a W1 adapter and it does not consume the W1 `instruction`/`output` format.
Each example is a complete raw document in sharded JSONL, and the model learns
next-token prediction over packed, tokenized `text`.

Status: an unvalidated CPU-oriented pilot pipeline is implemented. It contains
a corpus validator, byte-level BPE tokenizer builder, immutable release
assembler, approximately 50M-parameter Llama-style architecture, and resumable
full-weight trainer. No real tokenizer, successful full-weight run, accepted
checkpoint, or production W0 is claimed. The current 8 GiB-class VPS is useful
for a tiny overfit and slow pilot; it is not suitable for professional 1B-1.5B
pretraining with Adam.

## Record format

One UTF-8 JSON object per line:

```json
{
  "id": "stable-source-document-id",
  "text": "The complete normalized document...",
  "source": "licensed-corpus/release/path",
  "source_type": "documentation",
  "language": "en",
  "license": "Apache-2.0",
  "split": "train",
  "synthetic": false,
  "content_sha256": "lowercase SHA-256 of canonical text"
}
```

Synthetic documents additionally require the exact `generator` identity and a
`generation_prompt_hash`. Recording generator revision, generation parameters,
verification results, and parent source IDs is strongly recommended. The
machine-readable shape is in `dataset.schema.json`.

Canonical text uses NFC Unicode and LF newlines. Compute its hash with:

```python
from training.w0.validate_dataset import content_sha256
```

Assign train/validation/test at the source-document level before tokenization or
chunking. Do not allow variants or chunks of one source document to cross
splits. Keep task benchmarks in a separate denied-hash set before corpus
generation so the validator can reject exact contamination.

## Corpus composition

A general base needs diverse, licensed, provenance-preserving material: prose,
books, documentation, code, math/science exposition, worked examples, and the
languages the model must actually support. Synthetic textbook passages and
exercises can improve coverage, but they should be a measured component anchored
by real material—not repeated templates or an unchecked synthetic-only loop.

Before a shard is admitted, apply and record:

- license/consent review and source provenance;
- secret, credential, PII, and disallowed-content removal;
- Unicode and structural normalization;
- language and quality classification;
- exact and near-duplicate removal across every split;
- benchmark and evaluation contamination screening;
- per-source caps so one generator or domain cannot dominate;
- factual, compilation, execution, or answer checks where the domain allows;
- human audits of random and high-risk samples.

The local validator enforces the parts that can be checked deterministically
from this record format. Near-duplicate detection, PII/license review, factual
quality, and safety review require separate pipeline stages.

## Size planning

Choose the architecture from the available training compute first. A useful
compute-optimal planning baseline is roughly 20 training tokens per parameter:
about 2B tokens for 100M parameters, 6B for 300M, 20B for 1B, or 30B for 1.5B.
Those are planning estimates, not acceptance criteria, and repeated low-quality
synthetic text does not become useful merely by increasing token count.

Train the tokenizer only after a representative, approved sample exists. Freeze
its vocabulary and special-token IDs before W0 training, then store its hash in
every W0 release and W1 release that depends on it.

## Streaming validation

The validator keeps duplicate keys in an on-disk SQLite index instead of RAM:

```bash
python training/w0/validate_dataset.py /data/w0/shards \
  --deny-hashes /data/w0/evaluation-content-sha256.txt \
  --report /data/w0/validation-report.json
```

`approximate_tokens_chars_div_4` is only a rough sizing number. Final token
counts must be produced with the frozen W0 tokenizer. A successful structural
validation is necessary but does not approve the corpus or establish model
quality.

## Build an immutable pilot release

Put shards beneath one release directory so the final manifest can use bounded
relative paths. Do not edit a release after `w0-release.json` exists.

```bash
W0_RELEASE=/data/zdx-w0-pilot-001
mkdir -p "$W0_RELEASE/corpus"
# Generate or copy reviewed *.jsonl shards into $W0_RELEASE/corpus first.

python3 training/w0/validate_dataset.py "$W0_RELEASE/corpus" \
  --deny-hashes /data/evaluation-content-sha256.txt \
  --report "$W0_RELEASE/validation-report.source.json"

python3 training/w0/build_tokenizer.py "$W0_RELEASE/corpus" \
  --output "$W0_RELEASE/tokenizer" \
  --vocab-size 16384

python3 training/w0/prepare_release.py \
  --release "$W0_RELEASE" \
  --release-id zdx-w0-pilot-001 \
  --validation-report "$W0_RELEASE/validation-report.source.json" \
  --tokenizer "$W0_RELEASE/tokenizer"
```

`prepare_release.py` binds the exact validated shards, tokenizer tree,
`model-template.json`, and `train-config.json`. The default architecture uses
12 layers, hidden size 512, grouped-query attention, tied embeddings, and a
1,024-token architectural context. Training deliberately starts at 256 tokens
to bound pilot memory.

## Preflight and train

Use an isolated virtual environment. This repository does not install heavy
packages automatically:

```bash
python3 -m venv /data/zdx-w0-venv
/data/zdx-w0-venv/bin/pip install torch transformers tokenizers safetensors

/data/zdx-w0-venv/bin/python training/w0/trainer.py \
  --release "$W0_RELEASE" \
  --output /data/zdx-w0-output \
  --preflight
```

Only after preflight succeeds, run in the foreground or configure the Ubuntu
watchdog with `ZDX_TRAINING_STAGE=W0`:

```bash
/data/zdx-w0-venv/bin/python training/w0/trainer.py \
  --release "$W0_RELEASE" \
  --output /data/zdx-w0-output
```

At each checkpoint boundary the trainer atomically saves full model weights,
optimizer, cosine scheduler, Python/Torch/CUDA RNG state, and the exact shard,
byte offset, epoch, and pending packed-token buffer. Resume is rejected if the
manifest, corpus, tokenizer, architecture, training policy, state, or checkpoint
hash changed. A crash can still lose work since the last completed checkpoint.

The final directory is `model-final`, with status
`W0_TRAINING_COMPLETE_UNVALIDATED`. It must pass held-out loss/perplexity,
deterministic resume, corruption, safety, capability, and downstream BAN gates
before it can be accepted as W0 or used as the base for a W1 release.
