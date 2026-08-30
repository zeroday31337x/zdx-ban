#!/usr/bin/env python3
"""Bind a validated corpus, frozen tokenizer, architecture, and training policy."""

from __future__ import annotations

import argparse
import json
import time
from pathlib import Path

from release import atomic_json, path_within, sha256, sha256_tree, validate_train_config


def prepare(
    release_dir: Path,
    release_id: str,
    validation_report: Path,
    tokenizer: Path,
    model_template: Path,
    train_template: Path,
) -> dict:
    release_dir = release_dir.resolve()
    if not release_dir.is_dir():
        raise RuntimeError("release directory must already contain the corpus")
    manifest_path = release_dir / "w0-release.json"
    if manifest_path.exists():
        raise RuntimeError("w0-release.json already exists; releases are immutable")
    report = json.loads(validation_report.read_text(encoding="utf-8"))
    if not isinstance(report, dict) or report.get("valid") is not True:
        raise RuntimeError("corpus validation report is not successful")
    shards = []
    for item in report.get("files", []):
        path = Path(item["path"]).resolve()
        if not path_within(path, release_dir) or path.is_symlink():
            raise RuntimeError(f"validated corpus shard is outside release directory: {path}")
        if sha256(path) != item.get("sha256"):
            raise RuntimeError(f"validated corpus shard has changed: {path}")
        shards.append({"path": path.relative_to(release_dir).as_posix(), "sha256": item["sha256"], "records": item.get("records", 0)})
    if not shards:
        raise RuntimeError("validation report has no corpus shards")
    if not tokenizer.is_dir() or not path_within(tokenizer, release_dir):
        raise RuntimeError("frozen tokenizer must be inside the release directory")
    metadata = json.loads((tokenizer / "tokenizer-metadata.json").read_text(encoding="utf-8"))
    validated_hashes = {item["sha256"] for item in shards}
    tokenizer_hashes = {item.get("sha256") for item in metadata.get("corpus_shards", [])}
    if tokenizer_hashes != validated_hashes:
        raise RuntimeError("frozen tokenizer is not bound to exactly this validated corpus")
    architecture = json.loads(model_template.read_text(encoding="utf-8"))
    architecture["vocab_size"] = int(metadata["vocab_size"])
    architecture.update(metadata["special_token_ids"])
    training = validate_train_config(json.loads(train_template.read_text(encoding="utf-8")))
    model_config_path = release_dir / "model-config.json"
    train_config_path = release_dir / "train-config.json"
    report_path = release_dir / "validation-report.json"
    atomic_json(model_config_path, architecture)
    atomic_json(train_config_path, training)
    atomic_json(report_path, report)
    manifest = {
        "schema_version": 1,
        "release_id": release_id,
        "created_at": time.time(),
        "status": "READY_FOR_W0_PREFLIGHT",
        "corpus": {"shards": shards, "validation_report": "validation-report.json", "validation_report_sha256": sha256(report_path)},
        "artifacts": {
            "tokenizer": {"path": tokenizer.resolve().relative_to(release_dir).as_posix(), "sha256": sha256_tree(tokenizer)},
            "model_config": {"path": "model-config.json", "sha256": sha256(model_config_path)},
            "train_config": {"path": "train-config.json", "sha256": sha256(train_config_path)},
        },
    }
    atomic_json(manifest_path, manifest)
    return manifest


def main() -> int:
    here = Path(__file__).resolve().parent
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--release", required=True, type=Path)
    parser.add_argument("--release-id", required=True)
    parser.add_argument("--validation-report", required=True, type=Path)
    parser.add_argument("--tokenizer", required=True, type=Path)
    parser.add_argument("--model-template", type=Path, default=here / "model-template.json")
    parser.add_argument("--train-template", type=Path, default=here / "train-config.json")
    args = parser.parse_args()
    try:
        manifest = prepare(args.release, args.release_id, args.validation_report, args.tokenizer, args.model_template, args.train_template)
        print(json.dumps(manifest, indent=2, sort_keys=True))
        return 0
    except (OSError, ValueError, KeyError, RuntimeError, json.JSONDecodeError) as exc:
        print(json.dumps({"status": "FAILED", "error": str(exc)}, indent=2))
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
