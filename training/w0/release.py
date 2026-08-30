"""Dependency-light W0 release integrity and state helpers."""

from __future__ import annotations

import hashlib
import json
import math
import os
import shutil
import tempfile
from pathlib import Path

RELEASE_SCHEMA_VERSION = 1
STATE_SCHEMA_VERSION = 1
FINAL_STATUS = "W0_TRAINING_COMPLETE_UNVALIDATED"


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def sha256_tree(root: Path) -> str:
    root = root.resolve()
    if not root.is_dir():
        raise RuntimeError(f"artifact directory is missing: {root}")
    files = sorted(path for path in root.rglob("*") if path.is_file())
    if not files:
        raise RuntimeError(f"artifact directory is empty: {root}")
    digest = hashlib.sha256()
    for path in files:
        if path.is_symlink():
            raise RuntimeError(f"artifact may not contain symlinks: {path}")
        relative = path.relative_to(root).as_posix().encode("utf-8")
        digest.update(len(relative).to_bytes(8, "big"))
        digest.update(relative)
        digest.update(bytes.fromhex(sha256(path)))
    return digest.hexdigest()


def atomic_json(path: Path, value: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    descriptor, name = tempfile.mkstemp(prefix=f".{path.name}-", suffix=".tmp", dir=path.parent)
    temporary = Path(name)
    try:
        with os.fdopen(descriptor, "w", encoding="utf-8") as stream:
            json.dump(value, stream, indent=2, sort_keys=True)
            stream.write("\n")
            stream.flush()
            os.fsync(stream.fileno())
        temporary.replace(path)
    except BaseException:
        temporary.unlink(missing_ok=True)
        raise


def path_within(child: Path, parent: Path) -> bool:
    try:
        child.resolve().relative_to(parent.resolve())
        return True
    except ValueError:
        return False


def release_path(release_dir: Path, relative: str) -> Path:
    if not isinstance(relative, str) or not relative or Path(relative).is_absolute():
        raise RuntimeError("release artifact path must be non-empty and relative")
    path = (release_dir / relative).resolve()
    if not path_within(path, release_dir) or (release_dir / relative).is_symlink():
        raise RuntimeError(f"release artifact escapes release directory: {relative}")
    return path


def load_json(path: Path, description: str) -> dict:
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except FileNotFoundError as exc:
        raise RuntimeError(f"{description} is missing: {path}") from exc
    except json.JSONDecodeError as exc:
        raise RuntimeError(f"{description} is invalid JSON: {path}") from exc
    if not isinstance(value, dict):
        raise RuntimeError(f"{description} must be a JSON object")
    return value


def validate_train_config(config: object) -> dict:
    if not isinstance(config, dict):
        raise RuntimeError("W0 train config must be an object")
    if not isinstance(config.get("profile"), str) or not config["profile"].strip():
        raise RuntimeError("W0 train config profile is required")
    positive_ints = (
        "min_available_memory_mb",
        "min_free_disk_mb",
        "memory_poll_seconds",
        "max_sequence_length",
        "epochs",
        "max_optimizer_steps",
        "gradient_accumulation_steps",
        "warmup_optimizer_steps",
        "checkpoint_every_optimizer_steps",
        "torch_threads",
    )
    for name in positive_ints:
        if not isinstance(config.get(name), int) or config[name] < 1:
            raise RuntimeError(f"W0 train config {name} must be a positive integer")
    if config["max_sequence_length"] < 8:
        raise RuntimeError("W0 max_sequence_length must be at least 8")
    if config["warmup_optimizer_steps"] >= config["max_optimizer_steps"]:
        raise RuntimeError("W0 warmup steps must be less than max optimizer steps")
    for name in ("learning_rate", "weight_decay", "max_gradient_norm"):
        value = config.get(name)
        if not isinstance(value, (int, float)) or not math.isfinite(value):
            raise RuntimeError(f"W0 train config {name} must be finite")
    if config["learning_rate"] <= 0 or config["weight_decay"] < 0 or config["max_gradient_norm"] <= 0:
        raise RuntimeError("W0 learning rate and gradient norm must be positive; weight decay may be zero")
    if config.get("torch_dtype") != "float32":
        raise RuntimeError("W0 trainer v1 supports torch_dtype=float32 only")
    if config.get("device") not in ("auto", "cpu", "cuda"):
        raise RuntimeError("W0 device must be auto, cpu, or cuda")
    if not isinstance(config.get("gradient_checkpointing"), bool):
        raise RuntimeError("W0 gradient_checkpointing must be boolean")
    if not isinstance(config.get("seed"), int) or config["seed"] < 0:
        raise RuntimeError("W0 seed must be a non-negative integer")
    return config


def validate_model_config(config: object) -> dict:
    if not isinstance(config, dict) or config.get("model_type") != "llama":
        raise RuntimeError("W0 model config must use the supported llama architecture")
    positive = (
        "vocab_size",
        "hidden_size",
        "intermediate_size",
        "num_hidden_layers",
        "num_attention_heads",
        "num_key_value_heads",
        "max_position_embeddings",
    )
    for name in positive:
        if not isinstance(config.get(name), int) or config[name] < 1:
            raise RuntimeError(f"W0 model config {name} must be a positive integer")
    if config["hidden_size"] % config["num_attention_heads"] != 0:
        raise RuntimeError("W0 hidden_size must be divisible by num_attention_heads")
    if config["num_attention_heads"] % config["num_key_value_heads"] != 0:
        raise RuntimeError("W0 num_attention_heads must be divisible by num_key_value_heads")
    for name in ("pad_token_id", "bos_token_id", "eos_token_id"):
        if not isinstance(config.get(name), int) or not 0 <= config[name] < config["vocab_size"]:
            raise RuntimeError(f"W0 model config {name} is outside the vocabulary")
    return config


def load_release(release_dir: Path, verify_corpus: bool = True) -> dict:
    release_dir = release_dir.resolve()
    manifest_path = release_dir / "w0-release.json"
    manifest = load_json(manifest_path, "W0 release manifest")
    if manifest.get("schema_version") != RELEASE_SCHEMA_VERSION:
        raise RuntimeError("unsupported W0 release schema")
    if not isinstance(manifest.get("release_id"), str) or not manifest["release_id"].strip():
        raise RuntimeError("W0 release_id is required")
    artifacts = manifest.get("artifacts")
    if not isinstance(artifacts, dict):
        raise RuntimeError("W0 release artifacts are required")
    for name in ("tokenizer", "model_config", "train_config"):
        item = artifacts.get(name)
        if not isinstance(item, dict) or not isinstance(item.get("path"), str) or not isinstance(item.get("sha256"), str):
            raise RuntimeError(f"W0 release artifact {name} is invalid")
        path = release_path(release_dir, item["path"])
        actual = sha256_tree(path) if path.is_dir() else sha256(path)
        if actual != item["sha256"]:
            raise RuntimeError(f"W0 release artifact {name} has changed")
    corpus = manifest.get("corpus")
    if not isinstance(corpus, dict):
        raise RuntimeError("W0 release corpus is required")
    report_path = release_path(release_dir, corpus.get("validation_report", ""))
    if not report_path.is_file() or sha256(report_path) != corpus.get("validation_report_sha256"):
        raise RuntimeError("W0 corpus validation report has changed")
    report = load_json(report_path, "W0 validation report")
    if report.get("valid") is not True:
        raise RuntimeError("W0 corpus validation report is not successful")
    shards = corpus.get("shards")
    if not isinstance(shards, list) or not shards:
        raise RuntimeError("W0 release corpus shards are required")
    for item in shards:
        if not isinstance(item, dict) or not isinstance(item.get("path"), str) or not isinstance(item.get("sha256"), str):
            raise RuntimeError("W0 release corpus shard entry is invalid")
        path = release_path(release_dir, item["path"])
        if not path.is_file():
            raise RuntimeError(f"W0 corpus shard is missing: {item['path']}")
        if verify_corpus and sha256(path) != item["sha256"]:
            raise RuntimeError(f"W0 corpus shard has changed: {item['path']}")
    train_path = release_path(release_dir, artifacts["train_config"]["path"])
    validate_train_config(load_json(train_path, "W0 train config"))
    model_path = release_path(release_dir, artifacts["model_config"]["path"])
    validate_model_config(load_json(model_path, "W0 model config"))
    manifest["manifest_sha256"] = sha256(manifest_path)
    return manifest


def disk_available_mb(path: Path) -> int:
    probe = path
    while not probe.exists() and probe != probe.parent:
        probe = probe.parent
    try:
        return shutil.disk_usage(probe).free // 1024**2
    except OSError:
        return 0


def memory_available_mb() -> int:
    try:
        for line in Path("/proc/meminfo").read_text().splitlines():
            if line.startswith("MemAvailable:"):
                return int(line.split()[1]) // 1024
    except (OSError, ValueError, IndexError):
        pass
    return 0
