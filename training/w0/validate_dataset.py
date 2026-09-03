#!/usr/bin/env python3
"""Streaming, dependency-free validation for W0 pretraining JSONL shards."""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import os
import re
import sqlite3
import tempfile
import unicodedata
import zlib
from collections import Counter
from pathlib import Path
from typing import Iterable

SCHEMA_VERSION = 1
SPLITS = {"train", "validation", "test"}
REQUIRED_STRINGS = ("id", "text", "source", "source_type", "language", "license", "split", "content_sha256")

# High-precision patterns only: structured secrets/identifiers, not free-form
# phone numbers or addresses, which are too noisy to reject a pretraining
# record on regex alone. This is a gate, not a general PII classifier.
PII_PATTERNS: dict[str, re.Pattern[str]] = {
    "email": re.compile(r"[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}"),
    "us_ssn": re.compile(r"(?<!\d)\d{3}-\d{2}-\d{4}(?!\d)"),
    "aws_access_key": re.compile(r"\b(?:AKIA|ASIA)[0-9A-Z]{16}\b"),
    "aws_secret_key": re.compile(r"(?<![A-Za-z0-9/+=])[A-Za-z0-9/+=]{40}(?![A-Za-z0-9/+=])"),
    "private_key_block": re.compile(r"-----BEGIN (?:RSA |EC |OPENSSH |DSA |PGP )?PRIVATE KEY-----"),
    "github_token": re.compile(r"\bgh[pousr]_[A-Za-z0-9]{36}\b"),
    "slack_token": re.compile(r"\bxox[baprs]-[A-Za-z0-9-]{10,}\b"),
    "stripe_like_key": re.compile(r"\b(?:sk|pk)_(?:live|test)_[A-Za-z0-9]{16,}\b"),
    "jwt": re.compile(r"\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b"),
}
_CARD_CANDIDATE = re.compile(r"(?<!\d)(?:\d[ -]?){13,19}(?!\d)")


def _luhn_valid(digits: str) -> bool:
    total = 0
    parity = len(digits) % 2
    for index, char in enumerate(digits):
        value = int(char)
        if index % 2 == parity:
            value *= 2
            if value > 9:
                value -= 9
        total += value
    return total % 10 == 0


def _has_credit_card(text: str) -> bool:
    for match in _CARD_CANDIDATE.finditer(text):
        digits = re.sub(r"[ -]", "", match.group())
        if 13 <= len(digits) <= 19 and digits[0] in "3456" and _luhn_valid(digits):
            return True
    return False


def detect_pii(text: str) -> list[str]:
    """Return sorted category names for high-precision PII/secret matches."""
    found = [name for name, pattern in PII_PATTERNS.items() if pattern.search(text)]
    if _has_credit_card(text):
        found.append("credit_card")
    return sorted(found)


def shingles(text: str, size: int) -> set[str]:
    """Whitespace-token shingles used as the near-duplicate similarity basis."""
    words = text.lower().split()
    if len(words) <= size:
        return {" ".join(words)} if words else set()
    return {" ".join(words[i : i + size]) for i in range(len(words) - size + 1)}


def minhash_signature(shingle_set: set[str], num_hashes: int) -> tuple[int, ...]:
    """Deterministic MinHash signature via CRC32 seeded per hash slot."""
    if not shingle_set:
        return tuple(0 for _ in range(num_hashes))
    signature = []
    for seed in range(num_hashes):
        prefix = seed.to_bytes(4, "big")
        signature.append(min(zlib.crc32(prefix + shingle.encode("utf-8")) for shingle in shingle_set))
    return tuple(signature)


def lsh_bucket_keys(signature: tuple[int, ...], bands: int) -> list[str]:
    """LSH band keys: a collision in any band flags a near-duplicate candidate."""
    rows = len(signature) // bands
    keys = []
    for band in range(bands):
        chunk = signature[band * rows : (band + 1) * rows]
        digest = hashlib.sha256(f"{band}:{','.join(map(str, chunk))}".encode("utf-8")).hexdigest()
        keys.append(digest)
    return keys


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


def load_license_allowlist(path: Path | None) -> set[str] | None:
    """Return the approved license set, or None when no allow-list was given."""
    if path is None:
        return None
    values: set[str] = set()
    with path.open(encoding="utf-8") as stream:
        for line in stream:
            value = line.strip()
            if not value or value.startswith("#"):
                continue
            values.add(value)
    if not values:
        raise RuntimeError("license allow-list is empty")
    return values


def validate_record(
    item: object, min_chars: int, max_bytes: int, pii_scan: bool = True
) -> tuple[dict | None, list[str], list[str]]:
    errors: list[str] = []
    if not isinstance(item, dict):
        return None, ["record must be a JSON object"], []
    for field in REQUIRED_STRINGS:
        value = item.get(field)
        if not isinstance(value, str) or not value.strip():
            errors.append(f"{field} must be a non-empty string")
    if not isinstance(item.get("synthetic"), bool):
        errors.append("synthetic must be a boolean")
    if errors:
        return item, errors, []

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
    pii_categories = detect_pii(normalized) if pii_scan else []
    if pii_categories:
        errors.append(f"potential PII/secret detected: {', '.join(pii_categories)}")
    return item, errors, pii_categories


def validate(
    files: list[Path],
    min_chars: int,
    max_bytes: int,
    deny_hashes: set[str],
    index_path: Path,
    max_errors: int,
    pii_scan: bool = True,
    license_allow: set[str] | None = None,
    near_duplicate: bool = False,
    shingle_size: int = 5,
    num_minhashes: int = 24,
    lsh_bands: int = 8,
) -> dict:
    if near_duplicate and (num_minhashes < 1 or lsh_bands < 1 or num_minhashes % lsh_bands != 0):
        raise RuntimeError("num_minhashes must be a positive multiple of lsh_bands")
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
        "pii_records": 0,
        "license_rejected": 0,
        "near_duplicate_records": 0,
        "by_split": {},
        "by_source_type": {},
        "by_license": {},
        "errors": [],
    }
    split_counts: Counter[str] = Counter()
    source_counts: Counter[str] = Counter()
    license_counts: Counter[str] = Counter()
    connection = sqlite3.connect(index_path)
    connection.execute("PRAGMA journal_mode=WAL")
    connection.execute("PRAGMA synchronous=NORMAL")
    connection.execute("CREATE TABLE ids (value TEXT PRIMARY KEY) WITHOUT ROWID")
    connection.execute("CREATE TABLE contents (value TEXT PRIMARY KEY, split TEXT NOT NULL) WITHOUT ROWID")
    if near_duplicate:
        connection.execute("CREATE TABLE lsh_buckets (bucket TEXT PRIMARY KEY, first_id TEXT NOT NULL)")

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
                    item, errors, pii_categories = validate_record(item, min_chars, max_bytes, pii_scan)
                    if pii_categories:
                        report["pii_records"] += 1
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
                    if license_allow is not None and item["license"] not in license_allow:
                        report["license_rejected"] += 1
                        if not record_invalid:
                            add_error(path, line_number, f"license '{item['license']}' is not in the approved allow-list")
                        record_invalid = True
                    if near_duplicate and not record_invalid:
                        signature = minhash_signature(shingles(text, shingle_size), num_minhashes)
                        is_near_duplicate = False
                        for bucket in lsh_bucket_keys(signature, lsh_bands):
                            try:
                                connection.execute(
                                    "INSERT INTO lsh_buckets(bucket, first_id) VALUES (?, ?)",
                                    (bucket, item["id"]),
                                )
                            except sqlite3.IntegrityError:
                                is_near_duplicate = True
                        if is_near_duplicate:
                            report["near_duplicate_records"] += 1
                            add_error(path, line_number, "text is a near-duplicate of a previously accepted record")
                            record_invalid = True
                    if not record_invalid:
                        report["accepted_records"] += 1
                        report["utf8_bytes"] += len(encoded)
                        report["characters"] += len(text)
                        split_counts[item["split"]] += 1
                        source_counts[item["source_type"]] += 1
                        license_counts[item["license"]] += 1
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
    report["by_license"] = dict(sorted(license_counts.items()))
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
    parser.add_argument(
        "--allow-pii",
        action="store_true",
        help="disable the default-on high-precision PII/secret rejection gate",
    )
    parser.add_argument(
        "--license-allow",
        type=Path,
        help="newline-delimited approved license strings; records outside it are rejected",
    )
    parser.add_argument(
        "--near-duplicate",
        action="store_true",
        help="reject records whose MinHash/LSH signature collides with an earlier record",
    )
    parser.add_argument("--shingle-size", type=int, default=5, help="word-shingle size for near-duplicate detection")
    parser.add_argument("--minhash-count", type=int, default=24, help="MinHash signature length")
    parser.add_argument("--lsh-bands", type=int, default=8, help="LSH bands; must evenly divide --minhash-count")
    args = parser.parse_args()
    if args.min_chars < 1 or args.max_document_bytes < 1 or args.max_errors < 1:
        parser.error("size and error limits must be positive")
    if args.near_duplicate and (
        args.shingle_size < 1
        or args.minhash_count < 1
        or args.lsh_bands < 1
        or args.minhash_count % args.lsh_bands != 0
    ):
        parser.error("--shingle-size/--minhash-count/--lsh-bands must be positive, with bands dividing minhash-count")
    try:
        files = input_files(args.inputs)
        denied = load_deny_hashes(args.deny_hashes)
        license_allow = load_license_allowlist(args.license_allow)
        pii_scan = not args.allow_pii
        if args.index:
            index = args.index.resolve()
            if index.exists():
                raise RuntimeError(f"duplicate index already exists: {index}")
            index.parent.mkdir(parents=True, exist_ok=True)
            report = validate(
                files,
                args.min_chars,
                args.max_document_bytes,
                denied,
                index,
                args.max_errors,
                pii_scan,
                license_allow,
                args.near_duplicate,
                args.shingle_size,
                args.minhash_count,
                args.lsh_bands,
            )
        else:
            with tempfile.TemporaryDirectory(prefix="zdx-w0-validation-") as directory:
                report = validate(
                    files,
                    args.min_chars,
                    args.max_document_bytes,
                    denied,
                    Path(directory) / "duplicates.sqlite3",
                    args.max_errors,
                    pii_scan,
                    license_allow,
                    args.near_duplicate,
                    args.shingle_size,
                    args.minhash_count,
                    args.lsh_bands,
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
