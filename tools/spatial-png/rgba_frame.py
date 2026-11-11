"""RGBA spatial PNG frame geometry and byte I/O for weight storage.

Adapted from the ZDX PyxelVM spatial PNG model (zdx_spatial_frame.py):
the same geometry contract (execution plane on top, storage rows below,
row-major cell addressing), extended from RGB (3 bytes/pixel) to RGBA
(4 bytes/pixel) so the alpha channel carries weight data too.

The pyxel repo itself is NOT modified. This module is a clean-room
adaptation of its geometry and flat byte read/write approach, living in
the zdx-ban repo for the 8B model reduction.

Frame: 4096x4096 RGBA.
  - Rows 0..7   : execution plane (8 rows x 4096 cols). Holds the VM program.
  - Rows 8..4095: storage plane (4088 rows x 4096 cols x 4 bytes).
  - Storage capacity per frame: 4088 * 4096 * 4 = 66,977,792 bytes (~67.0 MB).
"""

from __future__ import annotations

import hashlib
import io

import numpy as np
from PIL import Image

WIDTH = 4096
HEIGHT = 4096
EXECUTION_ROWS = 8
CHANNELS = 4  # RGBA

STORAGE_ROWS = HEIGHT - EXECUTION_ROWS
STORAGE_CELLS = STORAGE_ROWS * WIDTH
STORAGE_CAPACITY_BYTES = STORAGE_CELLS * CHANNELS  # 66,977,792
TOTAL_CELLS = WIDTH * HEIGHT
TOTAL_CAPACITY_BYTES = TOTAL_CELLS * CHANNELS


def storage_capacity_bytes() -> int:
    """Bytes of weight data one frame's storage plane can hold."""
    return STORAGE_CAPACITY_BYTES


def address(x: int, y: int) -> int:
    """Row-major cell address for coordinate (x, y)."""
    if not (0 <= x < WIDTH and 0 <= y < HEIGHT):
        raise ValueError(f"coordinate ({x},{y}) outside {WIDTH}x{HEIGHT} frame")
    return y * WIDTH + x


def coordinates(addr: int) -> tuple[int, int]:
    """(x, y) for a row-major cell address."""
    if not (0 <= addr < TOTAL_CELLS):
        raise ValueError(f"spatial address out of range: {addr}")
    return addr % WIDTH, addr // WIDTH


def build_frame(storage: bytes, program_cells: dict | None = None) -> Image.Image:
    """Build one RGBA frame.

    storage: up to STORAGE_CAPACITY_BYTES of weight bytes, written
        row-major into rows EXECUTION_ROWS..HEIGHT-1 (4 bytes per pixel).
        Shorter payloads are zero-padded; the manifest records the true
        length, so padding is never mistaken for data.
    program_cells: {(x, y): (r, g, b, a)} for the execution plane
        (rows 0..EXECUTION_ROWS-1). Unset cells are NOP (0,0,0,255).
    """
    if len(storage) > STORAGE_CAPACITY_BYTES:
        raise ValueError(
            f"storage payload {len(storage)} bytes exceeds frame capacity "
            f"{STORAGE_CAPACITY_BYTES}"
        )
    arr = np.zeros((HEIGHT, WIDTH, CHANNELS), dtype=np.uint8)
    # Execution plane: opaque NOP by default (alpha 255, VM ignores alpha).
    arr[:EXECUTION_ROWS, :, 3] = 255
    # Storage plane: flat byte stream, 4 bytes per pixel, row-major.
    flat = np.frombuffer(storage, dtype=np.uint8)
    padded = np.zeros(STORAGE_CAPACITY_BYTES, dtype=np.uint8)
    padded[: len(flat)] = flat
    arr[EXECUTION_ROWS:, :, :] = padded.reshape(STORAGE_ROWS, WIDTH, CHANNELS)
    if program_cells:
        for (x, y), rgba in program_cells.items():
            if not (0 <= y < EXECUTION_ROWS):
                raise ValueError(f"program cell ({x},{y}) is outside the execution plane")
            if not (0 <= x < WIDTH):
                raise ValueError(f"program cell ({x},{y}) is outside the frame")
            if len(rgba) != 4 or any(not 0 <= int(v) <= 255 for v in rgba):
                raise ValueError(f"program cell {rgba} must be 4 bytes in 0..255")
            arr[y, x, :] = np.array(rgba, dtype=np.uint8)
    return Image.fromarray(arr, "RGBA")


def save_frame(image: Image.Image, path: str) -> str:
    """Save a frame as a PNG. compress_level=0: weight bytes are
    high-entropy and do not compress, so storing them avoids wasting
    CPU on zlib for ~0% savings."""
    image.save(path, format="PNG", compress_level=0)
    return path


def read_storage_bytes(path: str, length: int | None = None) -> bytes:
    """Read a frame's storage-plane bytes (row-major, 4 bytes/pixel).

    length: if given, return exactly that many bytes (the manifest's
    true payload length); otherwise return the full storage capacity.
    """
    with Image.open(path) as img:
        if img.format != "PNG":
            raise ValueError(f"spatial frame must be a PNG container, got {img.format}")
        arr = np.array(img.convert("RGBA"), dtype=np.uint8)
    if arr.shape != (HEIGHT, WIDTH, CHANNELS):
        raise ValueError(f"frame shape {arr.shape} does not match {WIDTH}x{HEIGHT}x{CHANNELS}")
    flat = arr[EXECUTION_ROWS:, :, :].reshape(-1).tobytes()
    if length is None:
        return flat
    if length < 0 or length > STORAGE_CAPACITY_BYTES:
        raise ValueError(f"read length {length} outside storage capacity")
    return flat[:length]


def read_program_cell(path: str, x: int, y: int) -> tuple[int, int, int, int]:
    """Read one execution-plane cell as (r, g, b, a)."""
    if not (0 <= y < EXECUTION_ROWS and 0 <= x < WIDTH):
        raise ValueError(f"program cell ({x},{y}) is outside the execution plane")
    with Image.open(path) as img:
        arr = np.array(img.convert("RGBA"), dtype=np.uint8)
    return tuple(int(v) for v in arr[y, x, :])


def storage_sha256(path: str, length: int | None = None) -> str:
    """SHA-256 of a frame's storage-plane bytes (the reliability hook,
    adapted from the pyxel spatial store's per-artifact checksum)."""
    data = read_storage_bytes(path, length=length)
    return hashlib.sha256(data).hexdigest()


def to_png_bytes(image: Image.Image) -> bytes:
    """Encode a frame as a PNG byte stream."""
    buf = io.BytesIO()
    image.save(buf, format="PNG", compress_level=0)
    return buf.getvalue()
