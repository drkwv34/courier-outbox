# Capabilities

Each folder is one capability. Its `spec.md` is the **current** truth: requirements appear there only after a change that implements them is merged and archived.

| Capability | Folder | SRS requirements | Status |
|------------|--------|------------------|--------|
| Service operations (health, readiness, OpenAPI, metrics) | `service-ops/` | FR-API-001..003, NFR-OBS-* | `/healthz` + `/readyz`; OpenAPI/metrics later |
| API keys & authentication | `api-keys/` | FR-AUTH-001..003, NFR-SEC-001 | CLI create + Bearer auth; query isolation later |
| Subscriptions & signing secrets | `subscriptions/` | FR-SUB-001..003, FR-ERR-002 | CRUD + secret-once + SSRF URL policy |
| Event enqueue & idempotency | `event-enqueue/` | FR-EVT-001..003, NFR-REL-002, NFR-PERF-001 | POST/GET events + idempotent replay |
| Delivery (worker, signing, retries, leases) | `delivery/` | FR-DEL-001..007, NFR-REL-001, NFR-PERF-002 | worker + HMAC + retries; load later |
| Dead-letter queue & replay | `dead-letter/` | FR-DLQ-001..002, FR-ERR-003 | stub |

Consumer documentation (FR-DOC-001) is a README deliverable. It is tracked through the PR template checklist, not a capability.

## Suggested change order

1. ~~`add-persistence-and-api-keys`~~ archived 2026-09-26.
2. ~~`add-subscriptions-signing-secrets`~~ archived 2026-10-07.
3. ~~`add-event-enqueue-idempotency`~~ archived 2026-10-08.
4. ~~`add-delivery-worker-signing-retries`~~ archived 2026-10-09.
5. `add-dead-letter-replay`: DLQ listing, replay, attempt log API.
6. `add-openapi-metrics`: full OpenAPI, `/openapi.json`, `/docs`, `/metrics`.
