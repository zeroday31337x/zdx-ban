"""End-to-end driver for the 8B spatial PNG reduction.

Stages:
  1. quantize : Qwen3-8B safetensors -> grouped int4 or int8 stream + manifest.
  2. pack     : stream -> RGBA spatial PNG frames + frames.json.
  3. unpack   : frames -> reassembled stream; SHA-256 round-trip check.
  4. dequant  : full per-tensor dequantization error vs the originals.
  5. stream   : VM daisy-chain through the frames, measuring live RAM.

Usage (run from this directory with the spatial venv):
  python run.py --model-dir ~/workspace/qwen3-8b \
                --work-dir ~/workspace/qwen3-8b-spatial \
                --frames-dir ~/workspace/qwen3-8b-spatial/frames \
                --bits 4
  python run.py --model-dir ~/workspace/qwen3-8b \
                --work-dir ~/workspace/qwen3-8b-spatial-int8 \
                --frames-dir ~/workspace/qwen3-8b-spatial-int8/frames \
                --bits 8

Stages are skipped automatically when their outputs already exist
(stream.bin, frames/frames.json); pass --force to redo a stage.
A JSON report is written to <work-dir>/report.json at the end.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import time

import numpy as np

# Allow running from this directory.
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from quantize import BLOCK_SIZE, bf16_to_f32, dequantize, quantize_model
from pack import pack_stream
from rgba_frame import STORAGE_CAPACITY_BYTES
from unpack import unpack_frames, verify_stream


def _log(msg: str) -> None:
    print(f"[{time.strftime('%H:%M:%S')}] {msg}", flush=True)


def dequant_error_check(model_dir: str, stream_path: str, quant_manifest: dict) -> dict:
    """Full per-tensor dequantization error vs the original weights.

    Loads each original tensor from its safetensors shard, dequantizes
    the matching values/scales from the (unpacked) stream, and
    accumulates global max/mean absolute error and mean relative error.
    Memory-bounded: the tensor is memory-mapped and both the BF16->f32
    conversion and the error accumulation are done one chunk at a time.

    The manifest's bits field selects the dequantizer (4 = nibbles,
    2 values/byte; 8 = one uint8/value) and the byte-range math.
    """
    from safetensors import safe_open

    bits = int(quant_manifest.get("bits", 4))
    bytes_per_value = bits / 8.0
    index_path = os.path.join(model_dir, "model.safetensors.index.json")
    with open(index_path, "r") as f:
        weight_map: dict[str, str] = json.load(f)["weight_map"]
    shards: dict[str, list[str]] = {}
    for name in sorted(weight_map):
        shards.setdefault(weight_map[name], []).append(name)
    by_name = {t["name"]: t for t in quant_manifest["tensors"]}
    CHUNK = BLOCK_SIZE * 65536
    global_max = 0.0
    global_sum_abs = 0.0
    global_sum_rel = 0.0
    global_n = 0
    worst_tensor = ""
    per_tensor: list[dict] = []

    def _read_range(f, offset: int, length: int) -> bytes:
        """Read a byte range from the stream via seek (never loads it all)."""
        f.seek(offset)
        return f.read(length)

    def _value_range(entry: dict) -> tuple[int, int]:
        """(offset, bytes) of a tensor's quantized values in the stream."""
        off = entry.get("value_offset", entry.get("nibble_offset"))
        nbytes = entry.get("value_bytes", entry.get("nibble_bytes"))
        return off, nbytes

    with open(stream_path, "rb") as stream_f:
        for shard in sorted(shards):
            shard_path = os.path.join(model_dir, shard)
            with safe_open(shard_path, framework="np") as f:
                for name in shards[shard]:
                    entry = by_name[name]
                    numel = entry["numel"]
                    val_off, val_bytes = _value_range(entry)
                    val = _read_range(stream_f, val_off, val_bytes)
                    sc = _read_range(stream_f, entry["scale_offset"], entry["scale_bytes"])
                    # Memory-mapped tensor (no RAM); flat is a view. Convert
                    # to float32 ONLY one chunk at a time (~33 MB), and
                    # accumulate the error per chunk. No full-tensor float32,
                    # recon, or error array is ever created (an earlier
                    # version did that and was OOM-killed on the 622M-param
                    # embedding).
                    flat = np.ascontiguousarray(f.get_tensor(name)).reshape(-1)
                    t_max = 0.0
                    t_sum_abs = 0.0
                    t_sum_rel = 0.0
                    for start in range(0, numel, CHUNK):
                        end = min(start + CHUNK, numel)
                        orig_chunk = bf16_to_f32(flat[start:end])
                        b0 = (start // BLOCK_SIZE) * BLOCK_SIZE
                        b1 = ((end + BLOCK_SIZE - 1) // BLOCK_SIZE) * BLOCK_SIZE
                        val_lo = int(b0 * bytes_per_value)
                        val_hi = int(b1 * bytes_per_value)
                        sc_lo = (b0 // BLOCK_SIZE) * 2
                        sc_hi = (b1 // BLOCK_SIZE) * 2
                        part = dequantize(val[val_lo:val_hi], sc[sc_lo:sc_hi], b1 - b0, bits=bits)
                        recon_chunk = part[start - b0: end - b0]
                        err = np.abs(recon_chunk - orig_chunk)
                        denom = np.maximum(np.abs(orig_chunk), 1e-6)
                        rel = err / denom
                        c_max = float(err.max()) if err.size else 0.0
                        if c_max > t_max:
                            t_max = c_max
                        t_sum_abs += float(err.sum())
                        t_sum_rel += float(rel.sum())
                        del orig_chunk, recon_chunk, err, denom, rel, part
                    t_mean = t_sum_abs / max(numel, 1)
                    t_rel = t_sum_rel / max(numel, 1)
                    if t_max > global_max:
                        global_max = t_max
                        worst_tensor = name
                    global_sum_abs += t_sum_abs
                    global_sum_rel += t_sum_rel
                    global_n += numel
                    per_tensor.append(
                        {"name": name, "max_abs": t_max, "mean_abs": t_mean, "mean_rel": t_rel}
                    )
    per_tensor.sort(key=lambda d: d["max_abs"], reverse=True)
    return {
        "tensors_checked": len(per_tensor),
        "total_params": global_n,
        "global_max_abs_error": global_max,
        "global_mean_abs_error": global_sum_abs / max(global_n, 1),
        "global_mean_rel_error": global_sum_rel / max(global_n, 1),
        "worst_tensor": worst_tensor,
        "worst_5_tensors": per_tensor[:5],
    }


def main() -> int:
    ap = argparse.ArgumentParser(description="8B spatial PNG reduction driver")
    ap.add_argument("--model-dir", required=True, help="Qwen3-8B safetensors dir")
    ap.add_argument("--work-dir", required=True, help="Working dir for stream + report")
    ap.add_argument("--frames-dir", required=True, help="Output dir for PNG frames")
    ap.add_argument("--bits", type=int, choices=[4, 8], default=4,
                    help="Quantization width: 4 (nibbles) or 8 (one byte/value)")
    ap.add_argument("--force", action="store_true", help="Redo stages even if outputs exist")
    args = ap.parse_args()

    os.makedirs(args.work_dir, exist_ok=True)
    stream_path = os.path.join(args.work_dir, "stream.bin")
    quant_manifest_path = os.path.join(args.work_dir, "quant-manifest.json")
    recon_path = os.path.join(args.work_dir, "stream-reconstructed.bin")
    report_path = os.path.join(args.work_dir, "report.json")
    report: dict = {"started": time.strftime("%Y-%m-%dT%H:%M:%S")}

    # Stage 1: quantize.
    if os.path.exists(stream_path) and os.path.exists(quant_manifest_path) and not args.force:
        _log(f"Stage 1 quantize: SKIP (stream exists: {stream_path})")
        with open(quant_manifest_path, "r") as f:
            quant_manifest = json.load(f)
    else:
        _log(f"Stage 1 quantize: {args.bits}-bit grouped (block 128) over Qwen3-8B ...")
        t0 = time.time()
        quant_manifest = quantize_model(args.model_dir, stream_path, quant_manifest_path, bits=args.bits)
        report["quantize_seconds"] = round(time.time() - t0, 1)
        _log(
            f"Stage 1 done in {report['quantize_seconds']}s: "
            f"{quant_manifest['total_params']:,} params, "
            f"stream {quant_manifest['stream_total_bytes']:,} bytes"
        )
    report["quantize"] = {
        "quantization": quant_manifest.get("quantization"),
        "bits": quant_manifest.get("bits", 4),
        "total_params": quant_manifest["total_params"],
        "value_bytes": quant_manifest.get("value_total_bytes", quant_manifest.get("nibble_total_bytes")),
        "scale_bytes": quant_manifest["scale_total_bytes"],
        "stream_bytes": quant_manifest["stream_total_bytes"],
    }

    # Stage 2: pack.
    frames_manifest_path = os.path.join(args.frames_dir, "frames.json")
    if os.path.exists(frames_manifest_path) and not args.force:
        _log(f"Stage 2 pack: SKIP (frames exist: {frames_manifest_path})")
        with open(frames_manifest_path, "r") as f:
            frames_manifest = json.load(f)
    else:
        _log("Stage 2 pack: stream -> RGBA spatial PNG frames ...")
        t0 = time.time()
        frames_manifest = pack_stream(stream_path, args.frames_dir, quant_manifest)
        report["pack_seconds"] = round(time.time() - t0, 1)
        _log(
            f"Stage 2 done in {report['pack_seconds']}s: "
            f"{frames_manifest['n_frames']} frames"
        )
    # Total PNG disk size.
    total_png = 0
    for entry in frames_manifest["frames"]:
        total_png += os.path.getsize(os.path.join(args.frames_dir, entry["file"]))
    report["pack"] = {
        "n_frames": frames_manifest["n_frames"],
        "storage_capacity_bytes_per_frame": STORAGE_CAPACITY_BYTES,
        "total_png_bytes": total_png,
        "expected_final_m0": frames_manifest["expected_final_m0"],
    }
    _log(f"Frames on disk: {frames_manifest['n_frames']} PNGs, {total_png:,} bytes total")

    # Stage 3: unpack + round-trip.
    _log("Stage 3 unpack: frames -> stream, SHA-256 round-trip check ...")
    t0 = time.time()
    unpack_report = unpack_frames(args.frames_dir, recon_path)
    verify = verify_stream(recon_path, stream_path)
    report["unpack_seconds"] = round(time.time() - t0, 1)
    report["round_trip"] = {
        "frames_checked": unpack_report["frames_checked"],
        "frames_ok": unpack_report["frames_ok"],
        "frames_bad": unpack_report["frames_bad"],
        "reassembled_bytes": unpack_report["total_bytes_written"],
        "stream_sha256_match": verify["match"],
        "reconstructed_sha256": verify["reconstructed_sha256"],
        "original_sha256": verify["original_sha256"],
    }
    _log(
        f"Stage 3 done: {unpack_report['frames_ok']}/{unpack_report['frames_checked']} "
        f"frames SHA-OK, stream match={verify['match']}"
    )
    if not verify["match"]:
        _log("ERROR: round-trip stream SHA-256 mismatch — stopping before stream test.")
        report["status"] = "FAILED: round-trip mismatch"
        with open(report_path, "w") as f:
            json.dump(report, f, indent=2)
        return 1

    # Stage 4: dequantization error (full, per-tensor).
    _log("Stage 4 dequant: full per-tensor error vs original weights ...")
    t0 = time.time()
    dq = dequant_error_check(args.model_dir, recon_path, quant_manifest)
    report["dequant_seconds"] = round(time.time() - t0, 1)
    report["dequant_error"] = dq
    _log(
        f"Stage 4 done: global max_abs={dq['global_max_abs_error']:.4f}, "
        f"mean_abs={dq['global_mean_abs_error']:.6f}, "
        f"mean_rel={dq['global_mean_rel_error']:.4f}, worst={dq['worst_tensor']}"
    )

    # Stage 5: VM daisy-chain streaming + RAM.
    # Run in a FRESH subprocess so the reported peak RSS reflects only
    # the streaming (one frame at a time), not this driver's earlier
    # stages (which held the model and the stream). The child's RSS is
    # sampled EXTERNALLY from /proc/<pid>/status every 0.1s by this
    # parent process: the streamer's own after-each-frame VmRSS reading
    # under-reports (sampled after the frame array is freed), so the
    # external peak is authoritative.
    _log("Stage 5 stream: VM daisy-chain through frames, measuring live RAM ...")
    t0 = time.time()
    stream_report_path = os.path.join(args.work_dir, "stream-report.json")
    mem_samples_path = os.path.join(args.work_dir, "stream-mem-samples.json")
    import resource
    import subprocess

    def _child_rss_kb(pid: int) -> int:
        try:
            with open(f"/proc/{pid}/status", "r") as fh:
                for line in fh:
                    if line.startswith("VmRSS:"):
                        return int(line.split()[1])
        except OSError:
            return 0
        return 0

    proc = subprocess.Popen(
        [
            sys.executable,
            os.path.join(os.path.dirname(os.path.abspath(__file__)), "stream.py"),
            "--frames-dir",
            args.frames_dir,
            "--report",
            stream_report_path,
        ],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    samples: list[int] = []
    while proc.poll() is None:
        rss = _child_rss_kb(proc.pid)
        if rss:
            samples.append(rss)
        time.sleep(0.1)
    stdout, stderr = proc.communicate()
    # One final sample in case the child exited between polls.
    rss = _child_rss_kb(proc.pid)
    if rss:
        samples.append(rss)
    with open(mem_samples_path, "w") as f:
        json.dump({"interval_s": 0.1, "rss_kb_samples": samples}, f)
    if proc.returncode != 0:
        _log(f"ERROR: stream subprocess failed:\n{stdout}\n{stderr}")
        report["status"] = "FAILED: stream subprocess error"
        report["stream_stderr"] = stderr[-2000:]
        with open(report_path, "w") as f:
            json.dump(report, f, indent=2)
        return 1
    with open(stream_report_path, "r") as f:
        stream_report = json.load(f)
    # External peak: highest VmRSS sampled from outside the child, plus
    # the child's own ru_maxrss (kernel high-water mark).
    external_peak_kb = max(samples) if samples else 0
    child_maxrss_kb = resource.getrusage(resource.RUSAGE_CHILDREN).ru_maxrss
    stream_report["rss_peak_external_kb"] = external_peak_kb
    stream_report["rss_peak_child_maxrss_kb"] = child_maxrss_kb
    stream_report["rss_external_samples"] = len(samples)
    with open(stream_report_path, "w") as f:
        json.dump(stream_report, f, indent=2)
    report["stream_seconds"] = round(time.time() - t0, 1)
    report["stream"] = stream_report
    _log(
        f"Stage 5 done: visited {stream_report['frames_visited']}/"
        f"{stream_report['expected_frames']} frames, "
        f"M0 match={stream_report['m0_matches']} "
        f"(M0={stream_report['final_shared'].get('M0')}, "
        f"expected={stream_report['expected_final_m0']}), "
        f"peak RSS external={external_peak_kb/1024:.1f} MB, "
        f"child ru_maxrss={child_maxrss_kb/1024:.1f} MB"
    )
    if not stream_report["all_frames_visited"] or not stream_report["m0_matches"]:
        report["status"] = "FAILED: stream chain incomplete or M0 mismatch"
        with open(report_path, "w") as f:
            json.dump(report, f, indent=2)
        return 1

    report["status"] = "OK"
    report["finished"] = time.strftime("%Y-%m-%dT%H:%M:%S")
    with open(report_path, "w") as f:
        json.dump(report, f, indent=2)
    _log(f"ALL STAGES OK. Report: {report_path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
