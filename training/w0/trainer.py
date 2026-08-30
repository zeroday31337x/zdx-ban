#!/usr/bin/env python3
"""Resumable full-weight causal-language-model trainer for an immutable W0 release."""

from __future__ import annotations

import argparse
import gc
import importlib
import json
import random
import shutil
import sys
import tempfile
import time
from pathlib import Path

from release import (
    FINAL_STATUS,
    STATE_SCHEMA_VERSION,
    atomic_json,
    disk_available_mb,
    load_json,
    load_release,
    memory_available_mb,
    path_within,
    release_path,
    sha256_tree,
    validate_train_config,
)

PAUSE_EXIT = 75


def initial_state(manifest: dict) -> dict:
    return {
        "schema_version": STATE_SCHEMA_VERSION,
        "release_id": manifest["release_id"],
        "release_manifest_sha256": manifest["manifest_sha256"],
        "status": "STARTING",
        "optimizer_step": 0,
        "micro_step": 0,
        "pending_micro_steps": 0,
        "last_loss": None,
        "checkpoint": "",
        "checkpoint_sha256": "",
        "cursor": {"epoch": 0, "shard_index": 0, "byte_offset": 0, "pending_tokens": []},
    }


def validate_cursor(cursor: object, shard_count: int) -> dict:
    if not isinstance(cursor, dict):
        raise RuntimeError("W0 resume cursor is invalid")
    for name in ("epoch", "shard_index", "byte_offset"):
        if not isinstance(cursor.get(name), int) or cursor[name] < 0:
            raise RuntimeError(f"W0 resume cursor {name} is invalid")
    if cursor["shard_index"] > shard_count:
        raise RuntimeError("W0 resume cursor shard_index is invalid")
    tokens = cursor.get("pending_tokens")
    if not isinstance(tokens, list) or not all(isinstance(value, int) and value >= 0 for value in tokens):
        raise RuntimeError("W0 resume pending token buffer is invalid")
    return cursor


def load_state(path: Path, expected: dict, output: Path, shard_count: int) -> dict:
    if not path.exists():
        return expected
    state = load_json(path, "W0 resume state")
    if state.get("schema_version") != STATE_SCHEMA_VERSION:
        raise RuntimeError("W0 resume state schema is invalid")
    for name in ("release_id", "release_manifest_sha256"):
        if state.get(name) != expected[name]:
            raise RuntimeError(f"W0 resume state {name} does not match release")
    for name in ("optimizer_step", "micro_step", "pending_micro_steps"):
        if not isinstance(state.get(name), int) or state[name] < 0:
            raise RuntimeError(f"W0 resume state {name} is invalid")
    if state["pending_micro_steps"] != 0:
        raise RuntimeError("W0 resume state contains uncheckpointed gradients")
    allowed = {"STARTING", "RUNNING", "STOPPED", "PAUSED_LOW_MEMORY", FINAL_STATUS}
    if state.get("status") not in allowed:
        raise RuntimeError("W0 resume state status is invalid")
    validate_cursor(state.get("cursor"), shard_count)
    checkpoint = state.get("checkpoint")
    if not isinstance(checkpoint, str):
        raise RuntimeError("W0 resume checkpoint is invalid")
    if checkpoint:
        expected_name = f"checkpoint-{state['optimizer_step']:08d}"
        if checkpoint != expected_name:
            raise RuntimeError("W0 resume checkpoint does not match optimizer step")
        checkpoint_path = (output / checkpoint).resolve()
        if not path_within(checkpoint_path, output) or not checkpoint_path.is_dir() or (output / checkpoint).is_symlink():
            raise RuntimeError("W0 resume checkpoint is missing or outside output")
        if sha256_tree(checkpoint_path) != state.get("checkpoint_sha256"):
            raise RuntimeError("W0 resume checkpoint hash does not match state")
    elif state["optimizer_step"] != 0:
        raise RuntimeError("W0 resume state has optimizer progress without a checkpoint")
    return state


def next_block(release_dir: Path, shards: list[dict], tokenizer, cursor: dict, sequence_length: int, epochs: int):
    pending = cursor["pending_tokens"]
    pad = tokenizer.pad_token_id
    eos = tokenizer.eos_token_id
    if pad is None or eos is None:
        raise RuntimeError("W0 tokenizer requires pad_token_id and eos_token_id")
    while len(pending) < sequence_length + 1:
        if cursor["epoch"] >= epochs:
            return None
        if cursor["shard_index"] >= len(shards):
            if len(pending) >= 2:
                block = pending[:]
                pending.clear()
                cursor["epoch"] += 1
                cursor["shard_index"] = 0
                cursor["byte_offset"] = 0
                block.extend([pad] * (sequence_length + 1 - len(block)))
                return block[: sequence_length + 1]
            pending.clear()
            cursor["epoch"] += 1
            cursor["shard_index"] = 0
            cursor["byte_offset"] = 0
            continue
        shard = release_path(release_dir, shards[cursor["shard_index"]]["path"])
        with shard.open("rb") as stream:
            stream.seek(cursor["byte_offset"])
            line = stream.readline()
            cursor["byte_offset"] = stream.tell()
        if not line:
            cursor["shard_index"] += 1
            cursor["byte_offset"] = 0
            continue
        if not line.strip():
            continue
        try:
            item = json.loads(line.decode("utf-8"))
        except (UnicodeDecodeError, json.JSONDecodeError) as exc:
            raise RuntimeError(f"invalid W0 corpus record in {shard}") from exc
        if item.get("split") != "train":
            continue
        text = item.get("text")
        if not isinstance(text, str) or not text:
            raise RuntimeError(f"W0 corpus record has no text in {shard}")
        token_ids = tokenizer(text, add_special_tokens=False)["input_ids"]
        if token_ids:
            pending.extend(int(value) for value in token_ids)
            pending.append(int(eos))
    block = pending[: sequence_length + 1]
    del pending[:sequence_length]
    return block


def preflight(release_dir: Path, output: Path) -> tuple[list[dict], dict | None, dict | None]:
    issues: list[dict] = []
    manifest = None
    config = None
    try:
        manifest = load_release(release_dir, verify_corpus=True)
        train_item = manifest["artifacts"]["train_config"]
        config = validate_train_config(load_json(release_path(release_dir, train_item["path"]), "W0 train config"))
        report = load_json(release_dir / "validation-report.json", "W0 validation report")
        if report.get("valid") is not True or report.get("by_split", {}).get("train", 0) < 1:
            raise RuntimeError("W0 corpus has no validated training records")
    except (OSError, RuntimeError, ValueError, KeyError) as exc:
        issues.append({"message": str(exc), "retryable": False})
    if path_within(output, release_dir) or path_within(release_dir, output):
        issues.append({"message": "W0 output must be isolated from its immutable release", "retryable": False})
    missing = []
    for name in ("torch", "transformers", "safetensors", "tokenizers"):
        try:
            importlib.import_module(name)
        except (ImportError, OSError):
            missing.append(name)
    if missing:
        issues.append({"message": "missing packages: " + ", ".join(missing), "retryable": False})
    if config:
        available = memory_available_mb()
        if available and available < int(config["min_available_memory_mb"]):
            issues.append({"message": f"available memory {available} MiB is below W0 threshold {config['min_available_memory_mb']} MiB", "retryable": True})
        disk = disk_available_mb(output)
        if disk and disk < int(config["min_free_disk_mb"]):
            issues.append({"message": f"free disk {disk} MiB is below W0 threshold {config['min_free_disk_mb']} MiB", "retryable": True})
    return issues, manifest, config


def save_checkpoint(model, optimizer, scheduler, torch, output: Path, state: dict) -> None:
    name = f"checkpoint-{state['optimizer_step']:08d}"
    checkpoint = output / name
    if checkpoint.exists():
        if state.get("checkpoint") != name:
            raise RuntimeError(f"W0 checkpoint already exists: {checkpoint}")
        atomic_json(output / "state.json", state)
        return
    temporary = Path(tempfile.mkdtemp(prefix=f".{name}-", dir=output))
    try:
        model.save_pretrained(temporary / "model", safe_serialization=True)
        torch.save(optimizer.state_dict(), temporary / "optimizer.pt")
        torch.save(scheduler.state_dict(), temporary / "scheduler.pt")
        rng = {"python": random.getstate(), "torch": torch.get_rng_state()}
        if torch.cuda.is_available():
            rng["cuda"] = torch.cuda.get_rng_state_all()
        torch.save(rng, temporary / "rng.pt")
        atomic_json(temporary / "checkpoint-state.json", state)
        temporary.replace(checkpoint)
    except BaseException:
        shutil.rmtree(temporary, ignore_errors=True)
        raise
    state["checkpoint"] = name
    state["checkpoint_sha256"] = sha256_tree(checkpoint)
    atomic_json(output / "state.json", state)


def flush_step(model, optimizer, scheduler, torch, state: dict, accumulation: int, max_norm: float) -> bool:
    pending = state["pending_micro_steps"]
    if pending == 0:
        return False
    if pending < accumulation:
        correction = accumulation / pending
        for parameter in model.parameters():
            if parameter.grad is not None:
                parameter.grad.mul_(correction)
    torch.nn.utils.clip_grad_norm_(model.parameters(), max_norm)
    optimizer.step()
    scheduler.step()
    optimizer.zero_grad(set_to_none=True)
    state["optimizer_step"] += 1
    state["pending_micro_steps"] = 0
    return True


def restore_rng(torch, checkpoint: Path) -> None:
    rng = torch.load(checkpoint / "rng.pt", map_location="cpu", weights_only=False)
    random.setstate(rng["python"])
    torch.set_rng_state(rng["torch"])
    if torch.cuda.is_available() and "cuda" in rng:
        torch.cuda.set_rng_state_all(rng["cuda"])


def train(release_dir: Path, output: Path) -> int:
    issues, manifest, config = preflight(release_dir, output)
    if issues:
        retryable = bool(issues) and all(issue["retryable"] for issue in issues)
        print(json.dumps({"ready": False, "retryable": retryable, "errors": [issue["message"] for issue in issues]}, indent=2), file=sys.stderr)
        return PAUSE_EXIT if retryable else 2
    assert manifest is not None and config is not None
    import torch
    from transformers import CONFIG_MAPPING, AutoModelForCausalLM, AutoTokenizer, get_cosine_schedule_with_warmup

    torch.set_num_threads(int(config["torch_threads"]))
    seed = int(config["seed"])
    random.seed(seed)
    torch.manual_seed(seed)
    if torch.cuda.is_available():
        torch.cuda.manual_seed_all(seed)
    requested_device = config["device"]
    if requested_device == "cuda" and not torch.cuda.is_available():
        raise RuntimeError("W0 train config requires CUDA but CUDA is unavailable")
    device = "cuda" if requested_device == "auto" and torch.cuda.is_available() else requested_device
    if device == "auto":
        device = "cpu"
    release_dir = release_dir.resolve()
    output.mkdir(parents=True, exist_ok=True)
    state_path = output / "state.json"
    state = load_state(state_path, initial_state(manifest), output, len(manifest["corpus"]["shards"]))
    if state["status"] == FINAL_STATUS:
        raise RuntimeError("W0 output is already complete and remains unvalidated")
    tokenizer_path = release_path(release_dir, manifest["artifacts"]["tokenizer"]["path"])
    tokenizer = AutoTokenizer.from_pretrained(tokenizer_path, local_files_only=True, trust_remote_code=False)
    model_config_path = release_path(release_dir, manifest["artifacts"]["model_config"]["path"])
    if state["checkpoint"]:
        checkpoint = output / state["checkpoint"]
        model = AutoModelForCausalLM.from_pretrained(checkpoint / "model", local_files_only=True, trust_remote_code=False)
    else:
        model_config_data = load_json(model_config_path, "W0 model config")
        model_type = model_config_data.pop("model_type", None)
        if not isinstance(model_type, str) or model_type not in CONFIG_MAPPING:
            raise RuntimeError("W0 model_type is unsupported by installed Transformers")
        model_config = CONFIG_MAPPING[model_type](**model_config_data)
        model = AutoModelForCausalLM.from_config(model_config, trust_remote_code=False)
    if int(config["max_sequence_length"]) > int(model.config.max_position_embeddings):
        raise RuntimeError("W0 sequence length exceeds model max_position_embeddings")
    if bool(config["gradient_checkpointing"]):
        model.gradient_checkpointing_enable()
        model.config.use_cache = False
    model.to(device=device, dtype=torch.float32)
    model.train()
    optimizer = torch.optim.AdamW(model.parameters(), lr=float(config["learning_rate"]), weight_decay=float(config["weight_decay"]))
    scheduler = get_cosine_schedule_with_warmup(
        optimizer,
        num_warmup_steps=int(config["warmup_optimizer_steps"]),
        num_training_steps=int(config["max_optimizer_steps"]),
    )
    if state["checkpoint"]:
        checkpoint = output / state["checkpoint"]
        optimizer.load_state_dict(torch.load(checkpoint / "optimizer.pt", map_location=device, weights_only=True))
        scheduler.load_state_dict(torch.load(checkpoint / "scheduler.pt", map_location="cpu", weights_only=True))
        restore_rng(torch, checkpoint)
    accumulation = int(config["gradient_accumulation_steps"])
    maximum = int(config["max_sequence_length"])
    checkpoint_interval = int(config["checkpoint_every_optimizer_steps"])
    max_steps = int(config["max_optimizer_steps"])
    max_norm = float(config["max_gradient_norm"])
    next_memory_poll = 0.0
    optimizer.zero_grad(set_to_none=True)
    state["status"] = "RUNNING"
    atomic_json(state_path, state)
    while state["optimizer_step"] < max_steps:
        if (output / "STOP").exists():
            flush_step(model, optimizer, scheduler, torch, state, accumulation, max_norm)
            state["status"] = "STOPPED"
            save_checkpoint(model, optimizer, scheduler, torch, output, state)
            return 0
        now = time.monotonic()
        if now >= next_memory_poll:
            available = memory_available_mb()
            next_memory_poll = now + int(config["memory_poll_seconds"])
            if available and available < int(config["min_available_memory_mb"]):
                flush_step(model, optimizer, scheduler, torch, state, accumulation, max_norm)
                state["status"] = "PAUSED_LOW_MEMORY"
                state["available_memory_mb"] = available
                save_checkpoint(model, optimizer, scheduler, torch, output, state)
                return PAUSE_EXIT
        block = next_block(release_dir, manifest["corpus"]["shards"], tokenizer, state["cursor"], maximum, int(config["epochs"]))
        if block is None:
            break
        inputs = torch.tensor([block[:-1]], dtype=torch.long, device=device)
        labels = torch.tensor([block[1:]], dtype=torch.long, device=device)
        attention = inputs.ne(tokenizer.pad_token_id).long()
        labels[labels == tokenizer.pad_token_id] = -100
        loss = model(input_ids=inputs, attention_mask=attention, labels=labels).loss / accumulation
        loss.backward()
        state["micro_step"] += 1
        state["pending_micro_steps"] += 1
        state["last_loss"] = float(loss.detach().cpu()) * accumulation
        if state["pending_micro_steps"] == accumulation:
            flush_step(model, optimizer, scheduler, torch, state, accumulation, max_norm)
            if state["optimizer_step"] % checkpoint_interval == 0:
                save_checkpoint(model, optimizer, scheduler, torch, output, state)
        del inputs, labels, attention, loss
        gc.collect()
        if torch.cuda.is_available():
            torch.cuda.empty_cache()
    if flush_step(model, optimizer, scheduler, torch, state, accumulation, max_norm):
        save_checkpoint(model, optimizer, scheduler, torch, output, state)
    if state["micro_step"] == 0:
        raise RuntimeError("W0 corpus produced no trainable token blocks")
    load_release(release_dir, verify_corpus=True)
    final = output / "model-final"
    if final.exists():
        raise RuntimeError("W0 model-final already exists; use an isolated output directory")
    temporary = Path(tempfile.mkdtemp(prefix=".model-final-", dir=output))
    try:
        model.config.use_cache = True
        model.save_pretrained(temporary, safe_serialization=True)
        tokenizer.save_pretrained(temporary)
        temporary.replace(final)
    except BaseException:
        shutil.rmtree(temporary, ignore_errors=True)
        raise
    state["status"] = FINAL_STATUS
    state["completed_at"] = time.time()
    state["model"] = "model-final"
    state["model_sha256"] = sha256_tree(final)
    atomic_json(state_path, state)
    return 0


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--release", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--preflight", action="store_true")
    args = parser.parse_args()
    try:
        if args.preflight:
            issues, manifest, config = preflight(args.release, args.output)
            print(
                json.dumps(
                    {
                        "ready": not issues,
                        "retryable": bool(issues) and all(issue["retryable"] for issue in issues),
                        "errors": [issue["message"] for issue in issues],
                        "release_id": manifest.get("release_id") if manifest else None,
                        "profile": config.get("profile") if config else None,
                        "available_memory_mb": memory_available_mb(),
                        "free_disk_mb": disk_available_mb(args.output),
                    },
                    indent=2,
                )
            )
            return 0 if not issues else 2
        return train(args.release, args.output)
    except (OSError, ValueError, KeyError, RuntimeError, json.JSONDecodeError) as exc:
        print(json.dumps({"status": "FAILED", "error": str(exc)}, indent=2), file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
