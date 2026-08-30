#!/usr/bin/env python3
"""RAM-aware, resumable, one-example-at-a-time W1 LoRA trainer."""

from __future__ import annotations

import argparse
import gc
import hashlib
import importlib
import json
import math
import os
import shutil
import sys
import tempfile
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Iterator

PAUSE_EXIT = 75
SCHEMA_VERSION = 1
FINAL_STATUS = "TRAINING_COMPLETE_UNVALIDATED"


@dataclass(frozen=True)
class PreflightIssue:
    message: str
    retryable: bool = False


@dataclass
class PreflightResult:
    issues: list[PreflightIssue]
    manifest: dict | None = None
    dataset_path: Path | None = None
    dataset_sha256: str = ""
    base_model_sha256: str = ""

    @property
    def ready(self) -> bool:
        return not self.issues

    @property
    def retryable(self) -> bool:
        return bool(self.issues) and all(issue.retryable for issue in self.issues)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def sha256_tree(root: Path) -> str:
    """Hash relative paths and contents of an immutable checkpoint directory."""
    root = root.resolve()
    if not root.is_dir():
        raise RuntimeError("base model must be a directory")
    digest = hashlib.sha256()
    files = sorted(path for path in root.rglob("*") if path.is_file())
    if not files:
        raise RuntimeError("base model directory is empty")
    for path in files:
        if path.is_symlink():
            raise RuntimeError(f"base model may not contain symlinks: {path}")
        relative = path.relative_to(root).as_posix().encode("utf-8")
        digest.update(len(relative).to_bytes(8, "big"))
        digest.update(relative)
        digest.update(bytes.fromhex(sha256(path)))
    return digest.hexdigest()


def memory_available_mb() -> int:
    try:
        for line in Path("/proc/meminfo").read_text().splitlines():
            if line.startswith("MemAvailable:"):
                return int(line.split()[1]) // 1024
    except (OSError, ValueError, IndexError):
        pass
    try:
        return int(os.sysconf("SC_AVPHYS_PAGES") * os.sysconf("SC_PAGE_SIZE") // 1024**2)
    except (OSError, ValueError):
        return 0


def disk_available_mb(path: Path) -> int:
    probe = path
    while not probe.exists() and probe != probe.parent:
        probe = probe.parent
    try:
        return shutil.disk_usage(probe).free // 1024**2
    except OSError:
        return 0


def atomic_json(path: Path, value: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary_name = tempfile.mkstemp(prefix=f".{path.name}-", suffix=".tmp", dir=path.parent)
    temporary = Path(temporary_name)
    try:
        with os.fdopen(descriptor, "w") as stream:
            json.dump(value, stream, indent=2, sort_keys=True)
            stream.write("\n")
            stream.flush()
            os.fsync(stream.fileno())
        temporary.replace(path)
    except BaseException:
        temporary.unlink(missing_ok=True)
        raise


def manifest_value(manifest: dict, *names: str):
    for name in names:
        value = manifest.get(name)
        if value not in (None, ""):
            return value
    return None


def expected_w0_sha256(manifest: dict) -> str:
    value = manifest_value(
        manifest,
        "w0_sha256",
        "W0SHA256",
        "base_model_sha256",
        "BaseModelSHA256",
        "foundation_sha256",
        "FoundationSHA256",
        "w0_hash",
        "W0Hash",
    )
    foundation = manifest.get("foundation") or manifest.get("Foundation") or {}
    if not value and isinstance(foundation, dict):
        value = manifest_value(foundation, "artifact_hash", "ArtifactHash", "hash", "Hash")
    if not isinstance(value, str) or len(value) != 64:
        raise RuntimeError("release is not bound to a 64-character W0 SHA-256")
    try:
        bytes.fromhex(value)
    except ValueError as exc:
        raise RuntimeError("release W0 SHA-256 is not hexadecimal") from exc
    return value.lower()


def validate_training_record(item: object, line_number: int) -> dict:
    if not isinstance(item, dict):
        raise RuntimeError(f"training record at line {line_number} must be an object")
    for key in ("instruction", "output"):
        if not isinstance(item.get(key), str) or not item[key].strip():
            raise RuntimeError(f"training record at line {line_number} has no {key}")
    return item


def records(path: Path) -> Iterator[dict]:
    with path.open(encoding="utf-8") as stream:
        for line_number, line in enumerate(stream, 1):
            if not line.strip():
                continue
            try:
                item = json.loads(line)
            except json.JSONDecodeError as exc:
                raise RuntimeError(f"invalid training record at line {line_number}") from exc
            yield validate_training_record(item, line_number)


def load_release(release_dir: Path) -> tuple[dict, Path, str, str]:
    manifest_path = release_dir / "w1-release.json"
    dataset_path = release_dir / "w1-training.jsonl"
    try:
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    except FileNotFoundError as exc:
        raise RuntimeError("release manifest w1-release.json is missing") from exc
    except json.JSONDecodeError as exc:
        raise RuntimeError("release manifest is invalid JSON") from exc
    if not isinstance(manifest, dict):
        raise RuntimeError("release manifest must be an object")
    release_id = manifest_value(manifest, "release_id", "ReleaseID")
    if not isinstance(release_id, str) or not release_id.strip():
        raise RuntimeError("release_id is required")
    expected_training = manifest_value(manifest, "training_sha256", "TrainingSHA256")
    if not dataset_path.is_file() or not isinstance(expected_training, str):
        raise RuntimeError("release training dataset or training_sha256 is missing")
    if len(expected_training) != 64:
        raise RuntimeError("release training_sha256 must contain 64 hexadecimal characters")
    try:
        bytes.fromhex(expected_training)
    except ValueError as exc:
        raise RuntimeError("release training_sha256 is not hexadecimal") from exc
    actual_training = sha256(dataset_path)
    if actual_training != expected_training.lower():
        raise RuntimeError("release training dataset has changed")
    return manifest, dataset_path, actual_training, expected_w0_sha256(manifest)


def validate_config(config: object) -> dict:
    if not isinstance(config, dict):
        raise RuntimeError("trainer config must be an object")
    positive_ints = (
        "min_available_memory_mb",
        "min_free_disk_mb",
        "memory_poll_seconds",
        "restart_wait_seconds",
        "max_sequence_length",
        "epochs",
        "gradient_accumulation_steps",
        "lora_rank",
        "lora_alpha",
        "torch_threads",
        "checkpoint_every_optimizer_steps",
    )
    for name in positive_ints:
        if not isinstance(config.get(name), int) or config[name] < 1:
            raise RuntimeError(f"config {name} must be a positive integer")
    if config["max_sequence_length"] < 8:
        raise RuntimeError("config max_sequence_length must be at least 8")
    for name in ("learning_rate", "lora_dropout"):
        if not isinstance(config.get(name), (int, float)) or not math.isfinite(config[name]):
            raise RuntimeError(f"config {name} must be finite")
    if config["learning_rate"] <= 0:
        raise RuntimeError("config learning_rate must be positive")
    if not 0 <= config["lora_dropout"] < 1:
        raise RuntimeError("config lora_dropout must be in [0,1)")
    modules = config.get("target_modules")
    if not isinstance(modules, list) or not modules or not all(isinstance(x, str) and x for x in modules):
        raise RuntimeError("config target_modules must be a non-empty string list")
    dtype = config.get("torch_dtype", "auto")
    if dtype not in ("auto", "float16", "bfloat16", "float32"):
        raise RuntimeError("config torch_dtype is unsupported")
    if not isinstance(config.get("gradient_checkpointing", True), bool):
        raise RuntimeError("config gradient_checkpointing must be boolean")
    return config


def path_within(child: Path, parent: Path) -> bool:
    try:
        child.resolve().relative_to(parent.resolve())
        return True
    except ValueError:
        return False


def preflight(release: Path, base_model: Path, output: Path, config: dict) -> PreflightResult:
    issues: list[PreflightIssue] = []
    result = PreflightResult(issues=issues)
    try:
        manifest, dataset, dataset_hash, expected_w0 = load_release(release)
        count = sum(1 for _ in records(dataset))
        if count == 0:
            raise RuntimeError("training dataset is empty")
        result.manifest = manifest
        result.dataset_path = dataset
        result.dataset_sha256 = dataset_hash
    except (OSError, ValueError, RuntimeError) as exc:
        issues.append(PreflightIssue(str(exc)))
        expected_w0 = ""

    if not base_model.is_dir():
        issues.append(PreflightIssue("base model must be a local unquantized Transformers checkpoint directory"))
    else:
        required_config = base_model / "config.json"
        weight_files = list(base_model.glob("*.safetensors")) + list(base_model.glob("pytorch_model*.bin"))
        if not required_config.is_file() or not weight_files:
            issues.append(PreflightIssue("base model is missing config.json or unquantized checkpoint tensors"))
        elif expected_w0:
            try:
                result.base_model_sha256 = sha256_tree(base_model)
                if result.base_model_sha256 != expected_w0:
                    issues.append(PreflightIssue("base model SHA-256 does not match the W1 release"))
            except (OSError, RuntimeError) as exc:
                issues.append(PreflightIssue(str(exc)))

    if (
        path_within(output, release)
        or path_within(output, base_model)
        or path_within(release, output)
        or path_within(base_model, output)
    ):
        issues.append(PreflightIssue("output must be isolated from the release and W0 directories"))

    missing = []
    for name in ("torch", "transformers", "peft", "safetensors"):
        try:
            importlib.import_module(name)
        except (ImportError, OSError):
            missing.append(name)
    if missing:
        issues.append(PreflightIssue("missing packages: " + ", ".join(missing)))

    available = memory_available_mb()
    minimum = int(config["min_available_memory_mb"])
    if available and available < minimum:
        issues.append(PreflightIssue(f"available memory {available} MiB is below start threshold {minimum} MiB", retryable=True))
    free_disk = disk_available_mb(output)
    minimum_disk = int(config["min_free_disk_mb"])
    if free_disk and free_disk < minimum_disk:
        issues.append(PreflightIssue(f"free disk {free_disk} MiB is below training threshold {minimum_disk} MiB", retryable=True))
    return result


def initial_state(release_id: str, dataset_hash: str, base_hash: str) -> dict:
    return {
        "schema_version": SCHEMA_VERSION,
        "release_id": release_id,
        "dataset_sha256": dataset_hash,
        "base_model_sha256": base_hash,
        "epoch": 0,
        "record_index": 0,
        "micro_step": 0,
        "pending_micro_steps": 0,
        "optimizer_step": 0,
        "status": "STARTING",
        "checkpoint": "",
        "checkpoint_sha256": "",
    }


def load_state(path: Path, expected: dict, output: Path) -> dict:
    if not path.exists():
        return expected
    try:
        prior = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise RuntimeError("resume state is corrupt") from exc
    if not isinstance(prior, dict) or prior.get("schema_version") != SCHEMA_VERSION:
        raise RuntimeError("resume state schema is invalid")
    for name in ("release_id", "dataset_sha256", "base_model_sha256"):
        if prior.get(name) != expected[name]:
            raise RuntimeError(f"resume state {name} does not match release")
    for name in ("epoch", "record_index", "micro_step", "pending_micro_steps", "optimizer_step"):
        if not isinstance(prior.get(name), int) or prior[name] < 0:
            raise RuntimeError(f"resume state {name} is invalid")
    if prior["pending_micro_steps"] != 0:
        raise RuntimeError("resume state contains gradients that were not checkpointed")
    allowed_statuses = {"STARTING", "RUNNING", "STOPPED", "PAUSED_LOW_MEMORY", FINAL_STATUS}
    if prior.get("status") not in allowed_statuses:
        raise RuntimeError("resume state status is invalid")
    checkpoint = prior.get("checkpoint", "")
    if not isinstance(checkpoint, str):
        raise RuntimeError("resume checkpoint is invalid")
    if checkpoint:
        expected_name = f"checkpoint-{prior['optimizer_step']:08d}"
        if checkpoint != expected_name:
            raise RuntimeError("resume checkpoint does not match optimizer step")
        checkpoint_path = (output / checkpoint).resolve()
        if (output / checkpoint).is_symlink() or not path_within(checkpoint_path, output) or not checkpoint_path.is_dir():
            raise RuntimeError("resume checkpoint is missing or outside output")
        checkpoint_hash = prior.get("checkpoint_sha256")
        if not isinstance(checkpoint_hash, str) or sha256_tree(checkpoint_path) != checkpoint_hash:
            raise RuntimeError("resume checkpoint hash does not match state")
    elif prior["optimizer_step"] != 0:
        raise RuntimeError("resume state has optimizer progress without a checkpoint")
    return prior


def save_checkpoint(model, optimizer, output: Path, state: dict) -> None:
    import torch

    name = f"checkpoint-{state['optimizer_step']:08d}"
    checkpoint = output / name
    if checkpoint.exists():
        if state.get("checkpoint") != name:
            raise RuntimeError(f"checkpoint already exists: {checkpoint}")
        atomic_json(output / "state.json", state)
        return
    temporary = Path(tempfile.mkdtemp(prefix=f".{name}-", dir=output))
    try:
        model.save_pretrained(temporary, safe_serialization=True)
        torch.save(optimizer.state_dict(), temporary / "optimizer.pt")
        temporary.replace(checkpoint)
    except BaseException:
        shutil.rmtree(temporary, ignore_errors=True)
        raise
    state["checkpoint"] = name
    state["checkpoint_sha256"] = sha256_tree(checkpoint)
    atomic_json(output / "state.json", state)


def flush_optimizer_step(model, optimizer, state: dict, accumulation: int) -> bool:
    import torch

    pending = int(state["pending_micro_steps"])
    if pending == 0:
        return False
    if pending < accumulation:
        correction = accumulation / pending
        for parameter in model.parameters():
            if parameter.grad is not None:
                parameter.grad.mul_(correction)
    torch.nn.utils.clip_grad_norm_(model.parameters(), 1.0)
    optimizer.step()
    optimizer.zero_grad(set_to_none=True)
    state["optimizer_step"] += 1
    state["pending_micro_steps"] = 0
    return True


def encoded_example(tokenizer, item: dict, maximum: int, torch):
    prompt = f"Instruction:\n{item['instruction']}\n\nResponse:\n"
    prompt_ids = tokenizer(prompt, add_special_tokens=False)["input_ids"]
    response_ids = tokenizer(item["output"], add_special_tokens=False)["input_ids"]
    eos_token_id = tokenizer.eos_token_id
    if eos_token_id is None:
        raise RuntimeError("tokenizer has no eos_token_id")
    if not response_ids:
        raise RuntimeError("tokenizer produced an empty response")
    # Reserve at least one prompt token and always retain EOS.
    response_ids = response_ids[: maximum - 2] + [eos_token_id]
    prompt_budget = maximum - len(response_ids)
    prompt_ids = prompt_ids[-prompt_budget:] if prompt_budget > 0 else []
    input_ids = prompt_ids + response_ids
    labels = [-100] * len(prompt_ids) + response_ids
    return {
        "input_ids": torch.tensor([input_ids], dtype=torch.long),
        "attention_mask": torch.ones((1, len(input_ids)), dtype=torch.long),
        "labels": torch.tensor([labels], dtype=torch.long),
    }


def train(release_dir: Path, base_model: Path, output: Path, config: dict) -> int:
    checked = preflight(release_dir, base_model, output, config)
    if not checked.ready:
        print(
            json.dumps(
                {"ready": False, "retryable": checked.retryable, "errors": [x.message for x in checked.issues]},
                indent=2,
            ),
            file=sys.stderr,
        )
        return PAUSE_EXIT if checked.retryable else 2

    import torch
    from peft import LoraConfig, PeftModel, TaskType, get_peft_model
    from transformers import AutoModelForCausalLM, AutoTokenizer

    assert checked.manifest is not None and checked.dataset_path is not None
    release_id = str(manifest_value(checked.manifest, "release_id", "ReleaseID"))
    torch.set_num_threads(int(config["torch_threads"]))
    output.mkdir(parents=True, exist_ok=True)
    state_path = output / "state.json"
    state = load_state(
        state_path,
        initial_state(release_id, checked.dataset_sha256, checked.base_model_sha256),
        output,
    )
    if state["status"] == FINAL_STATUS:
        raise RuntimeError("training output is already complete and remains unvalidated")

    tokenizer = AutoTokenizer.from_pretrained(base_model, local_files_only=True, trust_remote_code=False)
    if tokenizer.pad_token is None:
        if tokenizer.eos_token is None:
            raise RuntimeError("base tokenizer has neither pad_token nor eos_token")
        tokenizer.pad_token = tokenizer.eos_token
    requested_dtype = config.get("torch_dtype", "auto")
    dtype = "auto" if requested_dtype == "auto" else getattr(torch, requested_dtype)
    model = AutoModelForCausalLM.from_pretrained(
        base_model,
        local_files_only=True,
        trust_remote_code=False,
        low_cpu_mem_usage=True,
        torch_dtype=dtype,
    )
    if bool(config.get("gradient_checkpointing", True)):
        model.gradient_checkpointing_enable()
        model.config.use_cache = False
    if state["checkpoint"]:
        model = PeftModel.from_pretrained(model, output / state["checkpoint"], is_trainable=True)
    else:
        model = get_peft_model(
            model,
            LoraConfig(
                task_type=TaskType.CAUSAL_LM,
                r=int(config["lora_rank"]),
                lora_alpha=int(config["lora_alpha"]),
                lora_dropout=float(config["lora_dropout"]),
                target_modules=list(config["target_modules"]),
            ),
        )
    model.enable_input_require_grads()
    model.train()
    optimizer = torch.optim.AdamW(
        (parameter for parameter in model.parameters() if parameter.requires_grad),
        lr=float(config["learning_rate"]),
    )
    if state["checkpoint"]:
        optimizer_path = output / state["checkpoint"] / "optimizer.pt"
        if not optimizer_path.is_file():
            raise RuntimeError("resume checkpoint has no optimizer state")
        optimizer.load_state_dict(torch.load(optimizer_path, map_location="cpu", weights_only=True))

    accumulation = int(config["gradient_accumulation_steps"])
    maximum = int(config["max_sequence_length"])
    checkpoint_interval = int(config["checkpoint_every_optimizer_steps"])
    memory_poll_seconds = int(config["memory_poll_seconds"])
    optimizer.zero_grad(set_to_none=True)
    state["status"] = "RUNNING"
    atomic_json(state_path, state)
    device = next(model.parameters()).device
    next_memory_poll = 0.0

    for epoch in range(int(state["epoch"]), int(config["epochs"])):
        start_index = int(state["record_index"]) if epoch == int(state["epoch"]) else 0
        for index, item in enumerate(records(checked.dataset_path)):
            if index < start_index:
                continue
            if (output / "STOP").exists():
                flush_optimizer_step(model, optimizer, state, accumulation)
                state["status"] = "STOPPED"
                save_checkpoint(model, optimizer, output, state)
                return 0
            now = time.monotonic()
            if now >= next_memory_poll:
                available = memory_available_mb()
                next_memory_poll = now + memory_poll_seconds
                if available and available < int(config["min_available_memory_mb"]):
                    flush_optimizer_step(model, optimizer, state, accumulation)
                    state["status"] = "PAUSED_LOW_MEMORY"
                    state["available_memory_mb"] = available
                    save_checkpoint(model, optimizer, output, state)
                    return PAUSE_EXIT

            encoded = encoded_example(tokenizer, item, maximum, torch)
            encoded = {key: value.to(device) for key, value in encoded.items()}
            loss = model(**encoded).loss / accumulation
            loss.backward()
            state["micro_step"] += 1
            state["pending_micro_steps"] += 1
            state["record_index"] = index + 1
            state["last_loss"] = float(loss.detach().cpu()) * accumulation
            if state["pending_micro_steps"] == accumulation:
                flush_optimizer_step(model, optimizer, state, accumulation)
                if state["optimizer_step"] % checkpoint_interval == 0:
                    save_checkpoint(model, optimizer, output, state)
            del encoded, loss
            gc.collect()
            if torch.cuda.is_available():
                torch.cuda.empty_cache()

        if flush_optimizer_step(model, optimizer, state, accumulation):
            if state["optimizer_step"] % checkpoint_interval == 0:
                save_checkpoint(model, optimizer, output, state)
        state["epoch"] = epoch + 1
        state["record_index"] = 0

    if sha256_tree(base_model) != checked.base_model_sha256:
        raise RuntimeError("W0 changed during training")
    final_dir = output / "adapter-final"
    if final_dir.exists():
        raise RuntimeError("adapter-final already exists; use an isolated output directory")
    temporary_final = Path(tempfile.mkdtemp(prefix=".adapter-final-", dir=output))
    try:
        model.save_pretrained(temporary_final, safe_serialization=True)
        tokenizer.save_pretrained(temporary_final)
        temporary_final.replace(final_dir)
    except BaseException:
        shutil.rmtree(temporary_final, ignore_errors=True)
        raise
    state["status"] = FINAL_STATUS
    state["completed_at"] = time.time()
    state["adapter"] = "adapter-final"
    state["adapter_sha256"] = sha256_tree(final_dir)
    atomic_json(state_path, state)
    return 0


def preflight_json(result: PreflightResult, output: Path) -> dict:
    return {
        "ready": result.ready,
        "retryable": result.retryable,
        "errors": [issue.message for issue in result.issues],
        "available_memory_mb": memory_available_mb(),
        "free_disk_mb": disk_available_mb(output),
        "dataset_sha256": result.dataset_sha256 or None,
        "base_model_sha256": result.base_model_sha256 or None,
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--release", type=Path)
    parser.add_argument("--base-model", type=Path)
    parser.add_argument("--output", type=Path)
    parser.add_argument("--config", type=Path, default=Path(__file__).with_name("config.json"))
    parser.add_argument("--preflight", action="store_true")
    parser.add_argument("--hash-base-model", action="store_true")
    args = parser.parse_args()
    if args.hash_base_model:
        if args.base_model is None:
            parser.error("--hash-base-model requires --base-model")
        print(sha256_tree(args.base_model))
        return 0
    if args.release is None or args.base_model is None or args.output is None:
        parser.error("--release, --base-model, and --output are required")
    try:
        config = validate_config(json.loads(args.config.read_text(encoding="utf-8")))
        if args.preflight:
            result = preflight(args.release, args.base_model, args.output, config)
            print(json.dumps(preflight_json(result, args.output), indent=2))
            return 0 if result.ready else 2
        return train(args.release, args.base_model, args.output, config)
    except (OSError, ValueError, RuntimeError, json.JSONDecodeError) as exc:
        print(json.dumps({"status": "FAILED", "error": str(exc)}, indent=2), file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
