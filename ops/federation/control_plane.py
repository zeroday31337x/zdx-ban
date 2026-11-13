#!/usr/bin/env python3
"""ZDXBAN federation: tenant-isolated, append-only contribution staging control plane.
No autonomous weight training, promotion or activation occurs here.
"""
from __future__ import annotations
import argparse
from contextlib import contextmanager
import hashlib
import hmac
import json
import os
import re
import sqlite3
import threading
import urllib.error
import urllib.request
from datetime import datetime, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import parse_qs, urlparse

MAX_BODY = 262144
NAME = re.compile(r"^[a-zA-Z0-9][a-zA-Z0-9_.:-]{0,95}$")
HASH = re.compile(r"^[a-f0-9]{64}$")
OUTCOMES = {"SUPPORTED", "CONTRADICTED", "UNSUPPORTED", "INCONCLUSIVE", "NOT_MEASURED", "FAILED", "ERROR"}

def timestamp():
    return datetime.now(timezone.utc).isoformat()

def encoded(data):
    return json.dumps(data, sort_keys=True, separators=(",", ":"), ensure_ascii=False, allow_nan=False).encode()

def checksum(data):
    return hashlib.sha256(encoded(data)).hexdigest()

def valid_id(value, label):
    if not isinstance(value, str) or not NAME.fullmatch(value):
        raise ValueError("invalid " + label)
    return value

def safe_url(url):
    parsed = urlparse(url)
    return parsed.scheme == "https" or (parsed.scheme == "http" and parsed.hostname in ("127.0.0.1", "localhost", "::1"))

class Registry:
    """Node and tenant identities come from server-held credentials, never body fields."""
    def __init__(self, config, environ=None):
        env = os.environ if environ is None else environ
        self.nodes, self.memory = {}, {}
        deployments = config.get("deployments")
        if not isinstance(deployments, dict) or not deployments:
            raise ValueError("registry needs deployments")
        for tenant, spec in deployments.items():
            valid_id(tenant, "tenant")
            entries = spec.get("nodes", {})
            if not isinstance(entries, dict) or not entries:
                raise ValueError("deployment needs nodes")
            for node, cfg in entries.items():
                valid_id(node, "node")
                role, key = cfg.get("role"), cfg.get("token_env")
                if role not in ("edge", "reviewer") or not isinstance(key, str) or not key.startswith("BAN_"):
                    raise ValueError("invalid role or token_env")
                secret = env.get(key, "")
                if len(secret) < 32:
                    raise ValueError("missing strong credential in " + key)
                digest = hashlib.sha256(secret.encode()).digest()
                if digest in self.nodes:
                    raise ValueError("duplicate node credential")
                self.nodes[digest] = (tenant, node, role)
            mem = spec.get("memory")
            if mem:
                url, key = mem.get("url", ""), mem.get("token_env")
                if not safe_url(url) or not isinstance(key, str) or not key.startswith("BAN_") or len(env.get(key, "")) < 32:
                    raise ValueError("invalid isolated memory connector")
                self.memory[tenant] = (url.rstrip("/"), env[key])

    def auth(self, header):
        if not header.startswith("Bearer ") or len(header) > 4103:
            return None
        provided = hashlib.sha256(header[7:].encode()).digest()
        for key, scope in self.nodes.items():
            if hmac.compare_digest(key, provided):
                return scope
        return None

def validate(item, tenant, node):
    if not isinstance(item, dict) or item.get("schema_version") != 1:
        raise ValueError("invalid schema")
    valid_id(item.get("submission_id"), "submission_id")
    if item.get("tenant") != tenant or item.get("node") != node:
        raise ValueError("authenticated scope mismatch")
    if item.get("kind") not in ("experience", "weight_manifest"):
        raise ValueError("invalid kind")
    model = item.get("model_state")
    if not isinstance(model, dict):
        raise ValueError("model state required")
    valid_id(model.get("w0_id"), "w0_id")
    for field in ("w0_sha256", "w1_sha256", "w2_sha256"):
        if field in model and (not isinstance(model[field], str) or not HASH.fullmatch(model[field])):
            raise ValueError("invalid " + field)
    evidence = item.get("evidence")
    if not isinstance(evidence, dict) or evidence.get("outcome") not in OUTCOMES:
        raise ValueError("evidence outcome required")
    if "measurement_ids" in evidence:
        values = evidence["measurement_ids"]
        if not isinstance(values, list) or len(values) > 64 or any(not isinstance(v, str) or len(v) > 128 for v in values):
            raise ValueError("invalid measurement_ids")
    data = item.get("data")
    if not isinstance(data, dict):
        raise ValueError("data must be a mapping")
    if item["kind"] == "weight_manifest":
        art = data.get("artifact")
        if not isinstance(art, dict) or not isinstance(art.get("sha256"), str) or not HASH.fullmatch(art["sha256"]) or type(art.get("size_bytes")) is not int or art["size_bytes"] < 1:
            raise ValueError("weight artifact requires hash and positive size")
        if "w0_sha256" not in model or data.get("layer") not in ("W1", "W2"):
            raise ValueError("invalid weight lineage")
        if data["layer"] == "W2" and "w1_sha256" not in model:
            raise ValueError("W2 must bind exact W1 hash")
    return item

class Store:
    """Append-only observations, append-only review events, append-only sync attempts."""
    def __init__(self, filename):
        self.filename = filename
        self.lock = threading.RLock()
        Path(filename).parent.mkdir(parents=True, exist_ok=True)
        with self.open() as db:
            db.executescript("""
                CREATE TABLE IF NOT EXISTS contributions(
                  tenant TEXT NOT NULL,node TEXT NOT NULL,submission_id TEXT NOT NULL,
                  kind TEXT NOT NULL,outcome TEXT NOT NULL,body_sha256 TEXT NOT NULL,
                  body TEXT NOT NULL,received_at TEXT NOT NULL,
                  PRIMARY KEY(tenant,submission_id));
                CREATE TABLE IF NOT EXISTS reviews(
                  seq INTEGER PRIMARY KEY AUTOINCREMENT,tenant TEXT NOT NULL,
                  submission_id TEXT NOT NULL,reviewer TEXT NOT NULL,
                  decision TEXT NOT NULL,measurement_ref TEXT,axiomatic_ref TEXT,
                  received_at TEXT NOT NULL);
                CREATE TABLE IF NOT EXISTS sync_events(
                  seq INTEGER PRIMARY KEY AUTOINCREMENT,tenant TEXT NOT NULL,
                  submission_id TEXT NOT NULL,success INTEGER NOT NULL,
                  received_at TEXT NOT NULL);
            """)

    @contextmanager
    def open(self):
        db = sqlite3.connect(self.filename, timeout=10)
        db.execute("PRAGMA busy_timeout=10000")
        try:
            yield db
            db.commit()
        except Exception:
            db.rollback()
            raise
        finally:
            db.close()

    def add(self, scope, item):
        tenant, node, _ = scope
        sha = checksum(item)
        with self.lock, self.open() as db:
            try:
                db.execute("INSERT INTO contributions VALUES(?,?,?,?,?,?,?,?)",
                           (tenant, node, item["submission_id"], item["kind"],
                            item["evidence"]["outcome"], sha, encoded(item).decode(), timestamp()))
                return False, sha
            except sqlite3.IntegrityError:
                prev = db.execute("SELECT node,body_sha256 FROM contributions WHERE tenant=? AND submission_id=?",
                                  (tenant, item["submission_id"])).fetchone()
                if prev == (node, sha):
                    return True, sha
                raise ValueError("conflicting submission_id")

    def summary(self, tenant):
        with self.lock, self.open() as db:
            staged = db.execute("SELECT kind,COUNT(*) FROM contributions WHERE tenant=? GROUP BY kind", (tenant,)).fetchall()
            reviewed = db.execute("SELECT decision,COUNT(*) FROM reviews WHERE tenant=? GROUP BY decision", (tenant,)).fetchall()
        return {"tenant": tenant, "staged": dict(staged), "review_events": dict(reviewed),
                "weight_training": "NOT_IMPLEMENTED", "automatic_activation": False}

    def list(self, tenant, limit=100):
        with self.lock, self.open() as db:
            records = db.execute("SELECT node,submission_id,kind,outcome,body_sha256,received_at FROM contributions WHERE tenant=? ORDER BY received_at DESC LIMIT ?",
                                 (tenant, max(1, min(limit, 200)))).fetchall()
        return [{"node": n, "submission_id": i, "kind": k, "outcome": o, "sha256": h,
                 "received_at": t} for n, i, k, o, h, t in records]

    def review(self, tenant, reviewer, item):
        submission_id = valid_id(item.get("submission_id"), "submission_id")
        decision = item.get("decision")
        if decision not in ("HOLD", "REJECT", "ELIGIBLE_KNOWLEDGE"):
            raise ValueError("unsupported review decision")
        with self.lock, self.open() as db:
            prior = db.execute("SELECT kind,outcome FROM contributions WHERE tenant=? AND submission_id=?",
                               (tenant, submission_id)).fetchone()
            if not prior:
                raise KeyError(submission_id)
            if decision == "ELIGIBLE_KNOWLEDGE":
                if prior != ("experience", "SUPPORTED"):
                    raise ValueError("knowledge eligibility requires supported experience")
                for field in ("measurement_ref", "axiomatic_ref"):
                    valid_id(item.get(field), field)
            db.execute("INSERT INTO reviews(tenant,submission_id,reviewer,decision,measurement_ref,axiomatic_ref,received_at) VALUES(?,?,?,?,?,?,?)",
                       (tenant, submission_id, reviewer, decision, item.get("measurement_ref"),
                        item.get("axiomatic_ref"), timestamp()))
        return {"submission_id": submission_id, "decision": decision, "note": "No training, merging, or activation"}

    def pending(self, tenant):
        with self.lock, self.open() as db:
            return db.execute("""
                SELECT node,submission_id,kind,outcome,body_sha256,body FROM contributions c
                WHERE tenant=? AND NOT EXISTS(
                  SELECT 1 FROM sync_events s WHERE s.tenant=c.tenant
                  AND s.submission_id=c.submission_id AND s.success=1)
                ORDER BY received_at LIMIT 100""", (tenant,)).fetchall()

    def sync_result(self, tenant, submission_id, success):
        with self.lock, self.open() as db:
            db.execute("INSERT INTO sync_events(tenant,submission_id,success,received_at) VALUES(?,?,?,?)",
                       (tenant, submission_id, int(success), timestamp()))

def sync_memory(store, registry, tenant):
    if tenant not in registry.memory:
        return {"status": "NOT_CONFIGURED", "synced": 0, "failed": 0}
    url, token = registry.memory[tenant]
    ok_count = failed = 0
    for node, item, kind, outcome, sha, serialized in store.pending(tenant):
        payload = json.loads(serialized)
        record = {"key": "ban:federation:" + tenant + ":" + item, "kind": "episodic",
                  "value": {"node": node, "submission_id": item, "outcome": outcome,
                            "kind": kind, "sha256": sha, "model_state": payload["model_state"]},
                  "tags": ["ban", "federation", "staged"],
                  "source": "zdx-ban-federation", "references": [],
                  "expectedVersion": 0}
        req = urllib.request.Request(url + "/v1/memory/upsert", data=encoded(record), method="POST",
                                     headers={"Authorization": "Bearer " + token, "Content-Type": "application/json"})
        try:
            with urllib.request.urlopen(req, timeout=4) as response:
                success = 200 <= response.status < 300
        except (urllib.error.URLError, OSError, TimeoutError):
            success = False
        store.sync_result(tenant, item, success)
        ok_count += int(success)
        failed += int(not success)
    return {"status": "ATTEMPTED", "synced": ok_count, "failed": failed}

def make_handler(store, registry):
    class Handler(BaseHTTPRequestHandler):
        server_version = "ZDXBAN-Federation/0.1"

        def log_message(self, *_args):
            pass  # Do not log credentials or user material.

        def respond(self, status, data):
            body = encoded(data)
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Cache-Control", "no-store")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def identity(self):
            scope = registry.auth(self.headers.get("Authorization", ""))
            if scope is None:
                self.respond(401, {"error": "unauthorized"})
            return scope

        def read_body(self):
            try:
                size = int(self.headers.get("Content-Length", "-1"))
            except ValueError:
                size = -1
            if size < 0 or size > MAX_BODY:
                raise ValueError("request size invalid")
            body = json.loads(self.rfile.read(size))
            if not isinstance(body, dict):
                raise ValueError("JSON object required")
            return body

        def do_GET(self):
            if self.path == "/healthz":
                return self.respond(200, {"status": "running", "automatic_promotion": False})
            scope = self.identity()
            if not scope:
                return
            tenant, _node, role = scope
            path = urlparse(self.path)
            if path.path == "/v1/federation/summary":
                return self.respond(200, store.summary(tenant))
            if path.path == "/v1/federation/submissions" and role == "reviewer":
                try:
                    limit = int(parse_qs(path.query).get("limit", ["100"])[0])
                except ValueError:
                    return self.respond(400, {"error": "bad limit"})
                return self.respond(200, {"submissions": store.list(tenant, limit)})
            return self.respond(404, {"error": "not_found"})

        def do_POST(self):
            scope = self.identity()
            if not scope:
                return
            tenant, node, role = scope
            try:
                body = self.read_body()
                if self.path == "/v1/federation/contributions" and role == "edge":
                    validate(body, tenant, node)
                    duplicate, sha = store.add(scope, body)
                    return self.respond(200 if duplicate else 201,
                                        {"status": "STAGED", "duplicate": duplicate,
                                         "sha256": sha, "submission_id": body["submission_id"],
                                         "weight_training": "DISABLED"})
                if self.path == "/v1/federation/reviews" and role == "reviewer":
                    return self.respond(201, store.review(tenant, node, body))
                if self.path == "/v1/federation/sync-memory" and role == "reviewer":
                    return self.respond(200, sync_memory(store, registry, tenant))
                return self.respond(403, {"error": "not_authorized_for_operation"})
            except KeyError:
                return self.respond(404, {"error": "unknown submission"})
            except (UnicodeError, json.JSONDecodeError):
                return self.respond(400, {"error": "invalid JSON"})
            except ValueError as error:
                return self.respond(409 if str(error) == "conflicting submission_id" else 400,
                                    {"error": str(error)})
    return Handler

def main():
    p = argparse.ArgumentParser()
    p.add_argument("--registry", required=True)
    p.add_argument("--db", required=True)
    p.add_argument("--bind", default="127.0.0.1")
    p.add_argument("--port", type=int, default=18541)
    args = p.parse_args()
    registry = Registry(json.loads(Path(args.registry).read_text()))
    server = ThreadingHTTPServer((args.bind, args.port), make_handler(Store(args.db), registry))
    print("ZDXBAN federation staging listening at %s:%d" % (args.bind, args.port), flush=True)
    try:
        server.serve_forever()
    finally:
        server.server_close()

if __name__ == "__main__":
    main()
