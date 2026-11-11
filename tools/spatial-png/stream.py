"""Daisy-chained frame streaming through the pyxel VM's execution model.

This is an adaptation of the execution + SAVE_STATE chaining logic from
the ZDX PyxelVM (zdx_parallel_vm.py), not a copy of the whole VM. The
full VM (906 lines) pulls in a deep dependency chain (zdx_storage,
zdx_checkpoint, zdx_metrics), so this module re-implements the exact
subset the chaining program uses, with the SAME opcode R values, the
SAME column-major lockstep order, the SAME shared-memory semantics, and
the SAME registry-based SAVE_STATE chaining — but with the chain cap
raised (the original's 100-frame cap is arbitrary) and the raster read
as RGBA-then-converted-to-RGB (mirroring the original's
``img.convert("RGB")`` in _execute_single, so the alpha channel is
storage only and never executed).

Semantics (from the original):
  - Rows = threads (8), columns = time steps; all threads execute
    column X before column X+1.
  - R = opcode dispatch byte; R=0 is always NOP.
  - Registers per thread: A, B, OUT, T. Shared memory M0..M7 persists
    across frames within one chain.
  - SAVE_STATE: G = frame_index into the registry; the next frame's path
    is looked up and executed after this frame finishes.
  - HALT freezes one thread; others continue.
  - The chain cap is a constructor parameter (raised here); hitting it
    raises RuntimeError, never silently truncates.

RAM measurement: only ONE frame is decoded at a time. Peak RSS is read
from /proc/self/status (VmRSS) after each frame and from getrusage's
ru_maxrss at the end, so the reported peak is the true high-water mark
of the streaming run.
"""

from __future__ import annotations

import json
import os
import resource

import numpy as np
from PIL import Image

# Opcode R values — identical to the pyxel VM MAP (zdx_parallel_vm.py).
OP_SET_REG = 10
OP_SET_B = 11
OP_NOT = 12
OP_ADD_THREAD = 20
OP_SUB = 21
OP_MUL = 22
OP_DIV = 23
OP_COPY_OUT = 24
OP_IF_A_EQ = 30
OP_CMP = 31
OP_LOAD_MEM = 40
OP_STORE_MEM = 41
OP_VERIFY_FREQ = 50
OP_SAVE_STATE = 60
OP_JMP = 70
OP_HALT = 255


def _rss_kb() -> int:
    """Current resident set size in KiB, from /proc/self/status."""
    try:
        with open("/proc/self/status", "r") as f:
            for line in f:
                if line.startswith("VmRSS:"):
                    return int(line.split()[1])
    except OSError:
        pass
    return 0


def _peak_rss_kb() -> int:
    """Peak RSS high-water mark in KiB (getrusage ru_maxrss, KiB on Linux)."""
    return resource.getrusage(resource.RUSAGE_SELF).ru_maxrss


class SpatialVM:
    """Minimal pyxel-VM-compatible executor for the chaining program."""

    def __init__(self, threads: int = 8, max_chain: int = 10000,
                 max_col_steps: int = 4096, debug: bool = False):
        if threads < 1:
            raise ValueError(f"threads must be >= 1, got {threads}")
        if max_chain < 1:
            raise ValueError(f"max_chain must be >= 1, got {max_chain}")
        if max_col_steps < 1:
            raise ValueError(f"max_col_steps must be >= 1, got {max_col_steps}")
        self.threads = threads
        self.max_chain = max_chain
        self.max_col_steps = max_col_steps
        self.debug = debug
        self.clock = 0
        self.registers = {
            f"T{i}": {"A": 0, "B": 0, "OUT": 0, "T": 0} for i in range(threads)
        }
        self.shared = {f"M{i}": 0 for i in range(8)}
        self.frame_registry: dict[int, str] = {}
        self.execution_generation = 0

    def register_frame(self, index: int, path: str) -> None:
        self.frame_registry[index] = path

    def _execute_array(self, arr: np.ndarray, source_label: str = "<frame>") -> str | None:
        """Execute one decoded RGB raster. Returns the next frame path or None."""
        arr = np.asarray(arr)
        if arr.dtype != np.uint8 or arr.ndim != 3 or arr.shape[2] != 3:
            raise ValueError("VM raster must be an HxWx3 uint8 RGB array")
        width = arr.shape[1]
        height = arr.shape[0]
        halted: set[int] = set()
        skip_next: dict[int, bool] = {y: False for y in range(self.threads)}
        next_frame: str | None = None
        x = 0
        step = 0
        while x < width and step < self.max_col_steps:
            step += 1
            self.clock = x
            jump_to: int | None = None
            for y in range(self.threads):
                if y >= height or y in halted:
                    continue
                if skip_next[y]:
                    skip_next[y] = False
                    continue
                thread = self.registers[f"T{y}"]
                thread["T"] = x
                r, g, b = int(arr[y, x, 0]), int(arr[y, x, 1]), int(arr[y, x, 2])
                if r == 0:
                    continue  # NOP
                if r == OP_HALT:
                    halted.add(y)
                elif r == OP_SAVE_STATE:
                    if g in self.frame_registry:
                        next_frame = self.frame_registry[g]
                    elif self.debug:
                        print(f"[WARN] SAVE_STATE frame index {g} not in registry at x={x} T{y}")
                elif r == OP_SET_REG:
                    thread["A"] = b
                elif r == OP_SET_B:
                    thread["B"] = b
                elif r == OP_ADD_THREAD:
                    thread["OUT"] = (thread["A"] + thread["B"]) & 0xFF
                elif r == OP_SUB:
                    thread["A"] = (thread["A"] - thread["B"]) & 0xFF
                elif r == OP_MUL:
                    thread["A"] = (thread["A"] * thread["B"]) & 0xFF
                elif r == OP_DIV:
                    thread["A"] = thread["A"] // thread["B"] if thread["B"] != 0 else 0
                elif r == OP_COPY_OUT:
                    thread["A"] = thread["OUT"]
                elif r == OP_NOT:
                    thread["A"] = (~thread["A"]) & 0xFF
                elif r == OP_CMP:
                    thread["OUT"] = 1 if thread["A"] == thread["B"] else 0
                elif r == OP_IF_A_EQ:
                    if thread["A"] != b:
                        skip_next[y] = True
                elif r == OP_LOAD_MEM:
                    key = f"M{g}"
                    if key in self.shared:
                        thread["A"] = self.shared[key]
                elif r == OP_STORE_MEM:
                    key = f"M{g}"
                    if key in self.shared:
                        self.shared[key] = thread["A"]
                elif r == OP_VERIFY_FREQ:
                    mirror_x = width - 1 - x
                    mr, mg, mb = (int(arr[y, mirror_x, 0]),
                                  int(arr[y, mirror_x, 1]),
                                  int(arr[y, mirror_x, 2]))
                    thread["A"] = 1 if (r, g, b) == (mr, mg, mb) else 0
                elif r == OP_JMP:
                    if jump_to is None and 0 <= g < width:
                        jump_to = g
                else:
                    if self.debug:
                        print(f"[WARN] Unknown opcode R={r} G={g} B={b} at x={x} T{y}")
            if jump_to is not None:
                x = jump_to
            else:
                x += 1
        if step >= self.max_col_steps and x < width:
            raise RuntimeError(
                f"max_col_steps ({self.max_col_steps}) reached in frame "
                f"'{source_label}' at column {x}/{width}."
            )
        return next_frame

    def _execute_single(self, image_path: str) -> str | None:
        """Decode one PNG (RGBA -> RGB, mirroring the original) and execute it."""
        with Image.open(image_path) as img:
            arr = np.array(img.convert("RGB"), dtype=np.uint8)
        return self._execute_array(arr, source_label=image_path)

    def execute_chain(self, first_frame: str, frames_dir: str) -> dict:
        """Follow the SAVE_STATE daisy-chain from first_frame.

        Loads ONE frame at a time (the previous frame's array is released
        before the next is decoded). Returns a report with the frames
        visited, the final shared memory, and RAM measurements.
        """
        with open(os.path.join(frames_dir, "frames.json"), "r") as f:
            manifest = json.load(f)
        for entry in manifest["frames"]:
            self.register_frame(entry["index"], os.path.join(frames_dir, entry["file"]))
        rss_before = _rss_kb()
        rss_per_frame: list[int] = []
        visited: list[str] = []
        current = first_frame
        chained = False
        for _ in range(self.max_chain):
            nxt = self._execute_single(current)
            visited.append(current)
            rss_per_frame.append(_rss_kb())
            if nxt is None:
                chained = False
                break
            if not os.path.exists(nxt):
                raise FileNotFoundError(
                    f"SAVE_STATE chained to '{nxt}' but the file does not exist."
                )
            current = nxt
            chained = True
        else:
            if chained:
                raise RuntimeError(
                    f"max_chain ({self.max_chain}) reached while following "
                    f"SAVE_STATE daisy-chain. Last frame: '{current}'."
                )
        self.execution_generation += 1
        return {
            "frames_visited": len(visited),
            "expected_frames": len(manifest["frames"]),
            "all_frames_visited": len(visited) == len(manifest["frames"]),
            "final_shared": {k: int(v) for k, v in sorted(self.shared.items())},
            "expected_final_m0": manifest["expected_final_m0"],
            "m0_matches": self.shared.get("M0") == manifest["expected_final_m0"],
            "rss_before_kb": rss_before,
            "rss_peak_during_kb": max(rss_per_frame) if rss_per_frame else rss_before,
            "rss_peak_ru_maxrss_kb": _peak_rss_kb(),
            "rss_last_kb": rss_per_frame[-1] if rss_per_frame else rss_before,
        }


if __name__ == "__main__":
    import argparse

    ap = argparse.ArgumentParser(description="Stream frames through the VM chain")
    ap.add_argument("--frames-dir", required=True)
    ap.add_argument("--report", required=True, help="Write the stream report JSON here")
    args = ap.parse_args()
    with open(os.path.join(args.frames_dir, "frames.json"), "r") as f:
        _manifest = json.load(f)
    _vm = SpatialVM(threads=8, max_chain=_manifest["n_frames"] + 10)
    _first = os.path.join(args.frames_dir, _manifest["frames"][0]["file"])
    _report = _vm.execute_chain(_first, args.frames_dir)
    with open(args.report, "w") as f:
        json.dump(_report, f, indent=2)
    print(json.dumps(_report, indent=2))

