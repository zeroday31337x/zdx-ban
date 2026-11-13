# ZDXBAN Federated Learning: integration slice 0.1

Status: staging control plane and BAN candidate exporter implemented and tested. NOT a working weight trainer, adapter merger, production federated-learning system, or complete intelligence platform.

## Reuse existing infrastructure

- BAN Go harness: produces traces and candidates, including model identity, outcome and provenance.
- ZeroDriveX Inference Pool: existing authenticated local/remote model-routing gateway. Do not start a competing pool or expose model-management endpoints.
- ZDX Memory Pool: existing authenticated append-only API. Central federation can mirror strictly bounded staged contribution metadata through /v1/memory/upsert.
- Axiomatic Runtime: independent authority for consequential actions, with ALLOW/GATE/PRUNE. An LLM's own confidence cannot be substituted for an Axiomatic decision.

This slice adds a tenant-scoped, authenticated contribution inbox. Each edge instance contributes experience hashes / metadata or a weight artifact MANIFEST, never raw weights. The central system records contributions append-only, permits narrow reviewer events, and can synchronize staged metadata to a correctly scoped Knowledge Pool. It cannot authorize, train, merge or activate model weights.

## Customer / licensing boundary

Each licensed deployment must have an isolated registry and ideally a separate control-plane process, database, Memory Pool namespace/credential and backup chain. Never use one broad memory credential for unrelated tenants. The registry maps a deployment to named nodes, an edge or reviewer role, and server-held token environment-variable names.

Example registry.json file:

    {
      "deployments": {
        "demo": {
          "nodes": {
            "edge01": {"role": "edge", "token_env": "BAN_DEMO_EDGE01_TOKEN"},
            "review01": {"role": "reviewer", "token_env": "BAN_DEMO_REVIEW01_TOKEN"}
          },
          "memory": {
            "url": "http://127.0.0.1:11449",
            "token_env": "BAN_DEMO_MEMORY_TOKEN"
          }
        }
      }
    }

All tokens are random 32+ characters, held only by the server/agent via secrets management. Omit the optional memory block for a fully isolated local-only test. The Memory Pool role of this example must be restricted to the demo namespace.

Central staging server, bound to loopback:

    python3 ops/federation/control_plane.py \
      --registry /etc/zdxban/federation-registry.json \
      --db /var/lib/zdxban-federation/ledger.sqlite \
      --bind 127.0.0.1 --port 18541

Use authenticated HTTPS gateway or a private encrypted tunnel between machines. Never expose the default port publicly. Local credentials must not be committed to the repository.

After the existing BAN harness produces candidates, an edge node can submit them:

    export BAN_FEDERATION_NODE_TOKEN='from-secret-manager'
    python3 ops/federation/node.py \
      --tenant demo --node edge01 --w0-id 'qwen3:0.6b' \
      --candidates traces/EXAMPLE.candidates.jsonl \
      --url https://licensed-private-gateway.example.com

Use --dry-run before sending. Raw prompt and model-answer texts are not transmitted unless --include-content is explicitly requested. Hashes alone are not reconstructible learning examples: enabling actual content sharing requires explicit data rights and a separate privacy review.

## API contract

- GET /healthz: liveness, not upstream readiness.
- POST /v1/federation/contributions: edge role, append-only, idempotent; every receipt remains STAGED.
- GET /v1/federation/summary: own-deployment counts only.
- GET /v1/federation/submissions: scoped reviewer metadata only.
- POST /v1/federation/reviews: reviewer HOLD, REJECT or ELIGIBLE_KNOWLEDGE event.
- POST /v1/federation/sync-memory: reviewer-only retry of staged metadata into tenant Memory Pool.

ELIGIBLE_KNOWLEDGE requires a reported SUPPORTED outcome and measurement/Axiomatic reference IDs, but the central controller does not independently verify these IDs in this slice. It is an append-only REVIEW EVENT, not evidence of successful independent validation, execution authorization or model promotion. No interface promotes or activates weights.

A weight manifest is staged only when W0 SHA256, artifact SHA256 and byte size are supplied. W2 additionally requires exact W1 SHA256. No remote artifact URL is fetched.

## Explicit next steps for the complete intelligence system

1. Tenant-specific Memory Pool integration proof with an isolated disposable namespace.
2. Inference adapter: BAN's Ollama model provider needs credential-safe bearer support for existing authenticated api.zdxai.us. Check exact model selection and provider billing. JEV remains direct-only.
3. Signed/verifiable Axiomatic attestations and independently measured outcomes before allowing any accepted learning to influence training.
4. Real W2 adapter training and activation with W0/W1 hash-bound lineage, validation and rollback.
5. Federation aggregation across compatible adapters, client-data drift controls, non-IID tests and held-out regression evaluation.
6. Weekly W1 consolidation, release manifests, canary rollout and immutable generation registry.
7. Fleet deployment: tenant isolation, rate limiting, central queue, metrics, backups, revocation, TLS, recovery, per-node/tenant spending limits.

The current W2 policy/eligibility code must not be confused with real daily W2 weights. The W0/W1/W2 percentages in the BAN design are intended architectural shares; they do not mathematically guarantee safety.

## Verification

    python3 -m unittest discover -s ops/federation -p 'test_*.py' -v
    python3 -m py_compile ops/federation/control_plane.py ops/federation/node.py

Use temporary databases only for tests. No production configuration, service, database, model checkpoint or scheduler is modified by this integration branch.
