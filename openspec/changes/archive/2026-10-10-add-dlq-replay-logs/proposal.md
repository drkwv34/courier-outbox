# Change: Dead-letter list, replay, and delivery logs

- **Id:** `add-dlq-replay-logs`
- **Status:** approved
- **SRS requirements:** FR-DLQ-001, FR-DLQ-002, FR-DEL-006 (visibility), FR-DOC-001, FR-ERR-001

## Why

Exhausted deliveries already become `dead_lettered` after six failed attempts, but operators cannot list them, inspect attempt history, or send them back to the worker. Without a queryable DLQ and append-only replay, failed webhooks stay invisible and unrecoverable (FR-DLQ-001, FR-DLQ-002, FR-DEL-006 visibility).

## What Changes

- `GET /v1/deliveries` lists deliveries owned by the authenticated API key, filterable by `status` (including `dead_lettered`), `subscription_id`, event `type`, and `created_after`/`created_before`, with offset pagination (FR-DLQ-001). Operators can list only dead letters via `status=dead_lettered`. Auth is the existing Bearer API key. Errors use `{code, message}` (FR-ERR-001).
- `GET /v1/deliveries/{id}` returns the delivery and its append-only `delivery_attempts` (status_code, error_message, duration_ms, request_id, created_at) oldest→newest. Missing or foreign ids are 404. Signing secrets and raw event payloads are never included.
- `POST /v1/deliveries/{id}/replay` returns 202 and is allowed only for `dead_lettered` rows. Replay sets status `pending`, `next_attempt_at=now()`, and clears lease fields. It does **not** reset `attempt_count` or delete attempt rows. Replaying a non-dead-letter returns 409 with code `invalid_transition`. The worker then claims the row as usual.
- Worker dead-lettering after schedule exhaustion is unchanged. Integration tests force a delivery into DLQ, list it, inspect attempts, replay, and observe a later `delivered` status with new attempt rows after the old failures. Auth probes stay on unrouted `/v1/__probe`.
- README **Consumer responsibilities** documents verify HMAC + reject skew > 5 minutes, at-least-once dedupe on `X-Courier-Idempotency-Key`, 2xx only after durable accept, and fast responses (FR-DOC-001). Operators get a brief DLQ + replay note. The PR template already checkboxes this section.
- `openapi/openapi.yaml` is updated for the three routes in the same PR. No new goose migration is expected (Day 5 schema is complete).

## Capabilities

### New Capabilities

- None. Dead-letter already exists as a stub capability.

### Modified Capabilities

- `dead-letter`: replace the "pending implementation" stub with normative list, detail, and replay requirements (FR-DLQ-001, FR-DLQ-002). FR-ERR-003 auto-disable-after-410 is not part of this change.
- `delivery`: extend consumer-contract documentation so README covers operator DLQ/replay alongside the existing HMAC/at-least-once contract (FR-DOC-001). Attempt logging itself is already specified; this change exposes those rows over HTTP.

## Impact

- **Capabilities:** `dead-letter` (stub → implemented), `delivery` (docs requirement).
- **Endpoints / CLI:** `GET /v1/deliveries`, `GET /v1/deliveries/{id}`, `POST /v1/deliveries/{id}/replay`. No CLI changes. Auth tests remain on `/v1/__probe`.
- **Schema / migrations:** none expected. `deliveries` and `delivery_attempts` already exist.
- **Config:** none.
- **Packages:** `internal/domain` (Replay transition, list filter), `internal/store` (list/get/replay queries scoped by `api_key_id`), `internal/api` (handlers + 409 mapping for `ErrInvalidTransition`), OpenAPI, README.
- **Dependencies:** none new.

## Out of scope

- Dashboard SPA.
- OpenAPI serving (`/openapi.json`, `/docs`) and Prometheus metrics (Day 7).
- FR-ERR-003 optional auto-disable after 50 consecutive 410s.
- Purge/retention jobs (document Should only if already mentioned; do not implement).
- Replaying `delivered` deliveries.
- Multi-region; exactly-once claims.
- Changes to kickoff-club, beacon-watch, semaphore-flags, or the profile repo.
