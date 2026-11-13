#!/usr/bin/env python3
"""Submit existing zdx-ban Candidate JSONL traces to a licensed federation.

By default sends metadata + hashes, NOT private prompt/answer text.
The central service treats every submission as STAGED, never trained.
"""
from __future__ import annotations
import argparse
import hashlib
import json
import os
import re
import sys
import urllib.error
import urllib.request
from pathlib import Path
from urllib.parse import urlparse

from control_plane import encoded, safe_url, valid_id

ALLOWED_OUTCOMES = {"SUPPORTED", "CONTRADICTED", "UNSUPPORTED", "INCONCLUSIVE", "NOT_MEASURED", "FAILED", "ERROR"}
SHA = re.compile(r"^[a-f0-9]{64}$")

def fingerprint(text):
    return hashlib.sha256((text or "").encode("utf-8")).hexdigest()

def contribution(candidate, tenant, node, model_state, include_content=False):
    if not isinstance(candidate, dict):
        raise ValueError("candidate must be an object")
    candidate_id = candidate.get("ID", "")
    run_id = candidate.get("SourceRun", "")
    if not candidate_id or not run_id:
        raise ValueError("BAN candidate missing ID/SourceRun")
    proven = candidate.get("Provenance") or {}
    measurements = proven.get("MeasurementIDs") or []
    outcome = candidate.get("MeasurementOutcome") or "NOT_MEASURED"
    if outcome not in ALLOWED_OUTCOMES:
        raise ValueError("unknown BAN measurement outcome")
    submission_id = "ban-" + fingerprint(str(candidate_id) + "\0" + str(run_id) + "\0" + model_state["w0_id"])[:32]
    data = {
        "candidate_id": str(candidate_id)[:128],
        "source_run": str(run_id)[:128],
        "source_graph_node": str(candidate.get("SourceGraphNode", ""))[:128],
        "input_sha256": fingerprint(candidate.get("Input", "")),
        "output_sha256": fingerprint(candidate.get("ModelOutput", "")),
        "target": candidate.get("Target", "UNKNOWN"),
        "validation_state": candidate.get("ValidationState", "OBSERVED"),
        "memory_ids": (candidate.get("MemoryAttribution") or [])[:32],
        "repetition_count": candidate.get("RepetitionCount", 0),
    }
    if include_content:
        data["input"] = candidate.get("Input", "")
        data["output"] = candidate.get("ModelOutput", "")
    return {
        "schema_version": 1, "submission_id": submission_id,
        "tenant": tenant, "node": node, "kind": "experience",
        "model_state": model_state,
        "evidence": {"outcome": outcome, "measurement_ids": measurements[:64]},
        "data": data,
    }

def main():
    p = argparse.ArgumentParser(description="ZDXBAN edge trace contribution exporter")
    p.add_argument("--candidates", type=Path, required=True, help="Existing traces/*.candidates.jsonl")
    p.add_argument("--tenant", required=True)
    p.add_argument("--node", required=True)
    p.add_argument("--w0-id", required=True, help="Exact foundation model identity (can be declared external model)")
    p.add_argument("--w0-sha256")
    p.add_argument("--w1-sha256")
    p.add_argument("--url", help="Federation HTTPS URL, or loopback HTTP for tests")
    p.add_argument("--token-env", default="BAN_FEDERATION_NODE_TOKEN")
    p.add_argument("--dry-run", action="store_true", help="Validate and count; no network calls")
    p.add_argument("--include-content", action="store_true", help="Opt in to transmitting raw prompt/output; NOT recommended for private data")
    args = p.parse_args()
    valid_id(args.tenant, "tenant")
    valid_id(args.node, "node")
    valid_id(args.w0_id, "w0-id")
    model = {"w0_id": args.w0_id}
    for name in ("w0_sha256", "w1_sha256"):
        value = getattr(args, name)
        if value:
            if not SHA.fullmatch(value):
                p.error("invalid " + name)
            model[name] = value
    if not args.dry_run and (not args.url or not safe_url(args.url)):
        p.error("a secure --url (HTTPS or loopback HTTP) is required")
    token = os.environ.get(args.token_env, "")
    if not args.dry_run and len(token) < 32:
        p.error("node bearer token missing from " + args.token_env)
    totals = {"staged": 0, "duplicates": 0, "invalid": 0}
    with args.candidates.open(encoding="utf-8") as file:
        for line_num, line in enumerate(file, 1):
            if not line.strip():
                continue
            try:
                candidate = json.loads(line)
                body = contribution(candidate, args.tenant, args.node, model, args.include_content)
                if len(encoded(body)) > 256 * 1024:
                    raise ValueError("candidate exceeds transport limit")
                if args.dry_run:
                    totals["staged"] += 1
                    continue
                req = urllib.request.Request(args.url.rstrip("/") + "/v1/federation/contributions",
                                             data=encoded(body), method="POST",
                                             headers={"Authorization": "Bearer " + token, "Content-Type": "application/json"})
                try:
                    with urllib.request.urlopen(req, timeout=10) as resp:
                        outcome = json.loads(resp.read(4096))
                        if outcome.get("status") != "STAGED":
                            raise ValueError("unexpected receipt")
                        totals["duplicates" if outcome.get("duplicate") else "staged"] += 1
                except urllib.error.HTTPError as e:
                    raise ValueError("federation rejected with HTTP " + str(e.code)) from e
            except (ValueError, KeyError, TypeError, json.JSONDecodeError) as e:
                totals["invalid"] += 1
                print("line %d: %s" % (line_num, e), file=sys.stderr)
    print(json.dumps(totals, sort_keys=True))
    if totals["invalid"]:
        raise SystemExit(1)

if __name__ == "__main__":
    main()
