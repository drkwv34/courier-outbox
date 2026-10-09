# Change: Delivery worker with HMAC signing and retries

- **Id:** `add-delivery-worker-signing-retries`
- **Status:** approved
- **SRS requirements:** FR-DEL-001, FR-DEL-002, FR-DEL-003, FR-DEL-004, FR-DEL-005, FR-DEL-006, FR-DEL-007, FR-DOC-001, NFR-REL-001, NFR-SEC-002 (signature goldens), NFR-OBS-001

## Why

Events sit in `pending` deliveries with no worker to claim or send them. Without SKIP LOCKED leases, HMAC signatures, the 1s→5s→25s→2m→10m schedule, and attempt logging, the outbox cannot deliver (FR-DEL-001..007) and crash recovery is unproven (NFR-REL-001). Day 5 ships the claim/send path so Compose and tests can prove at-least-once signed delivery.

## What Changes

- `courier worker` subcommand (same binary as serve) runs a fixed-size pool that claims due deliveries (`status IN (pending, retrying)`, `next_attempt_at <= now()`, lease null or expired) via `FOR UPDATE SKIP LOCKED LIMIT N` in a short TX that sets `lease_owner` and `lease_until` (default 30s) and commits before any HTTP. Compose starts a `worker` service on the same image.
- Signing (FR-DEL-002): `X-Courier-Idempotency-Key: <event_id>`, `X-Courier-Timestamp: <unix_seconds>`, `X-Courier-Signature: v1=<hex hmac_sha256>` over `{timestamp}.{raw_body_bytes}` using the subscription signing secret decrypted with the existing envelope. Golden unit tests; httptest mock verifies the signature and rejects skew > 5 minutes.
- Isolated HTTP client (FR-DEL-003): dial 2s, TLS handshake 2s, total 5s; read ≤ 8 KiB then drain; `User-Agent: courier-outbox/1.0`; do not follow redirects. A mock slower than 5s is a failed attempt.
- Outcomes (FR-DEL-004/005): 2xx → `delivered` + `delivered_at=now`. Else `attempt_count++`, `retrying`, `next_attempt_at = now + schedule[attempt_count]` (1→1s, 2→5s, 3→25s, 4→2m, 5→10m); 6+ → `dead_lettered` (status only; no DLQ API). Pure backoff function with a table-driven unit test. `COURIER_BACKOFF_MS` is a **test-only** override documented as such.
- Every attempt inserts a `delivery_attempts` row (FR-DEL-006). The lease is released on completion. A claimed-but-unfinished delivery is reclaimable after `lease_until` (FR-DEL-007).
- Integration tests (real Postgres, `-race`, httptest): 500 then 200 → delivered with two attempt rows; signature verified by the mock; slow mock → failure attempt; lease reclaim after claim-without-complete; two concurrent workers never process the same claim twice in one lease window. Logs include `delivery_id`/`event_id`/`attempt` and never secrets or payload bodies (NFR-OBS-001).
- README worker + signature scheme; Consumer responsibilities stay accurate and state at-least-once (not exactly-once) (FR-DOC-001). `.env.example` documents new knobs (placeholders only). Auth/unauthorized probes use `/v1/__probe`, never a future resource path.
- Schema: `deliveries.lease_owner`, `lease_until`, `delivered_at` and `delivery_attempts` already exist from Day 2. Add a forward-only goose migration only if a required column/constraint is missing (none expected).

## Capabilities

### New Capabilities

- None. Delivery already exists as a stub capability.

### Modified Capabilities

- `delivery`: replace the "pending implementation" stub with normative claim/lease, HMAC signing, outbound client, success/retry/dead-letter outcomes, attempt logging, lease expiry reclaim, and at-least-once crash semantics.

## Impact

- **Capabilities:** `delivery` (stub → implemented).
- **Endpoints / CLI:** `courier worker`. No new producer HTTP routes. Auth tests move their unrouted probe from `/v1/deliveries` to `/v1/__probe`.
- **Schema / migrations:** no new tables expected. Day 2 already created `deliveries` lease columns and `delivery_attempts` (`delivery_id`, nullable `status_code`/`error_message`, `duration_ms`, `request_id`, `created_at`).
- **Config:** `WORKER_ID`, `WORKER_CONCURRENCY`, `WORKER_CLAIM_LIMIT`, `WORKER_LEASE_TTL`, `WORKER_POLL_INTERVAL`, test-only `COURIER_BACKOFF_MS`. Document in `.env.example`.
- **Packages:** `internal/domain` (backoff, attempt outcome, lease sentinels), `internal/sign` (v1 HMAC sign/verify), `internal/store` (`ClaimDue`, `RecordAttempt`, load event+subscription+secret for a claim), `internal/worker` (pool, client, loop), `internal/config`, `cmd/courier`, Compose, README, OpenAPI only if an HTTP surface changes (none planned besides the probe path, which is not a contract route).
- **Dependencies:** none new (stdlib crypto/hmac, existing pgx, optional `golang.org/x/sync/errgroup` which is already indirect).

## Out of scope

- DLQ list/replay endpoints and delivery-log API (Day 6).
- Metrics endpoint, `/openapi.json` serving, OpenAPI hardening (Day 7).
- Dial-time SSRF IP blocking beyond not following redirects (FR-SUB-003 dial-time remains later).
- Multi-region; Redis as the durable lease store (Postgres lease columns remain authoritative).
- Exactly-once claims anywhere. Courier remains at-least-once.
- NFR-PERF-002 load script (≥ 20 deliveries/s) — correctness only tonight.
- Changes to kickoff-club, beacon-watch, semaphore-flags, or the profile repo.
