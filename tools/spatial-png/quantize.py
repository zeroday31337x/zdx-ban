"""Grouped weight quantization for the spatial PNG reduction.

Method (the phone standard, like Q4/Q8): symmetric per-block quantization.
  - Block size 128 consecutive weights share one FP16 scale.
  - 4-bit: scale = max_abs(block) / 7 ; q = round(w / scale), clamped to
    [-8, 7]. Two int4 values are packed per byte (low nibble = first
    element). 8.19B params -> ~4.10 GB of nibbles + ~0.13 GB of scales.
  - 8-bit: scale = max_abs(block) / 127 ; q = round(w / scale), clamped
    to [-128, 127]. Stored as one uint8 per weight (q + 128).
    8.19B params -> ~8.19 GB of bytes + ~0.13 GB of scales.
  - Scales are stored as raw FP16 bytes, one per block, in stream order.

Dequantization error is bounded by scale/2 per weight. INT8 has 16x
more levels per block than 4-bit, so its error is much smaller; it
costs exactly 2x the storage.

BF16 handling: safetensors stores BF16. numpy has no portable BF16
dtype, so BF16 is read as raw uint16 bits and converted to float32 by
shifting into the top 16 bits of an FP32 (exact, no rounding).
"""

from __future__ import annotations

import json
import math
import os

import ml_dtypes  # noqa: F401  (registers bfloat16 with numpy; safetensors needs it)
import numpy as np

BLOCK_SIZE = 128

# Per-bit-width quantization ranges (symmetric).
_QRANGE = {
    4: (-8, 7),
    8: (-128, 127),
}

SUPPORTED_BITS = (4, 8)


def bf16_to_f32(arr: np.ndarray) -> np.ndarray:
    """Convert a BF16 tensor (as raw uint16 bits, or float16/float32) to float32.

    BF16 is the top 16 bits of an FP32, so the conversion is exact:
    float32_bits = uint16_bits << 16.
    """
    if arr.dtype == np.float32:
        return np.ascontiguousarray(arr, dtype=np.float32)
    if arr.dtype == np.float16:
        return arr.astype(np.float32)
    if arr.dtype == np.uint16:
        # Raw BF16 bits.
        bits = arr.astype(np.uint32) << 16
        return bits.view(np.float32)
    # ml_dtypes.bfloat16 (if present) exposes the same bit pattern.
    if arr.dtype.name == "bfloat16":
        bits = arr.view(np.uint16).astype(np.uint32) << 16
        return bits.view(np.float32)
    raise TypeError(f"bf16_to_f32: unsupported dtype {arr.dtype}")


def quantize_f32(
    arr: np.ndarray, block_size: int = BLOCK_SIZE, bits: int = 4
) -> tuple[bytes, bytes]:
    """Quantize a 1D float32 array to grouped int4 or int8.

    Returns (packed_values, scales_f16_bytes). Length must be a multiple
    of block_size; the caller pads the final block with zeros if needed.
    4-bit packs two values per byte (low nibble first); 8-bit stores one
    uint8 per value (q + 128).
    """
    if bits not in _QRANGE:
        raise ValueError(f"quantize_f32: unsupported bits {bits} (use 4 or 8)")
    qmin, qmax = _QRANGE[bits]
    flat = np.ascontiguousarray(arr, dtype=np.float32).reshape(-1)
    n = flat.shape[0]
    if n % block_size != 0:
        raise ValueError(f"quantize_f32: length {n} is not a multiple of block_size {block_size}")
    blocks = flat.reshape(-1, block_size)
    max_abs = np.max(np.abs(blocks), axis=1).astype(np.float32)
    # Avoid division by zero for all-zero blocks.
    scale = np.where(max_abs > 0, max_abs / float(qmax), np.float32(1.0)).astype(np.float32)
    q = np.round(blocks / scale[:, None]).astype(np.int16)
    q = np.clip(q, qmin, qmax)
    if bits == 4:
        # Pack two int4 per byte: low nibble = even index, high nibble = odd.
        # Store as unsigned nibbles by adding 8 (range 0..15).
        qu = (q + 8).astype(np.uint8)
        even = qu[:, 0::2]
        odd = qu[:, 1::2]
        packed = ((odd << 4) | even).astype(np.uint8).tobytes()
    else:  # bits == 8: one uint8 per value (q + 128, range 0..255).
        packed = (q + 128).astype(np.uint8).tobytes()
    scales = scale.astype(np.float16).tobytes()
    return packed, scales


def dequantize(
    packed: bytes, scales: bytes, numel: int, block_size: int = BLOCK_SIZE, bits: int = 4
) -> np.ndarray:
    """Reconstruct float32 weights from packed values + FP16 scales.

    Returns exactly numel values (any zero-padding in the final block is
    dropped).
    """
    if bits not in _QRANGE:
        raise ValueError(f"dequantize: unsupported bits {bits} (use 4 or 8)")
    qmin, _qmax = _QRANGE[bits]
    nblocks = math.ceil(numel / block_size)
    padded = nblocks * block_size
    raw = np.frombuffer(packed, dtype=np.uint8)
    if bits == 4:
        if raw.shape[0] * 2 < padded:
            raise ValueError("dequantize: packed nibbles too short for numel")
        # Unpack nibbles: low nibble first.
        low = (raw & 0x0F).astype(np.int8)
        high = ((raw >> 4) & 0x0F).astype(np.int8)
        q = np.empty(raw.shape[0] * 2, dtype=np.int8)
        q[0::2] = low
        q[1::2] = high
        q = (q - 8).astype(np.float32)[:padded]
    else:  # bits == 8
        if raw.shape[0] < padded:
            raise ValueError("dequantize: packed bytes too short for numel")
        q = (raw.astype(np.int16) + qmin).astype(np.float32)[:padded]
    scale = np.frombuffer(scales, dtype=np.float16).astype(np.float32)
    if scale.shape[0] < nblocks:
        raise ValueError("dequantize: scales too short for numel")
    scale = np.repeat(scale[:nblocks], block_size)
    return (q * scale)[:numel]


def quantize_model(model_dir: str, stream_path: str, manifest_path: str, bits: int = 4) -> dict:
    """Quantize every tensor in a safetensors model dir to grouped int4/int8.

    Reads tensors shard by shard (memory-mapped, chunked so peak RAM stays
    bounded), appends values then scales to one contiguous stream file,
    and writes a manifest JSON describing each tensor's layout in the
    stream. Returns the manifest dict.

    Stream layout: [all tensors' values, in manifest order]
                   [all tensors' scales, in manifest order]
    The manifest records per-tensor value/scale byte offsets and lengths,
    so the unpacker can reconstruct each tensor independently.
    """
    from safetensors import safe_open

    if bits not in _QRANGE:
        raise ValueError(f"quantize_model: unsupported bits {bits} (use 4 or 8)")
    qmin, qmax = _QRANGE[bits]

    index_path = os.path.join(model_dir, "model.safetensors.index.json")
    with open(index_path, "r") as f:
        index = json.load(f)
    weight_map: dict[str, str] = index["weight_map"]
    # Group tensors by shard file, preserving a deterministic order.
    shards: dict[str, list[str]] = {}
    for name in sorted(weight_map):
        shards.setdefault(weight_map[name], []).append(name)

    # Chunk size (elements) for large tensors; must be a multiple of BLOCK_SIZE.
    CHUNK = BLOCK_SIZE * 65536  # 8,388,608 elements (~33.5 MB as float32)

    manifest: dict = {
        "model": "Qwen/Qwen3-8B",
        "quantization": f"int{bits}-grouped",
        "bits": bits,
        "block_size": BLOCK_SIZE,
        "qmin": qmin,
        "qmax": qmax,
        "tensors": [],
    }
    total_params = 0
    value_total = 0
    scale_total = 0
    # Stream to disk incrementally (values -> stream file, scales -> temp
    # file) so peak RAM stays bounded; never accumulate the ~8.3 GB stream
    # in memory. The scale section is appended after the value section.
    scale_tmp_path = stream_path + ".scales.tmp"
    with open(stream_path, "wb") as stream_out, open(scale_tmp_path, "wb") as scale_out:
        for shard in sorted(shards):
            shard_path = os.path.join(model_dir, shard)
            with safe_open(shard_path, framework="np") as f:
                for name in shards[shard]:
                    tensor = f.get_tensor(name)
                    shape = list(tensor.shape)
                    numel = int(np.prod(shape)) if shape else 1
                    total_params += numel
                    flat = np.ascontiguousarray(tensor).reshape(-1)
                    nblocks = math.ceil(numel / BLOCK_SIZE)
                    value_off = value_total
                    scale_off = scale_total
                    value_bytes = 0
                    scale_bytes = 0
                    for start in range(0, numel, CHUNK):
                        end = min(start + CHUNK, numel)
                        chunk = bf16_to_f32(flat[start:end])
                        if chunk.shape[0] % BLOCK_SIZE != 0:
                            pad = BLOCK_SIZE - (chunk.shape[0] % BLOCK_SIZE)
                            chunk = np.concatenate([chunk, np.zeros(pad, dtype=np.float32)])
                        packed, sc = quantize_f32(chunk, bits=bits)
                        stream_out.write(packed)
                        scale_out.write(sc)
                        value_bytes += len(packed)
                        scale_bytes += len(sc)
                    value_total += value_bytes
                    scale_total += scale_bytes
                    manifest["tensors"].append(
                        {
                            "name": name,
                            "shape": shape,
                            "dtype": str(tensor.dtype),
                            "numel": numel,
                            "nblocks": nblocks,
                            "value_offset": value_off,
                            "value_bytes": value_bytes,
                            # scale_offset is fixed up to absolute below.
                            "scale_offset": scale_off,
                            "scale_bytes": scale_bytes,
                        }
                    )
    # Append the scale section and fix scale offsets to absolute.
    with open(stream_path, "ab") as stream_out, open(scale_tmp_path, "rb") as scale_in:
        while True:
            block = scale_in.read(1024 * 1024)
            if not block:
                break
            stream_out.write(block)
    os.remove(scale_tmp_path)
    for entry in manifest["tensors"]:
        entry["scale_offset"] = value_total + entry["scale_offset"]

    manifest["total_params"] = total_params
    manifest["value_total_bytes"] = value_total
    manifest["scale_total_bytes"] = scale_total
    manifest["stream_total_bytes"] = value_total + scale_total
    with open(manifest_path, "w") as f:
        json.dump(manifest, f, indent=2)
    return manifest
