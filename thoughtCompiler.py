#!/usr/bin/env python3
"""
ZDX ThoughtCompiler v1.0
Entropy-based text -> Pyxel VM brain frame compiler
Usage: python3 thought_compiler.py "your text here" [--size 512] [--output brain_frame.png]
"""

import argparse
import json
import math
import os
import re
import sys
import subprocess
import numpy as np
from collections import Counter
from datetime import datetime
from PIL import Image

subprocess.check_call([
    sys.executable,
    "-m",
    "pip",
    "install",
    "pillow",
    "numpy"
])

# ── ZDX Pyxel ISA ──────────────────────────────────────────────────────────────
OPCODES = {
    "SET_REG":     (10, 0, 0),
    "ADD_THREAD":  (20, 0, 0),
    "SAVE_STATE":  (0, 255, 0),
    "VERIFY_FREQ": (50, 50, 50),
    "HALT":        (255, 255, 255),
    "NOP":         (0, 0, 0),
}

STOP_WORDS = set("""
a an the is are was were be been being have has had do does did will would could should
may might shall can need dare used to and or but if in on at by for of with as from
that this these those it its i you he she we they me him her us them my your his our their
what which who whom when where why how all any both each few more most other some such
no nor not only same so than too very just into up out down through over about
""".split())

AUTH_FREQ = 142
AUTH_ROW = 7

def tokenize(text):
    return re.findall(r"[a-zA-Z0-9'_-]+", text.lower())

def word_entropy(word, corpus_freq, total_words):
    """Shannon entropy proxy: rare words = high information density"""
    p = corpus_freq.get(word, 1) / total_words
    return -math.log2(p)

def text_to_opcodes(text, size):
    """
    Entropy-based compilation:
    - High entropy (rare, meaningful) words  -> SET_REG with value = entropy * 10
    - Medium entropy words                   -> ADD_THREAD
    - Low entropy / stop words               -> NOP
    - Sentence boundaries                    -> SAVE_STATE
    - Questions / negations                  -> VERIFY_FREQ
    - End of input                           -> HALT
    """
    tokens = tokenize(text)
    sentences = re.split(r'[.!?]+', text)
    freq = Counter(tokens)
    total = max(len(tokens), 1)

    # Compute entropy per token
    token_entropy = {}
    for t in set(tokens):
        token_entropy[t] = word_entropy(t, freq, total)

    max_entropy = max(token_entropy.values()) if token_entropy else 1.0
    min_entropy = min(token_entropy.values()) if token_entropy else 0.0

    # If all words have identical entropy, treat them as maximally informative
    # instead of collapsing everything to NOP.
    
    if abs(max_entropy - min_entropy) < 1e-9:
       entropy_range = None
    else:
   
       entropy_range = max_entropy - min_entropy
   
    # Build instruction stream per sentence (Y=thread, X=clock)
    instruction_grid = {}  # (x, y) -> (R, G, B)
    thread_metadata = {}   # y -> {sentence, entropy_sum, word_count}

    x_cursor = 0
    for y, sentence in enumerate(sentences[:size]):
        if not sentence.strip():
            continue
        words = tokenize(sentence)
        if not words:
            continue

        thread_metadata[y] = {
            "sentence": sentence.strip()[:80],
            "words": len(words),
            "entropy_sum": 0,
            "opcodes": Counter()
        }

        for word in words:
            if x_cursor >= size - 1:
                x_cursor = 0

            ent = token_entropy.get(word, min_entropy)
            if entropy_range is None:
               norm = 1.0
            else:
               norm = (ent - min_entropy) / entropy_range

            # Check for special tokens
            is_question = word in ("what", "why", "how", "when", "where", "who", "which")
            is_negation = word in ("not", "no", "never", "none", "nothing", "without", "cant", "dont", "wont")
            is_stop = word in STOP_WORDS

            if is_question or is_negation:
                opcode = OPCODES["VERIFY_FREQ"]
                op_name = "VERIFY_FREQ"
            elif is_stop or norm < 0.25:
                opcode = OPCODES["NOP"]
                op_name = "NOP"
            elif norm < 0.5:
                opcode = OPCODES["ADD_THREAD"]
                op_name = "ADD_THREAD"
            else:
                # SET_REG: encode entropy magnitude in B channel
                val = min(255, int(norm * 255))
                opcode = (10, 0, val)
                op_name = "SET_REG"

            instruction_grid[(x_cursor, y)] = opcode
            thread_metadata[y]["entropy_sum"] += ent
            thread_metadata[y]["opcodes"][op_name] += 1
            x_cursor += 1

        # Sentence boundary -> SAVE_STATE
        instruction_grid[(x_cursor, y)] = OPCODES["SAVE_STATE"]
        thread_metadata[y]["opcodes"]["SAVE_STATE"] += 1
        x_cursor += 1

    # Final token -> HALT
    if x_cursor < size:
        instruction_grid[(x_cursor, min(len(sentences)-1, size-1))] = OPCODES["HALT"]

    return instruction_grid, thread_metadata, token_entropy

def build_frame(instruction_grid, thread_metadata, size, secret_freq=AUTH_FREQ):
    """Render instruction grid to SIZE x SIZE PNG"""
    frame = np.zeros((size, size, 3), dtype=np.uint8)

    # Place instructions
    for (x, y), rgb in instruction_grid.items():
        if 0 <= x < size and 0 <= y < size:
            frame[y, x] = rgb

    # AUTH row: VERIFY_FREQ mirrored pattern
    for x in range(size):
        frame[AUTH_ROW, x] = [secret_freq, 0, x % 255]
    for x in range(size // 2):
        frame[AUTH_ROW, size - 1 - x] = frame[AUTH_ROW, x].copy()

    # SAVE_STATE at last column (daisy-chain trigger)
    for y in range(size):
        if y != AUTH_ROW:
            frame[y, size - 1] = OPCODES["SAVE_STATE"]

    return frame

def execution_report(frame, instruction_grid, thread_metadata, token_entropy, text, size):
    """Run VM and produce report dict"""
    registers = np.zeros((size, 3), dtype=np.int32)
    op_counts = Counter()
    auth_matches = 0

    for x in range(size):
        col = frame[:, x, :]
        set_mask = col[:, 0] == 10
        registers[set_mask, 0] = col[set_mask, 2]
        op_counts["SET_REG"] += int(set_mask.sum())

        add_mask = (col[:, 0] == 20) & (col[:, 1] == 0) & (col[:, 2] == 0)
        registers[add_mask, 2] = registers[add_mask, 0] + registers[add_mask, 1]
        op_counts["ADD_THREAD"] += int(add_mask.sum())

        halt_mask = (col[:, 0] == 255) & (col[:, 1] == 255) & (col[:, 2] == 255)
        op_counts["HALT"] += int(halt_mask.sum())

        save_mask = (col[:, 0] == 0) & (col[:, 1] == 255) & (col[:, 2] == 0)
        op_counts["SAVE_STATE"] += int(save_mask.sum())

        vf_mask = (col[:, 0] == 50) & (col[:, 1] == 50) & (col[:, 2] == 50)
        op_counts["VERIFY_FREQ"] += int(vf_mask.sum())
        
        nop_mask = (
          (col[:, 0] == 0) &
          (col[:, 1] == 0) &
          (col[:, 2] == 0)
        )
        op_counts["NOP"] += int(nop_mask.sum())

    # Auth check
    row7 = frame[AUTH_ROW]
    for x in range(size):
        if np.array_equal(row7[x], row7[size - 1 - x]):
            auth_matches += 1

    # Top entropy words
    top_words = sorted(token_entropy.items(), key=lambda x: x[1], reverse=True)[:15]

    # Thread summary
    active_threads = {k: v for k, v in thread_metadata.items() if v["words"] > 0}

    return {
        "compiled_at": datetime.utcnow().isoformat() + "Z",
        "input_length": len(text),
        "frame_size": f"{size}x{size}",
        "auth_freq": AUTH_FREQ,
        "auth_status": "AUTHORIZED" if auth_matches > size // 2 else "DENIED",
        "auth_mirror_matches": auth_matches,
        "opcodes_fired": dict(op_counts),
        "register_out_range": [int(registers[:, 2].min()), int(registers[:, 2].max())],
        "register_out_mean": round(float(registers[:, 2].mean()), 3),
        "active_threads": len(active_threads),
        "top_entropy_words": [{"word": w, "entropy": round(e, 3)} for w, e in top_words],
        "threads": {
            str(y): {
                "sentence": m["sentence"],
                "words": m["words"],
                "entropy_sum": round(m["entropy_sum"], 3),
                "opcodes": dict(m["opcodes"])
            }
            for y, m in list(active_threads.items())[:20]
        }
    }

def compile_text(text, size=512, output="brain_frame.png", report_path=None):
    print(f"[ZDX ThoughtCompiler] Input: {len(text)} chars")
    print(f"[ZDX ThoughtCompiler] Frame size: {size}x{size}")

    grid, thread_meta, token_ent = text_to_opcodes(text, size)
    print(f"[ZDX ThoughtCompiler] Instructions placed: {len(grid)}")

    frame = build_frame(grid, thread_meta, size)
    img = Image.fromarray(frame, "RGB")
    img.save(output)
    print(f"[ZDX ThoughtCompiler] Brain frame saved: {output}")

    report = execution_report(frame, grid, thread_meta, token_ent, text, size)

    rpath = report_path or output.replace(".png", "_report.json")
    with open(rpath, "w") as f:
        json.dump(report, f, indent=2)
    print(f"[ZDX ThoughtCompiler] Execution report: {rpath}")

    # Print summary
    print()
    print("=== EXECUTION SUMMARY ===")
    print(f"  Auth:          {report['auth_status']} ({report['auth_mirror_matches']}/{size} mirror matches)")
    print(f"  Active threads:{report['active_threads']}")
    print(f"  SET_REG:       {report['opcodes_fired'].get('SET_REG', 0):,}")
    print(f"  ADD_THREAD:    {report['opcodes_fired'].get('ADD_THREAD', 0):,}")
    print(f"  SAVE_STATE:    {report['opcodes_fired'].get('SAVE_STATE', 0):,}")
    print(f"  VERIFY_FREQ:   {report['opcodes_fired'].get('VERIFY_FREQ', 0):,}")
    print(f"  OUT range:     {report['register_out_range']}")
    print(f"  NOP:           {report['opcodes_fired'].get('NOP', 0):,}")
    print()
    print("  Top entropy words (highest information density):")
    for item in report["top_entropy_words"][:8]:
        bar = "█" * int(item["entropy"])
        print(f"    {item['word']:<20} {item['entropy']:.2f}  {bar}")

    return output, rpath

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="ZDX ThoughtCompiler — text to Pyxel VM brain frame")
    parser.add_argument("text", nargs="?", help="Text to compile (or pipe via stdin)")
    parser.add_argument("--size", type=int, default=512, help="Frame size (default: 512)")
    parser.add_argument("--output", default="brain_frame.png", help="Output PNG path")
    parser.add_argument("--report", default=None, help="Report JSON path")
    args = parser.parse_args()

    if args.text:
        text = args.text
    elif not sys.stdin.isatty():
        text = sys.stdin.read()
    else:
        print("Usage: python3 thought_compiler.py 'your text here' [--size 512]")
        sys.exit(1)

    compile_text(text, size=args.size, output=args.output, report_path=args.report)
