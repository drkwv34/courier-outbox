# Design: add-event-enqueue-idempotency

## Context

See proposal.md for motivation. Day 2 already created `events` (`UNIQUE (api_key_id, idempotency_key)`, payload jsonb object, idempotency_key 1–128) and `deliveries` (pending default, indexes on `(status, next_attempt_at)` and `(subscription_id, created_at)`). Subscription matching is `["*"]` or named types (`NormalizeEventTypes`). `/v1` already authenticates; `GET /v1/events` currently 404s as an unrouted path used by auth tests. Constraints: domain stays pure; store owns the TX; api maps errors and sets `Idempotent-Replay`; never hold a TX across HTTP.

## Goals / Non-Goals

**Goals:**

- One use-case-shaped store method inserts event + matching pending deliveries atomically.
- Concurrent duplicate keys resolve to one event via UNIQUE + retry, returning replay to losers.
- Thin handlers: decode (256 KiB) → domain validate → store → 201 or 200+header.
- GET scoped by `api_key_id`; payload only for the owner.

**Non-Goals:**

- Worker, HMAC, retries, DLQ (see proposal out of scope).
- New migration unless a required constraint is actually missing.
- Changing subscription CRUD or auth middleware.

## Decisions

- **No new migration.** `000003` and `000004` already encode the uniqueness and delivery indexes from SRS §6. Alternative: add a no-op migration for the change id. Rejected — forward-only migrations must change schema.
- **`inTx` helper on Postgres.** First multi-row TX in this repo. Rolls back on error/panic; never exposes `pgx.Tx`. Alternative: ad-hoc Begin/Commit in EnqueueEvent. Rejected so later claim/record can reuse the helper.
- **Insert path:** `INSERT INTO events (...) VALUES (...) ON CONFLICT (api_key_id, idempotency_key) DO NOTHING RETURNING id`. One row → insert deliveries for matching enabled subscriptions, commit, `Replay=false`. Zero rows → `SELECT` event + deliveries; if found, commit nothing new, `Replay=true`. If not found (winner uncommitted under READ COMMITTED), rollback and retry the whole TX with a small bounded loop. Alternative: treat unique violation as 409. Rejected by FR-EVT-002. Alternative: SERIALIZABLE. Forbidden without a design that this change does not need.
- **Matching in the TX:** `SELECT` enabled subscriptions for `api_key_id`, filter with domain `MatchesEventType` (`*` alone or exact type), `INSERT` deliveries with `status='pending'`, `attempt_count=0`, `next_attempt_at=now()`. Alternative: jsonb `?` in SQL only. Rejected so matching is unit-tested in domain. Empty match set is valid (`delivery_ids: []`).
- **Domain constructors:** `ParseEventType`, `ParseIdempotencyKey`, `ParseEventID`, `NewEvent` wrapping `ErrValidation`. Payload MUST be a JSON object (first non-space `{`, `json.Valid`, `jsonb_typeof` already CHECKs in DB). Event type reuses the subscription token regex and 128-char max (stricter than the 200-char DB CHECK). `occurred_at` parsed as RFC 3339 (`time.RFC3339` or `time.RFC3339Nano`); nil pointer means DB `now()`.
- **GET payload:** owner receives `payload` in the JSON body. Foreign/missing ids share `404 not_found` with no payload field, so existence and content do not leak.
- **IDs:** path `{id}` is a UUID (same regex as subscriptions). Invalid UUID → 400, not 404.
- **JSON decode:** reuse `decodeJSON` (MaxBytesReader 256 KiB, DisallowUnknownFields). Map `http.MaxBytesError` → 413. DTOs: `type`, `payload` as `json.RawMessage`, `idempotency_key`, `occurred_at` as `*string`.
- **Replay header:** `Idempotent-Replay: true` only on 200. 201 MUST omit it. Body shape is identical: `{event_id, delivery_ids}` (`delivery_ids` always an array, never null).
- **Wiring:** `api.Deps.Events` consumer-declared interface (`EnqueueEvent`, `GetEvent`). `cmd/courier` passes `*store.Postgres`. Logs: `event_id`, `api_key_id`; never payload (optional `payload_bytes`).
- **Auth tests:** keep using `GET /v1/events` (no id) as the unrouted 404 probe. Routed surface is `POST /v1/events` and `GET /v1/events/{id}` only — no list.

## Risks / Trade-offs

- **[Risk] READ COMMITTED + ON CONFLICT: loser SELECT misses uncommitted winner.** → Mitigation: bounded TX retry until the row is visible or ctx cancels; integration test with 20 goroutines.
- **[Risk] Retry livelock under extreme contention.** → Mitigation: cap retries (e.g. 8) then return a wrapped error → 500 only if the row never appears; the unique constraint still prevents doubles.
- **[Trade-off] Matching in Go after selecting enabled rows.** Fine at per-key subscription counts; SQL jsonb matching can be added later without changing the API.
- **[Trade-off] GET returns payload to the owner.** Required to inspect what was stored; isolation still hides it from other keys.

## Migration Plan

- Schema already migrated; `courier migrate` is a no-op on up-to-date databases.
- Deploy is a binary + OpenAPI bump. Rollback: revert the binary; leftover events/deliveries are valid outbox rows for a future worker.
- Do not drop the unique constraint.

## Test plan

- **Unit (domain):** type regex/length; idempotency key 1–128; payload object vs array/null/string; RFC 3339 `occurred_at`; `MatchesEventType` for `*`, exact, miss, disabled is a store concern.
- **Unit (api, httptest + fakes):** 201 body; sequential 200 + `Idempotent-Replay: true`; validation table; 413; GET owner vs 404; malformed id 400; 401 unchanged; replay-header formatting.
- **Integration (tagged, real Postgres):** enqueue creates event + pending deliveries only for enabled matching subs; sequential replay; **20 parallel duplicate POSTs → one event row and one delivery set**; cross-key GET 404 and distinct events for shared idempotency keys; 413 oversized.
