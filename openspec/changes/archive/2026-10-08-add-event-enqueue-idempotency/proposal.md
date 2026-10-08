# Change: Transactional event enqueue and idempotency keys

- **Id:** `add-event-enqueue-idempotency`
- **Status:** archived
- **SRS requirements:** FR-EVT-001, FR-EVT-002, FR-EVT-003, NFR-REL-002

## Why

Producers can authenticate and register subscriptions, but they still cannot persist events. Without a transactional enqueue that creates pending deliveries for matching subscriptions, the outbox is unused and Day 5 has nothing to claim. Idempotency keys scoped to the API key are required now so duplicate or concurrent POSTs never create two events (FR-EVT-002, NFR-REL-002).

## What Changes

- Authenticated `POST /v1/events` under the existing Bearer `/v1` surface. Body: required `type`, JSON-object `payload`, `idempotency_key` (1–128 chars); optional `occurred_at` (RFC 3339, default now).
- One database transaction inserts the event and one `pending` delivery (`attempt_count=0`, `next_attempt_at=now()`) per **enabled** subscription of the same API key whose `event_types` match the event type (`["*"]` matches all).
- New event: `201` with `{ event_id, delivery_ids }`. Replay of the same `(api_key_id, idempotency_key)`: `200`, header `Idempotent-Replay: true`, same ids. Concurrent duplicates resolve to exactly one event row (UNIQUE constraint + TX/retry); never 500 and never two events.
- Validation: `400` `validation_failed` for missing/invalid type, non-object payload, bad idempotency key, bad `occurred_at`. Body over 256 KiB: `413` `payload_too_large` before decode. Unauthenticated: existing `401`.
- `GET /v1/events/{id}` returns event metadata (including payload for the owning key) plus delivery summaries (`id`, `subscription_id`, `status`, `attempt_count`, `next_attempt_at`). Another key's event is `404 not_found` (no existence leak).
- Structured logs with `event_id` and `api_key_id`; never secrets or full payloads.
- OpenAPI and README updated only for the event surface that exists after this change. No new config knobs.

## Capabilities

### New Capabilities

- None. Event enqueue already exists as a stub capability.

### Modified Capabilities

- `event-enqueue`: replace the "pending implementation" stub with normative enqueue, idempotent replay under concurrency, payload limit, matching pending deliveries, GET-by-id, and per-key isolation.

## Impact

- **Capabilities:** `event-enqueue` (stub → implemented).
- **Endpoints:** `POST /v1/events`, `GET /v1/events/{id}`. Both behind existing Bearer auth.
- **Schema / migrations:** no new tables. Day 2 already has `events` with `UNIQUE (api_key_id, idempotency_key)` and `deliveries` with `(status, next_attempt_at)` and `(subscription_id, created_at)`. Add a forward-only goose migration only if a required constraint or index is missing (it is not).
- **Config:** none. `.env.example` unchanged unless a knob is added (none planned).
- **Packages:** `internal/domain` (event/delivery constructors, type matching, idempotency key), `internal/store` (`inTx`, `EnqueueEvent`, `GetEvent`), `internal/api` (handlers, replay header, 413), `cmd/courier` (wire Events store), `openapi/openapi.yaml`, README.
- **Dependencies:** none new.

## Out of scope

- HTTP POST to subscribers, worker loop, claim/lease logic, HMAC signing of payloads, retries/backoff (Day 5).
- DLQ, replay, and delivery-log endpoints (Day 6).
- Metrics endpoint or full OpenAPI surface beyond events (Day 7).
- Exactly-once delivery claims. Courier remains at-least-once.
- New config variables, Redis writes, or changes to subscription CRUD.
