"""Pack a quantized weight byte stream into RGBA spatial PNG frames.

Each frame is 4096x4096 RGBA. The storage plane (rows 8..4095) holds
66,977,792 bytes of the stream. The execution plane (rows 0..7) holds a
small VM program that accumulates the frame index into shared memory
M0 and daisy-chains to the next frame via SAVE_STATE, so the VM streams
the frames one at a time instead of holding them all in RAM.

Program (row 0 = thread T0; rows 1..7 HALT at column 0), opcode values
taken from the pyxel VM ISA (zdx_parallel_vm.py MAP):
  col 0: LOAD_MEM  M0        (40, 0, 0)   A = running sum
  col 1: SET_B     frame_idx (11, 0, idx) B = this frame's index
  col 2: ADD_THREAD             (20, 0, 0)   OUT = (A + B) & 0xFF
  col 3: COPY_OUT               (24, 0, 0)   A = OUT
  col 4: STORE_MEM M0        (41, 0, 0)   M0 = A
  col 5: SAVE_STATE next_idx (60, i+1, 0) chain (all but last frame)
         HALT                (255,0, 0)   last frame only
  col 6: HALT                (255, 0, 0)

After the full chain, M0 = sum(frame indices 0..N-1) mod 256, which the
streamer verifies — proving every frame executed and chained.
Frame indices must fit in one byte (N <= 255); the 8B reduction uses ~62.
"""

from __future__ import annotations

import hashlib
import json
import os

from rgba_frame import (
    EXECUTION_ROWS,
    STORAGE_CAPACITY_BYTES,
    WIDTH,
    build_frame,
    save_frame,
)

# Opcode R values from the pyxel VM ISA.
OP_LOAD_MEM = 40
OP_SET_B = 11
OP_ADD_THREAD = 20
OP_COPY_OUT = 24
OP_STORE_MEM = 41
OP_SAVE_STATE = 60
OP_HALT = 255

MAX_FRAMES = 255  # frame index must fit in the SAVE_STATE G channel (1 byte)


def chaining_program(frame_index: int, next_index: int | None) -> dict:
    """Execution-plane cells {(x, y): (r, g, b, a)} for one frame."""
    cells: dict[tuple[int, int], tuple[int, int, int, int]] = {}
    # Threads T1..T7 halt immediately.
    for y in range(1, EXECUTION_ROWS):
        cells[(0, y)] = (OP_HALT, 0, 0, 255)
    # Thread T0: accumulate frame index into M0, then chain or halt.
    cells[(0, 0)] = (OP_LOAD_MEM, 0, 0, 255)            # A = M0
    cells[(1, 0)] = (OP_SET_B, 0, frame_index & 0xFF, 255)  # B = frame index
    cells[(2, 0)] = (OP_ADD_THREAD, 0, 0, 255)           # OUT = (A + B) & 0xFF
    cells[(3, 0)] = (OP_COPY_OUT, 0, 0, 255)             # A = OUT
    cells[(4, 0)] = (OP_STORE_MEM, 0, 0, 255)            # M0 = A
    if next_index is None:
        cells[(5, 0)] = (OP_HALT, 0, 0, 255)
    else:
        cells[(5, 0)] = (OP_SAVE_STATE, next_index & 0xFF, 0, 255)
    cells[(6, 0)] = (OP_HALT, 0, 0, 255)
    return cells


def expected_final_m0(n_frames: int) -> int:
    """M0 after the full chain: sum of frame indices 0..N-1, mod 256."""
    return (n_frames * (n_frames - 1) // 2) % 256


def pack_stream(stream_path: str, out_dir: str, quant_manifest: dict) -> dict:
    """Split a byte stream into RGBA spatial PNG frames.

    Returns the frames manifest dict and writes frames.json + frame-*.png
    into out_dir. The stream is read in frame-sized chunks so peak RAM
    stays bounded (one frame at a time).
    """
    os.makedirs(out_dir, exist_ok=True)
    stream_size = os.path.getsize(stream_path)
    n_frames = (stream_size + STORAGE_CAPACITY_BYTES - 1) // STORAGE_CAPACITY_BYTES
    if n_frames > MAX_FRAMES:
        raise ValueError(
            f"stream needs {n_frames} frames but SAVE_STATE frame index is 1 byte "
            f"(max {MAX_FRAMES})"
        )
    frames: list[dict] = []
    with open(stream_path, "rb") as src:
        for i in range(n_frames):
            chunk = src.read(STORAGE_CAPACITY_BYTES)
            if not chunk:
                break
            sha = hashlib.sha256(chunk).hexdigest()
            program = chaining_program(i, i + 1 if i + 1 < n_frames else None)
            image = build_frame(chunk, program_cells=program)
            fname = f"frame-{i:04d}.png"
            save_frame(image, os.path.join(out_dir, fname))
            frames.append(
                {
                    "index": i,
                    "file": fname,
                    "storage_bytes": len(chunk),
                    "sha256": sha,
                    "next_index": i + 1 if i + 1 < n_frames else None,
                }
            )
    frames_manifest = {
        "format": "spatial-png-rgba-v1",
        "width": WIDTH,
        "height": 4096,
        "execution_rows": EXECUTION_ROWS,
        "channels": 4,
        "storage_capacity_bytes_per_frame": STORAGE_CAPACITY_BYTES,
        "stream_total_bytes": stream_size,
        "n_frames": len(frames),
        "expected_final_m0": expected_final_m0(len(frames)),
        "quantization": {
            "method": quant_manifest.get("quantization"),
            "block_size": quant_manifest.get("block_size"),
            "total_params": quant_manifest.get("total_params"),
        },
        "frames": frames,
    }
    with open(os.path.join(out_dir, "frames.json"), "w") as f:
        json.dump(frames_manifest, f, indent=2)
    return frames_manifest
