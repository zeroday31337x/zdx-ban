#!/usr/bin/env python3
"""Train and atomically freeze a byte-level BPE tokenizer for a W0 corpus."""

from __future__ import annotations

import argparse
import json
import shutil
import tempfile
from pathlib import Path

from release import atomic_json, sha256
from validate_dataset import input_files, validate_record

SPECIAL_TOKENS = ["<|pad|>", "<|bos|>", "<|eos|>", "<|unk|>"]


def texts(files: list[Path]):
    for path in files:
        with path.open(encoding="utf-8") as stream:
            for line_number, line in enumerate(stream, 1):
                if not line.strip():
                    continue
                try:
                    item = json.loads(line)
                except json.JSONDecodeError as exc:
                    raise RuntimeError(f"invalid JSON at {path}:{line_number}") from exc
                item, errors, _pii_categories = validate_record(item, 1, 64 * 1024 * 1024)
                if errors or item is None:
                    raise RuntimeError(f"invalid W0 record at {path}:{line_number}: {'; '.join(errors)}")
                if item["split"] == "train":
                    yield item["text"]


def build(files: list[Path], output: Path, vocab_size: int, min_frequency: int) -> None:
    try:
        from tokenizers import Tokenizer, decoders, models, normalizers, pre_tokenizers, processors, trainers
        from transformers import PreTrainedTokenizerFast
    except ImportError as exc:
        raise RuntimeError("build_tokenizer requires tokenizers and transformers") from exc
    if output.exists():
        raise RuntimeError(f"tokenizer output already exists: {output}")
    output.parent.mkdir(parents=True, exist_ok=True)
    temporary = Path(tempfile.mkdtemp(prefix=f".{output.name}-", dir=output.parent))
    try:
        tokenizer = Tokenizer(models.BPE(unk_token=SPECIAL_TOKENS[3]))
        tokenizer.normalizer = normalizers.NFC()
        tokenizer.pre_tokenizer = pre_tokenizers.ByteLevel(add_prefix_space=False)
        tokenizer.decoder = decoders.ByteLevel()
        trainer = trainers.BpeTrainer(
            vocab_size=vocab_size,
            min_frequency=min_frequency,
            special_tokens=SPECIAL_TOKENS,
            show_progress=True,
        )
        tokenizer.train_from_iterator(texts(files), trainer=trainer)
        ids = {token: tokenizer.token_to_id(token) for token in SPECIAL_TOKENS}
        if any(value is None for value in ids.values()):
            raise RuntimeError("trained tokenizer is missing a required special token")
        tokenizer.post_processor = processors.TemplateProcessing(
            single=f"{SPECIAL_TOKENS[1]} $A {SPECIAL_TOKENS[2]}",
            special_tokens=[(SPECIAL_TOKENS[1], ids[SPECIAL_TOKENS[1]]), (SPECIAL_TOKENS[2], ids[SPECIAL_TOKENS[2]])],
        )
        wrapped = PreTrainedTokenizerFast(
            tokenizer_object=tokenizer,
            pad_token=SPECIAL_TOKENS[0],
            bos_token=SPECIAL_TOKENS[1],
            eos_token=SPECIAL_TOKENS[2],
            unk_token=SPECIAL_TOKENS[3],
            model_max_length=1024,
        )
        wrapped.save_pretrained(temporary)
        atomic_json(
            temporary / "tokenizer-metadata.json",
            {
                "schema_version": 1,
                "algorithm": "byte-level-bpe",
                "vocab_size": len(wrapped),
                "special_token_ids": {
                    "pad_token_id": wrapped.pad_token_id,
                    "bos_token_id": wrapped.bos_token_id,
                    "eos_token_id": wrapped.eos_token_id,
                    "unk_token_id": wrapped.unk_token_id,
                },
                "corpus_shards": [{"path": str(path), "sha256": sha256(path)} for path in files],
            },
        )
        temporary.replace(output)
    except BaseException:
        shutil.rmtree(temporary, ignore_errors=True)
        raise


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("inputs", nargs="+", type=Path)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--vocab-size", type=int, default=16384)
    parser.add_argument("--min-frequency", type=int, default=2)
    args = parser.parse_args()
    if args.vocab_size < 1024 or args.min_frequency < 1:
        parser.error("vocab size must be at least 1024 and min frequency must be positive")
    try:
        build(input_files(args.inputs), args.output, args.vocab_size, args.min_frequency)
        print(json.dumps({"status": "TOKENIZER_FROZEN", "path": str(args.output)}, indent=2))
        return 0
    except (OSError, RuntimeError) as exc:
        print(json.dumps({"status": "FAILED", "error": str(exc)}, indent=2))
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
