import json
import os
import sys
import tempfile
import threading
import unittest
import urllib.error
import urllib.request
from http.server import ThreadingHTTPServer
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from control_plane import Registry, Store, make_handler, encoded, validate
from node import contribution


TOKEN_A = "A" * 48
TOKEN_B = "B" * 48
TOKEN_R = "R" * 48


def example(tenant="alpha", node="edge1", outcome="SUPPORTED", item="exp1"):
    return {"schema_version": 1, "tenant": tenant, "node": node, "submission_id": item,
            "kind": "experience", "model_state": {"w0_id": "qwen3:0.6b"},
            "evidence": {"outcome": outcome, "measurement_ids": ["m1"]},
            "data": {"source_run": "run1", "input_sha256": "a" * 64}}


class TestFederation(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        registry = Registry({"deployments": {
            "alpha": {"nodes": {
                "edge1": {"role": "edge", "token_env": "BAN_EDGE_A"},
                "review1": {"role": "reviewer", "token_env": "BAN_REVIEW_A"}}},
            "beta": {"nodes": {"edge2": {"role": "edge", "token_env": "BAN_EDGE_B"}}}}},
            {"BAN_EDGE_A": TOKEN_A, "BAN_EDGE_B": TOKEN_B, "BAN_REVIEW_A": TOKEN_R})
        self.db = Store(str(Path(self.tmp.name) / "federation.sqlite"))
        self.http = ThreadingHTTPServer(("127.0.0.1", 0), make_handler(self.db, registry))
        self.thread = threading.Thread(target=self.http.serve_forever, daemon=True)
        self.thread.start()
        self.base = "http://127.0.0.1:%d" % self.http.server_address[1]

    def tearDown(self):
        self.http.shutdown()
        self.http.server_close()
        self.thread.join(timeout=2)
        self.tmp.cleanup()

    def call(self, method, route, token=None, body=None):
        data = encoded(body) if body is not None else None
        headers = {"Content-Type": "application/json"}
        if token:
            headers["Authorization"] = "Bearer " + token
        request = urllib.request.Request(self.base + route, data=data, headers=headers, method=method)
        try:
            with urllib.request.urlopen(request, timeout=3) as response:
                return response.status, json.loads(response.read())
        except urllib.error.HTTPError as e:
            return e.code, json.loads(e.read())

    def test_auth_tenant_isolation_idempotence(self):
        status, _ = self.call("POST", "/v1/federation/contributions", body=example())
        self.assertEqual(status, 401)
        status, _ = self.call("POST", "/v1/federation/contributions", TOKEN_B, example())
        self.assertEqual(status, 400)
        status, receipt = self.call("POST", "/v1/federation/contributions", TOKEN_A, example())
        self.assertEqual(status, 201)
        self.assertEqual(receipt["status"], "STAGED")
        self.assertEqual(receipt["weight_training"], "DISABLED")
        status, again = self.call("POST", "/v1/federation/contributions", TOKEN_A, example())
        self.assertEqual(status, 200)
        self.assertTrue(again["duplicate"])
        altered = example()
        altered["data"]["input_sha256"] = "b" * 64
        status, _ = self.call("POST", "/v1/federation/contributions", TOKEN_A, altered)
        self.assertEqual(status, 409)
        status, beta = self.call("GET", "/v1/federation/summary", TOKEN_B)
        self.assertEqual(status, 200)
        self.assertEqual(beta["staged"], {})
        status, alpha = self.call("GET", "/v1/federation/summary", TOKEN_A)
        self.assertEqual(alpha["staged"], {"experience": 1})

    def test_reviewer_is_not_edge_and_review_does_not_activate(self):
        status, _ = self.call("POST", "/v1/federation/contributions", TOKEN_R, example())
        self.assertEqual(status, 403)
        self.assertEqual(self.call("POST", "/v1/federation/contributions", TOKEN_A, example())[0], 201)
        self.assertEqual(self.call("GET", "/v1/federation/submissions", TOKEN_A)[0], 404)
        bad = {"submission_id": "exp1", "decision": "ELIGIBLE_KNOWLEDGE"}
        self.assertEqual(self.call("POST", "/v1/federation/reviews", TOKEN_R, bad)[0], 400)
        okay = {**bad, "measurement_ref": "measurement:1", "axiomatic_ref": "axiom:1"}
        status, report = self.call("POST", "/v1/federation/reviews", TOKEN_R, okay)
        self.assertEqual(status, 201)
        self.assertIn("No training", report["note"])
        self.assertFalse(self.call("GET", "/v1/federation/summary", TOKEN_R)[1]["automatic_activation"])
        self.assertEqual(self.call("POST", "/v1/federation/sync-memory", TOKEN_A, {})[0], 403)

    def test_cannot_promote_unverified(self):
        observation = example(outcome="NOT_MEASURED")
        self.assertEqual(self.call("POST", "/v1/federation/contributions", TOKEN_A, observation)[0], 201)
        body = {"submission_id": "exp1", "decision": "ELIGIBLE_KNOWLEDGE",
                "measurement_ref": "measurement:1", "axiomatic_ref": "axiom:1"}
        self.assertEqual(self.call("POST", "/v1/federation/reviews", TOKEN_R, body)[0], 400)
        self.assertEqual(self.call("POST", "/v1/federation/reviews", TOKEN_R,
                                   {"submission_id": "exp1", "decision": "HOLD"})[0], 201)

    def test_weight_lineage_and_no_activation(self):
        item = example(item="delta1")
        item["kind"] = "weight_manifest"
        item["data"] = {"layer": "W2", "artifact": {"sha256": "f" * 64, "size_bytes": 100}}
        status, _ = self.call("POST", "/v1/federation/contributions", TOKEN_A, item)
        self.assertEqual(status, 400)
        item["model_state"]["w0_sha256"] = "0" * 64
        status, _ = self.call("POST", "/v1/federation/contributions", TOKEN_A, item)
        self.assertEqual(status, 400)
        item["model_state"]["w1_sha256"] = "1" * 64
        status, result = self.call("POST", "/v1/federation/contributions", TOKEN_A, item)
        self.assertEqual(status, 201)
        self.assertEqual(result["status"], "STAGED")
        self.assertEqual(self.call("POST", "/v1/federation/reviews", TOKEN_R,
                                   {"submission_id": "delta1", "decision": "ELIGIBLE_KNOWLEDGE",
                                    "measurement_ref": "m1", "axiomatic_ref": "a1"})[0], 400)

    def test_exporter_does_not_leak_raw_prompt_by_default(self):
        source = {"ID": "tc-1", "SourceRun": "trace1", "MeasurementOutcome": "SUPPORTED",
                  "Input": "private patient text", "ModelOutput": "confidential answer",
                  "Provenance": {"MeasurementIDs": ["formal-42"]}}
        body = contribution(source, "alpha", "edge1", {"w0_id": "qwen3:0.6b"})
        self.assertNotIn("private patient text", json.dumps(body))
        self.assertNotIn("confidential answer", json.dumps(body))
        self.assertEqual(body["evidence"]["measurement_ids"], ["formal-42"])
        validate(body, "alpha", "edge1")

    def test_memory_connector_disabled_without_explicit_tenant_credential(self):
        self.assertEqual(self.call("POST", "/v1/federation/contributions", TOKEN_A, example())[0], 201)
        status, result = self.call("POST", "/v1/federation/sync-memory", TOKEN_R, {})
        self.assertEqual(status, 200)
        self.assertEqual(result["status"], "NOT_CONFIGURED")
        self.assertEqual(result["synced"], 0)


if __name__ == "__main__":
    unittest.main()


class TestMemoryBridge(unittest.TestCase):
    def test_sync_uses_scoped_memory_endpoint_without_raw_content(self):
        from control_plane import sync_memory
        captured = []

        class MemoryHandler(__import__("http.server", fromlist=["BaseHTTPRequestHandler"]).BaseHTTPRequestHandler):
            def log_message(self, *_args):
                pass

            def do_POST(self):
                size = int(self.headers.get("Content-Length", "0"))
                item = json.loads(self.rfile.read(size))
                captured.append((self.path, self.headers.get("Authorization"), item))
                self.send_response(201)
                self.send_header("Content-Length", "2")
                self.end_headers()
                self.wfile.write(b"{}")

        memory_server = ThreadingHTTPServer(("127.0.0.1", 0), MemoryHandler)
        thread = threading.Thread(target=memory_server.serve_forever, daemon=True)
        thread.start()
        try:
            env = {"BAN_TEST_EDGE": TOKEN_A, "BAN_TEST_REVIEW": TOKEN_R,
                   "BAN_TEST_MEMORY": "M" * 48}
            reg = Registry({"deployments": {"alpha": {
                "nodes": {"edge1": {"role": "edge", "token_env": "BAN_TEST_EDGE"},
                          "review1": {"role": "reviewer", "token_env": "BAN_TEST_REVIEW"}},
                "memory": {"url": "http://127.0.0.1:%d" % memory_server.server_address[1],
                           "token_env": "BAN_TEST_MEMORY"}}}}, env)
            with tempfile.TemporaryDirectory() as temp:
                store = Store(str(Path(temp) / "ledger.sqlite"))
                item = example()
                item["data"]["private_text"] = "not for memory"
                store.add(("alpha", "edge1", "edge"), item)
                self.assertEqual(sync_memory(store, reg, "alpha")["synced"], 1)
                self.assertEqual(sync_memory(store, reg, "alpha")["synced"], 0)
                self.assertEqual(len(captured), 1)
                self.assertEqual(captured[0][0], "/v1/memory/upsert")
                self.assertEqual(captured[0][1], "Bearer " + "M" * 48)
                self.assertEqual(captured[0][2]["expectedVersion"], 0)
                self.assertNotIn("not for memory", json.dumps(captured[0][2]))
                self.assertEqual(captured[0][2]["value"]["outcome"], "SUPPORTED")
        finally:
            memory_server.shutdown()
            memory_server.server_close()
            thread.join(timeout=2)
