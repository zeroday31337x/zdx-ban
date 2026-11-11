# Spatial PNG reduction — 8B model to RGBA frames

Reduces a large model (Qwen3-8B) to runnable inference on constrained
hardware by translating what the weights are stored as.

## The pattern

Weights as one big blob force the whole model into RAM. Translate the
weights into spatial PNG frames and stream them one at a time,
daisy-chained, and the RAM needed drops to one frame instead of the
model. Storage becomes many small files instead of one huge one.

- **Representation:** grouped quantization (block 128, one FP16
  scale per block) + RGBA packing (4 bytes/pixel). Two widths are
  supported (`--bits 4` or `--bits 8`): 4-bit packs two values per
  byte (nibbles), INT8 stores one byte per value.
- **Frame:** 4096x4096 RGBA. Rows 0..7 are the execution plane (the VM
  program). Rows 8..4095 are storage: 66,977,792 bytes per frame.
- **Chaining:** each frame's program accumulates its index into shared
  memory M0 and SAVE_STATEs to the next frame, so the VM streams the
  frames one at a time. The 100-frame cap is arbitrary and is raised
  here (constructor parameter, not a code change to the VM).
- **Result (Qwen3-8B, 8.19B params):**
  - 4-bit: ~4.2 GB quantized stream -> 64 RGBA PNG frames (~67 MB
    each), measured peak live RSS during the full 64-frame stream
    ~249 MB (frame data is 67 MB; the peak includes transient PNG
    decode buffers + Python/numpy/PIL overhead) instead of 8 GB —
    about 32x less RAM than holding the whole model.
  - INT8: ~8.3 GB quantized stream -> 125 RGBA PNG frames, measured
    peak live RSS ~255 MB. Same RAM as 4-bit (RAM is bounded by one
    frame, not by the frame count), 2x the storage, ~18x smaller
    weight error and ~25-27x smaller layer-0 block-output error.
    See the comparison at `~/workspace/qwen3-8b-spatial-int8/comparison.json`.
    **Recommendation: INT8 is the better method** (precision wins at
    the same RAM); 4-bit stays as the storage-constrained fallback.

## Provenance

Adapted from the ZDX PyxelVM (spatial PNG model). The pyxel repo is
NOT modified; the relevant pieces were taken into this tool:
- Frame geometry + flat byte read/write: `zdx_spatial_frame.py`
  (adapted RGB -> RGBA, 4 bytes/pixel).
- Pixel codec approach: `zdx_pixel_memory/codec.py`.
- Per-artifact checksum (reliability hook): `zdx_pixel_memory/spatial_store.py`.
- VM execution + SAVE_STATE chaining semantics (opcode values,
  column-major lockstep, registry): `zdx_parallel_vm.py` — re-implemented
  as a lean executor in `stream.py` (the full VM pulls in a deep
  dependency chain; this keeps the tool self-contained).

The alpha channel was originally avoided in the pyxel VM to evade image
scanning detection (not to hide malicious code). Since the tech is open
source, that no longer matters, and RGBA adds +33% capacity per PNG
vs RGB (8B INT8: 159 -> 120 PNGs; 4-bit: ~62 PNGs).

## Layout

- `rgba_frame.py` — RGBA frame geometry, byte I/O, SHA-256.
- `quantize.py` — grouped int4/int8 quantization + dequantization, BF16 handling.
- `pack.py` — stream -> RGBA PNG frames + frames.json + chaining program.
- `unpack.py` — frames -> stream, SHA-256 round-trip verification.
- `stream.py` — lean VM chaining executor + live RAM measurement.
- `run.py` — end-to-end driver (quantize -> pack -> unpack -> dequant -> stream).
  `--bits 4` (default) or `--bits 8`. Stage 5 samples the streaming
  child's RSS externally from /proc/<pid>/status every 0.1s; that
  external peak is authoritative.
- `block_perturb.py` — single-transformer-block (layer 0) perturbation
  comparison: runs the block in numpy on a fixed token input with the
  original, INT8 and 4-bit weights and reports how far each quantized
  output drifts from the original. Not a perplexity; the relative
  INT8-vs-4-bit error is the result.

## Reproduce

```bash
# 1. Download Qwen3-8B (BF16, ~16.4 GB) — needs ~144 GB free; do NOT use
#    the VPS (only 9.9 GB free there).
python -c "from huggingface_hub import snapshot_download; \
  snapshot_download(repo_id='Qwen/Qwen3-8B', local_dir='~/workspace/qwen3-8b', \
  allow_patterns=['*.safetensors','*.json'])"

# 2. Run the full pipeline (quantize, pack, verify, stream).
python run.py --model-dir ~/workspace/qwen3-8b \
              --work-dir ~/workspace/qwen3-8b-spatial \
              --frames-dir ~/workspace/qwen3-8b-spatial/frames \
              --bits 4

# 2b. The INT8 version (same tool, --bits 8).
python run.py --model-dir ~/workspace/qwen3-8b \
              --work-dir ~/workspace/qwen3-8b-spatial-int8 \
              --frames-dir ~/workspace/qwen3-8b-spatial-int8/frames \
              --bits 8

# 3. Read the reports.
cat ~/workspace/qwen3-8b-spatial/report.json
cat ~/workspace/qwen3-8b-spatial-int8/report.json
cat ~/workspace/qwen3-8b-spatial-int8/comparison.json

# 4. Block-perturbation comparison (layer 0, numpy, no inference engine needed).
python block_perturb.py
```

Stages are skipped when their outputs already exist; pass `--force` to
redo. The report records PNG count, total PNG disk size, live RAM
measured during streaming, the round-trip SHA-256 result, and the full
per-tensor dequantization error (global max/mean abs and mean relative).

## Verification (what "proven" means here)

1. **Lossless round-trip:** every frame's storage bytes SHA-256-match
   `frames.json`, and the reassembled stream SHA-256-matches the
   original quantized stream byte-for-byte.
2. **Dequantization error:** every tensor is dequantized from the
   unpacked stream and compared to the original weights; global
   max/mean absolute error and mean relative error are reported.
3. **Streaming + RAM:** the VM visits every frame exactly once via
   SAVE_STATE, M0 equals the expected sum of frame indices (proving
   every frame executed), and peak live RSS during the stream is
   measured by sampling the streaming process externally (~249 MB),
   not the whole 8 GB model. Note: the pipeline's own after-each-frame
   VmRSS reading (~31 MB) under-reports because it is sampled after
   the frame array is freed; one pipeline run also showed an anomalous
   ru_maxrss of 4.07 GB that was NOT reproduced on direct
   re-measurement (255 MB). The externally sampled ~249 MB is the
   authoritative peak.

## Reliability (future)

Once the reduction is perfected, reliability (error detection /
correction, redundancy) may be needed and will be explored then.
Pixel binning (the low-light photography technique) does NOT help:
it combines N pixels into 1, reducing storage slots, and PNG storage
is lossless so there is no sensor noise to reduce.
