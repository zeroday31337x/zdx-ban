#!/usr/bin/env python3
"""Single-transformer-block perturbation comparison: INT8 vs 4-bit vs original.

Runs Qwen3 layer 0 (the first decoder block) in numpy on a fixed hidden
state — the embedding of a short real token sequence — once with the
original BF16 weights, once with the INT8-dequantized weights, and once
with the 4-bit-dequantized weights. Reports how far each quantized
block output drifts from the original block output (relative L2 error
and max absolute error).

This is NOT a perplexity or end-to-end quality measure: it does not run
the full 36-layer model, and no forward pass of the whole 8B model fits
in this machine's ~6 GB RAM (no torch/transformers/llama.cpp here).
What it does measure, honestly, is how much each quantization perturbs
a real computation (attention + MLP) given the same input. The same
code path runs all three weight sources, so any systematic quirk in
this re-implementation affects all three equally and the RELATIVE
comparison (INT8 error vs 4-bit error) is the meaningful result.

Weights are loaded one tensor at a time as float32 and freed after use;
peak RAM stays well under 1 GB.
"""

from __future__ import annotations

import json
import math
import os
import sys

import numpy as np

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from quantize import BLOCK_SIZE, bf16_to_f32, dequantize

MODEL_DIR = os.path.expanduser("~/workspace/qwen3-8b")
INT8_DIR = os.path.expanduser("~/workspace/qwen3-8b-spatial-int8")
INT4_DIR = os.path.expanduser("~/workspace/qwen3-8b-spatial")

# Real token sequences (IDs looked up in vocab.json).
INPUTS = {
    "fox": [785, 3974, 13876, 38835, 34208, 916, 279, 15678, 5562],  # The quick brown fox jumps over the lazy dog
    "hello": [9707, 21927, 374, 264, 1273],  # Hello world is a test
}


def embed_rows(token_ids: list[int]) -> np.ndarray:
    """Embedding rows for the given token IDs only (never loads the full
    622M-param embedding as float32 — that is 2.5 GB and was the earlier
    OOM trap). Rows are sliced from the memory-mapped BF16 tensor first,
    then converted."""
    from safetensors import safe_open

    index = json.load(open(os.path.join(MODEL_DIR, "model.safetensors.index.json")))
    shard = index["weight_map"]["model.embed_tokens.weight"]
    with safe_open(os.path.join(MODEL_DIR, shard), framework="np") as f:
        tensor = f.get_tensor("model.embed_tokens.weight")
        rows = np.ascontiguousarray(tensor[np.array(token_ids, dtype=np.int64)])
    return bf16_to_f32(rows)


def load_config():
    with open(os.path.join(MODEL_DIR, "config.json")) as f:
        return json.load(f)


def rmsnorm(x: np.ndarray, weight: np.ndarray, eps: float) -> np.ndarray:
    var = np.mean(x * x, axis=-1, keepdims=True)
    return (x / np.sqrt(var + eps)) * weight


def silu(x: np.ndarray) -> np.ndarray:
    return x / (1.0 + np.exp(-x))


def rotate_half(x: np.ndarray) -> np.ndarray:
    """GPT-NeoX style rotate_half: split last dim in half, (-x2, x1)."""
    d = x.shape[-1]
    x1, x2 = x[..., : d // 2], x[..., d // 2 :]
    return np.concatenate([-x2, x1], axis=-1)


def apply_rope(q: np.ndarray, k: np.ndarray, theta: float) -> tuple[np.ndarray, np.ndarray]:
    """q, k: (T, n_heads, head_dim). Positions 0..T-1. HF rotate_half convention."""
    T, _nh, d = q.shape
    pos = np.arange(T, dtype=np.float32)[:, None]  # (T, 1)
    inv_freq = 1.0 / (theta ** (np.arange(0, d, 2, dtype=np.float32) / d))  # (d/2,)
    freqs = pos * inv_freq[None, :]  # (T, d/2)
    cos = np.cos(freqs)[..., None, :]  # (T, 1, d/2)
    sin = np.sin(freqs)[..., None, :]
    # Broadcast cos/sin over the two halves of the head dim.
    cos_full = np.concatenate([cos, cos], axis=-1)
    sin_full = np.concatenate([sin, sin], axis=-1)
    q = q * cos_full + rotate_half(q) * sin_full
    k = k * cos_full + rotate_half(k) * sin_full
    return q, k


class WeightSource:
    """One tensor at a time, as float32, from original / int8 / int4."""

    def __init__(self, kind: str):
        self.kind = kind  # "orig", "int8", "int4"
        if kind == "orig":
            from safetensors import safe_open

            index = json.load(open(os.path.join(MODEL_DIR, "model.safetensors.index.json")))
            self.weight_map = index["weight_map"]
            self._open: dict[str, object] = {}
        else:
            work = INT8_DIR if kind == "int8" else INT4_DIR
            self.manifest = json.load(open(os.path.join(work, "quant-manifest.json")))
            # Old 4-bit manifests predate the "bits" field; derive from the
            # quantization method string ("int4-grouped" / "int8-grouped").
            if "bits" in self.manifest:
                self.bits = int(self.manifest["bits"])
            else:
                self.bits = 4 if self.manifest.get("quantization", "").startswith("int4") else 8
            self.by_name = {t["name"]: t for t in self.manifest["tensors"]}
            # Prefer the unpacked (round-tripped) stream; fall back to stream.bin.
            recon = os.path.join(work, "stream-reconstructed.bin")
            self.stream_path = recon if os.path.exists(recon) else os.path.join(work, "stream.bin")
            self._stream_f = open(self.stream_path, "rb")

    def _orig_tensor(self, name: str) -> np.ndarray:
        from safetensors import safe_open

        shard = self.weight_map[name]
        if shard not in self._open:
            self._open[shard] = safe_open(os.path.join(MODEL_DIR, shard), framework="np")
        return self._open[shard].get_tensor(name)

    def get(self, name: str) -> np.ndarray:
        if self.kind == "orig":
            return bf16_to_f32(self._orig_tensor(name))
        entry = self.by_name[name]
        off = entry.get("value_offset", entry.get("nibble_offset"))
        nbytes = entry.get("value_bytes", entry.get("nibble_bytes"))
        self._stream_f.seek(off)
        val = self._stream_f.read(nbytes)
        self._stream_f.seek(entry["scale_offset"])
        sc = self._stream_f.read(entry["scale_bytes"])
        return dequantize(val, sc, entry["numel"], bits=self.bits).reshape(entry["shape"])

    def close(self):
        if self.kind != "orig":
            self._stream_f.close()
        else:
            for f in self._open.values():
                try:
                    f.close()
                except Exception:
                    pass


def run_block(src: WeightSource, cfg: dict, token_ids: list[int]) -> np.ndarray:
    """Run Qwen3 layer 0 on the embedding of token_ids. Returns (T, hidden)."""
    hidden = cfg["hidden_size"]
    n_q = cfg["num_attention_heads"]
    n_kv = cfg["num_key_value_heads"]
    head_dim = hidden // n_q
    n_rep = n_q // n_kv
    eps = float(cfg["rms_norm_eps"])
    theta = float(cfg["rope_theta"])
    L = "model.layers.0"

    # Input hidden state from the embedding (always the ORIGINAL embedding:
    # the comparison is about the block weights, not the embedding).
    x = embed_rows(token_ids).astype(np.float32)
    T = x.shape[0]

    # --- Attention sub-block ---
    h = rmsnorm(x, src.get(f"{L}.input_layernorm.weight"), eps)
    q = h @ src.get(f"{L}.self_attn.q_proj.weight").T
    k = h @ src.get(f"{L}.self_attn.k_proj.weight").T
    v = h @ src.get(f"{L}.self_attn.v_proj.weight").T
    q = q.reshape(T, n_q, head_dim)
    k = k.reshape(T, n_kv, head_dim)
    v = v.reshape(T, n_kv, head_dim)
    # Per-head Q/K norms (Qwen3).
    q = rmsnorm(q, src.get(f"{L}.self_attn.q_norm.weight"), eps)
    k = rmsnorm(k, src.get(f"{L}.self_attn.k_norm.weight"), eps)
    q, k = apply_rope(q, k, theta)
    # Repeat KV heads (GQA).
    k = np.repeat(k, n_rep, axis=1)
    v = np.repeat(v, n_rep, axis=1)
    # Causal attention.
    scores = np.matmul(q.transpose(1, 0, 2), k.transpose(1, 0, 2).transpose(0, 2, 1)) / math.sqrt(head_dim)
    mask = np.triu(np.ones((T, T), dtype=bool), k=1)
    scores = np.where(mask[None, :, :], -1e9, scores)
    scores = scores - scores.max(axis=-1, keepdims=True)
    probs = np.exp(scores)
    probs = probs / probs.sum(axis=-1, keepdims=True)
    attn = np.matmul(probs, v.transpose(1, 0, 2)).transpose(1, 0, 2)  # (T, n_q, head_dim)
    attn = attn.reshape(T, hidden)
    o = attn @ src.get(f"{L}.self_attn.o_proj.weight").T
    x = x + o
    del h, q, k, v, scores, probs, attn, o

    # --- MLP sub-block (SwiGLU) ---
    h2 = rmsnorm(x, src.get(f"{L}.post_attention_layernorm.weight"), eps)
    gate = h2 @ src.get(f"{L}.mlp.gate_proj.weight").T
    up = h2 @ src.get(f"{L}.mlp.up_proj.weight").T
    mlp = silu(gate) * up
    del gate, up, h2
    down = mlp @ src.get(f"{L}.mlp.down_proj.weight").T
    x = x + down
    return x


def main() -> int:
    cfg = load_config()
    results: dict = {"inputs": {}, "note": (
        "Single-block (layer 0) perturbation comparison on fixed token inputs. "
        "Not a perplexity or end-to-end quality measure. Same code path for all "
        "three weight sources; the relative INT8-vs-4-bit error is the result."
    )}
    for label, token_ids in INPUTS.items():
        orig_src = WeightSource("orig")
        out_orig = run_block(orig_src, cfg, token_ids)
        orig_src.close()
        assert np.all(np.isfinite(out_orig)), "original block output non-finite"
        entry: dict = {"tokens": token_ids, "orig_output_norm": float(np.linalg.norm(out_orig))}
        for kind in ("int8", "int4"):
            src = WeightSource(kind)
            out_q = run_block(src, cfg, token_ids)
            src.close()
            assert np.all(np.isfinite(out_q)), f"{kind} block output non-finite"
            diff = out_q - out_orig
            entry[kind] = {
                "rel_l2_error": float(np.linalg.norm(diff) / np.linalg.norm(out_orig)),
                "max_abs_error": float(np.abs(diff).max()),
                "mean_abs_error": float(np.abs(diff).mean()),
            }
            print(f"[{label}] {kind}: rel_l2={entry[kind]['rel_l2_error']:.6f} "
                  f"max_abs={entry[kind]['max_abs_error']:.4f} "
                  f"mean_abs={entry[kind]['mean_abs_error']:.6f}", flush=True)
        results["inputs"][label] = entry
    out_path = os.path.join(INT8_DIR, "block-perturb.json")
    with open(out_path, "w") as f:
        json.dump(results, f, indent=2)
    print(f"Wrote {out_path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
