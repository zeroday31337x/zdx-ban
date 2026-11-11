"""Decode RGBA spatial PNG frames back to the quantized byte stream.

Verification (the round-trip check):
  1. Each frame's storage bytes must SHA-256-match its frames.json entry.
  2. The reassembled stream must SHA-256-match the original stream file.
Both must pass for the reduction to be called lossless. Any mismatch is
reported with the frame index and the two hashes; nothing is silently
accepted.
"""

from __future__ import annotations

import hashlib
import json
import os

from rgba_frame import read_storage_bytes

_CHUNK = 1024 * 1024  # 1 MB


def file_sha256(path: str) -> str:
    """Streaming SHA-256 of a file (bounded RAM, any size)."""
    h = hashlib.sha256()
    with open(path, "rb") as f:
        while True:
            block = f.read(_CHUNK)
            if not block:
                break
            h.update(block)
    return h.hexdigest()


def unpack_frames(frames_dir: str, out_stream_path: str) -> dict:
    """Reassemble the byte stream from frames.json + frame-*.png.

    Returns a report dict: total bytes, per-frame SHA-256 results, and
    whether every frame matched. Raises on the first mismatch so a bad
    frame is never silently passed through.
    """
    manifest_path = os.path.join(frames_dir, "frames.json")
    with open(manifest_path, "r") as f:
        manifest = json.load(f)
    report: dict = {
        "n_frames": len(manifest["frames"]),
        "stream_total_bytes": manifest["stream_total_bytes"],
        "frames_checked": 0,
        "frames_ok": 0,
        "frames_bad": [],
        "total_bytes_written": 0,
    }
    with open(out_stream_path, "wb") as out:
        for entry in manifest["frames"]:
            fpath = os.path.join(frames_dir, entry["file"])
            data = read_storage_bytes(fpath, length=entry["storage_bytes"])
            actual = hashlib.sha256(data).hexdigest()
            report["frames_checked"] += 1
            if actual != entry["sha256"]:
                report["frames_bad"].append(
                    {
                        "index": entry["index"],
                        "file": entry["file"],
                        "expected": entry["sha256"],
                        "actual": actual,
                    }
                )
                raise ValueError(
                    f"frame {entry['index']} ({entry['file']}) SHA-256 mismatch: "
                    f"expected {entry['sha256']}, got {actual}"
                )
            report["frames_ok"] += 1
            out.write(data)
            report["total_bytes_written"] += len(data)
    if report["total_bytes_written"] != manifest["stream_total_bytes"]:
        raise ValueError(
            f"reassembled stream is {report['total_bytes_written']} bytes, "
            f"expected {manifest['stream_total_bytes']}"
        )
    return report


def verify_stream(reconstructed_path: str, original_path: str) -> dict:
    """Byte-for-byte comparison of two streams via SHA-256 (streamed)."""
    rec = file_sha256(reconstructed_path)
    orig = file_sha256(original_path)
    return {
        "reconstructed_sha256": rec,
        "original_sha256": orig,
        "match": rec == orig,
        "reconstructed_bytes": os.path.getsize(reconstructed_path),
        "original_bytes": os.path.getsize(original_path),
    }
