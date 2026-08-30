#!/usr/bin/env python3
"""Streaming, dependency-free validation for W0 pretraining JSONL shards."""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import os
import sqlite3
import tempfile
import unicodedata
from collections import Counter
from pathlib import Path
from typing import Iterable

SCHEMA_VERSION = 1
SPLITS = {"train", "validation", "test"}
REQUIRED_STRINGS = ("id", "text", "source", "source_type", "language", "license", "split", "content_sha256")


def canonical_text(text: str) -> str:
    """Return the exact normalization used for content hashes and training."""
    return unicodedata.normalize("NFC", text.replace("\r\n", "\n").replace("\r", "\n"))


def content_sha256(text: str) -> str:
    return hashlib.sha256(canonical_text(text).encode("utf-8")).hexdigest()


def is_sha256(value: object) -> bool:
    if not isinstance(value, str) or len(value) != 64 or value != value.lower():
        return False
    try:
        bytes.fromhex(value)
    except ValueError:
        return False
    return True


def input_files(inputs: Iterable[Path]) -> list[Path]:
    files: list[Path] = []
    for item in inputs:
        if item.is_dir():
            files.extend(sorted(path for path in item.rglob("*.jsonl") if path.is_file()))
        elif item.is_file():
            files.append(item)
        else:
            raise RuntimeError(f"dataset input does not exist: {item}")
    unique: list[Path] = []
    seen: set[Path] = set()
    for path in files:
        resolved = path.resolve()
        if resolved not in seen:
            seen.add(resolved)
            unique.append(resolved)
    if not unique:
        raise RuntimeError("no JSONL dataset shards found")
    return unique


def load_deny_hashes(path: Path | None) -> set[str]:
    if path is None:
        return set()
    hashes: set[str] = set()
    with path.open(encoding="utf-8") as stream:
        for line_number, line in enumerate(stream, 1):
            value = line.strip().lower()
            if not value or value.startswith("#"):
                continue
            if not is_sha256(value):
                raise RuntimeError(f"invalid denied content hash at line {line_number}")
            hashes.add(value)
    return hashes


def validate_record(item: object, min_chars: int, max_bytes: int) -> tuple[dict | None, list[str]]:
    errors: list[str] = []
    if not isinstance(item, dict):
        return None, ["record must be a JSON object"]
    for field in REQUIRED_STRINGS:
        value = item.get(field)
        if not isinstance(value, str) or not value.strip():
            errors.append(f"{field} must be a non-empty string")
    if not isinstance(item.get("synthetic"), bool):
        errors.append("synthetic must be a boolean")
    if errors:
        return item, errors

    text = item["text"]
    normalized = canonical_text(text)
    encoded = normalized.encode("utf-8")
    if text != normalized:
        errors.append("text is not canonical NFC with LF newlines")
    if len(normalized) < min_chars:
        errors.append(f"text is shorter than {min_chars} characters")
    if len(encoded) > max_bytes:
        errors.append(f"text exceeds {max_bytes} UTF-8 bytes")
    if "\x00" in normalized:
        errors.append("text contains a NUL character")
    if item["split"] not in SPLITS:
        errors.append("split must be train, validation, or test")
    expected_hash = content_sha256(normalized)
    if not is_sha256(item["content_sha256"]):
        errors.append("content_sha256 must be lowercase 64-character hexadecimal")
    elif item["content_sha256"] != expected_hash:
        errors.append("content_sha256 does not match canonical text")
    if item["synthetic"]:
        if not isinstance(item.get("generator"), str) or not item["generator"].strip():
            errors.append("synthetic record has no generator")
        if not is_sha256(item.get("generation_prompt_hash")):
            errors.append("synthetic record has no valid generation_prompt_hash")
    return item, errors


def validate(
    files: list[Path],
    min_chars: int,
    max_bytes: int,
    deny_hashes: set[str],
    index_path: Path,
    max_errors: int,
) -> dict:
    report: dict = {
        "schema_version": SCHEMA_VERSION,
        "valid": True,
        "files": [],
        "records": 0,
        "accepted_records": 0,
        "invalid_records": 0,
        "blank_lines": 0,
        "utf8_bytes": 0,
        "characters": 0,
        "approximate_tokens_chars_div_4": 0,
        "synthetic_records": 0,
        "duplicate_ids": 0,
        "duplicate_contents": 0,
        "denied_contents": 0,
        "by_split": {},
        "by_source_type": {},
        "errors": [],
    }
    split_counts: Counter[str] = Counter()
    source_counts: Counter[str] = Counter()
    connection = sqlite3.connect(index_path)
    connection.execute("PRAGMA journal_mode=WAL")
    connection.execute("PRAGMA synchronous=NORMAL")
    connection.execute("CREATE TABLE ids (value TEXT PRIMARY KEY) WITHOUT ROWID")
    connection.execute("CREATE TABLE contents (value TEXT PRIMARY KEY, split TEXT NOT NULL) WITHOUT ROWID")

    def add_error(path: Path, line_number: int, message: str) -> None:
        report["invalid_records"] += 1
        if len(report["errors"]) < max_errors:
            report["errors"].append({"file": str(path), "line": line_number, "error": message})

    try:
        for path in files:
            shard_hash = hashlib.sha256()
            shard_records = 0
            with path.open("rb") as raw:
                for line_number, line in enumerate(raw, 1):
                    shard_hash.update(line)
                    if not line.strip():
                        report["blank_lines"] += 1
                        continue
                    report["records"] += 1
                    shard_records += 1
                    try:
                        decoded = line.decode("utf-8")
                    except UnicodeDecodeError as exc:
                        add_error(path, line_number, f"invalid UTF-8: {exc}")
                        continue
                    try:
                        item = json.loads(decoded)
                    except json.JSONDecodeError as exc:
                        add_error(path, line_number, f"invalid JSON: {exc.msg}")
                        continue
                    item, errors = validate_record(item, min_chars, max_bytes)
                    if errors or item is None:
                        add_error(path, line_number, "; ".join(errors))
                        continue
                    text = canonical_text(item["text"])
                    encoded = text.encode("utf-8")
                    record_invalid = False
                    try:
                        connection.execute("INSERT INTO ids(value) VALUES (?)", (item["id"],))
                    except sqlite3.IntegrityError:
                        report["duplicate_ids"] += 1
                        add_error(path, line_number, "duplicate id")
                        record_invalid = True
                    try:
                        connection.execute(
                            "INSERT INTO contents(value, split) VALUES (?, ?)",
                            (item["content_sha256"], item["split"]),
                        )
                    except sqlite3.IntegrityError:
                        report["duplicate_contents"] += 1
                        if not record_invalid:
                            add_error(path, line_number, "duplicate canonical content")
                            record_invalid = True
                    if item["content_sha256"] in deny_hashes:
                        report["denied_contents"] += 1
                        if not record_invalid:
                            add_error(path, line_number, "content matches the denied/evaluation hash set")
                        record_invalid = True
                    if not record_invalid:
                        report["accepted_records"] += 1
                        report["utf8_bytes"] += len(encoded)
                        report["characters"] += len(text)
                        split_counts[item["split"]] += 1
                        source_counts[item["source_type"]] += 1
                        report["synthetic_records"] += int(item["synthetic"])
                    if report["records"] % 10000 == 0:
                        connection.commit()
            report["files"].append(
                {"path": str(path), "sha256": shard_hash.hexdigest(), "records": shard_records}
            )
        connection.commit()
    finally:
        connection.close()
    report["approximate_tokens_chars_div_4"] = math.ceil(report["characters"] / 4)
    report["by_split"] = dict(sorted(split_counts.items()))
    report["by_source_type"] = dict(sorted(source_counts.items()))
    report["synthetic_fraction"] = (
        report["synthetic_records"] / report["accepted_records"]
        if report["accepted_records"]
        else 0.0
    )
    report["valid"] = report["records"] > 0 and report["invalid_records"] == 0
    if not report["records"]:
        report["errors"].append({"file": "", "line": 0, "error": "dataset has no records"})
    return report


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


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("inputs", nargs="+", type=Path, help="JSONL shards or directories")
    parser.add_argument("--deny-hashes", type=Path, help="newline-delimited evaluation content hashes")
    parser.add_argument("--index", type=Path, help="SQLite duplicate index (must not already exist)")
    parser.add_argument("--report", type=Path, help="write the complete validation report atomically")
    parser.add_argument("--min-chars", type=int, default=200)
    parser.add_argument("--max-document-bytes", type=int, default=4 * 1024 * 1024)
    parser.add_argument("--max-errors", type=int, default=100)
    args = parser.parse_args()
    if args.min_chars < 1 or args.max_document_bytes < 1 or args.max_errors < 1:
        parser.error("size and error limits must be positive")
    try:
        files = input_files(args.inputs)
        denied = load_deny_hashes(args.deny_hashes)
        if args.index:
            index = args.index.resolve()
            if index.exists():
                raise RuntimeError(f"duplicate index already exists: {index}")
            index.parent.mkdir(parents=True, exist_ok=True)
            report = validate(files, args.min_chars, args.max_document_bytes, denied, index, args.max_errors)
        else:
            with tempfile.TemporaryDirectory(prefix="zdx-w0-validation-") as directory:
                report = validate(
                    files,
                    args.min_chars,
                    args.max_document_bytes,
                    denied,
                    Path(directory) / "duplicates.sqlite3",
                    args.max_errors,
                )
        if args.report:
            atomic_json(args.report, report)
        print(json.dumps(report, indent=2, sort_keys=True))
        return 0 if report["valid"] else 2
    except (OSError, RuntimeError, sqlite3.Error) as exc:
        print(json.dumps({"valid": False, "error": str(exc)}, indent=2))
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
