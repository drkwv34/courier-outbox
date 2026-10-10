# Design: add-dlq-replay-logs

## Context

See proposal.md for motivation. Day 5 already dead-letters after six failures, writes append-only `delivery_attempts`, and moved auth probes to `/v1/__probe` so this change can mount `/v1/deliveries`. Schema (`000004`, `000005`) is complete. Constraints: domain stays pure; store owns every TX; queries always filter by `api_key_id` via the events join; secrets and payload bodies never appear in list/detail responses or logs; delivery remains at-least-once.

## Goals / Non-Goals

**Goals:**

- Tenant-scoped list/get/replay over existing tables, no migration.
- Replay is a short UPDATE that returns the row to the existing claim loop.
- 409 for illegal status, 404 for missing/foreign ids (indistinguishable).

**Non-Goals:**

- New worker paths, new backoff, dashboard UI, metrics, or replaying `delivered` rows.

## Decisions

- **No goose migration.** Existing indexes on `(status, next_attempt_at)` and `(subscription_id, created_at)` plus the events `api_key_id` join are enough for operator-sized lists. Alternative: a covering index on `(status, created_at)`. Rejected until a real list is slow; forward-only files must change schema.
- **Offset pagination**, not keyset. `limit` (default 50, max 100) + `offset` + `total` is enough for DLQ ops and matches the spec's "cursor/offset" allowance. Alternative: opaque `(created_at, id)` cursor. Deferred; offset is simpler and the dataset is the caller's deliveries, not a firehose.
- **Default list is unfiltered.** Operators pass `status=dead_lettered` to see only the DLQ. Alternative: default to dead letters. Rejected so the same route can inspect pending/retrying/delivered without a second endpoint.
- **Tenant isolation via `JOIN events`.** Deliveries have no `api_key_id`; ownership is the parent event. Every SELECT/UPDATE includes `events.api_key_id = $caller`. A foreign id returns `ErrNotFound`, never a distinct forbidden.
- **Replay only `dead_lettered`.** Domain `Delivery.Replay()` returns `ErrInvalidTransition` otherwise. Store UPDATE is fenced `WHERE status = 'dead_lettered'`; zero rows then a follow-up SELECT distinguishes not-found vs wrong status. Alternative: also re-fire `delivered`. Rejected by the brief; `delivered` stays terminal in the domain model.
- **`attempt_count` is historical, not a retry budget on replay.** Replay does not decrement it. After replay the worker's next failure uses `RecordFailure` which increments again; a previously exhausted row that fails once more dead-letters immediately (`attempt_count >= 6`). That is intended: operators replay to give the subscriber another chance, not to reset the schedule. Alternative: reset count to 0. Rejected — SRS says history is retained and `attempt_count` never decreases.
- **HTTP mapping:** `ErrInvalidTransition` → 409 `invalid_transition`. `ErrNotFound` stays 404. Replay success is 202 with the delivery body (no attempts required on this response). GET detail includes attempts; list items omit them.
- **GET detail omits payload.** Include `event_id` and `event_type` (from the join) so operators can correlate without echoing the body. Attempt fields match FR-DEL-006 columns only.
- **Interfaces:** api declares `DeliveryStore` with `ListDeliveries`, `GetDelivery`, `ReplayDelivery`. `*store.Postgres` implements them. Worker interfaces are unchanged.
- **Integration test** lives under `internal/api` with the existing tagged Postgres setup plus a worker pool and httptest subscriber (AllowHTTP). Sequence: always-fail mock + short backoff → `dead_lettered` → list/detail → flip mock to 2xx → replay → `delivered` with more attempt rows than before. Auth stays on `/v1/__probe`.

## Risks / Trade-offs

- **[Risk] Replay of a still-leased dead letter.** Dead-lettered rows should already have null leases (RecordAttempt clears them). Replay still SET NULL lease columns. Mitigation: UPDATE always clears leases.
- **[Risk] Offset pagination skips/duplicates if rows are inserted during paging.** → Mitigation: operator lists are small; document newest-first (`created_at DESC, id DESC`).
- **[Trade-off] Immediate re-dead-letter if replay fails once.** Keeping `attempt_count` is honest history; operators who need more automatic retries would need a future "reset budget" change.
- **[Trade-off] List can return pending/delivered as well as DLQ.** One resource collection is simpler than a dedicated `/v1/dlq` path.

## Migration Plan

- Schema already migrated. Deploy the new API binary; workers need no change. Rollback: old binary 404s the new routes; `dead_lettered` rows remain valid.

## Test plan

- **Unit (domain):** `Replay` from `dead_lettered` → `pending` with unchanged `attempt_count` and cleared lease; `Replay` from pending/delivered/retrying → `ErrInvalidTransition`; `ParseDeliveryID`.
- **Unit (api):** list filters/validation; get 200/400/404; replay 202/409/404; unauthorized; fake store errors 500 without leaking internals; response never contains payload or secrets.
- **Integration (store):** list filters + pagination isolation; get with attempts oldest-first; replay UPDATE; foreign key 404 vs wrong-status 409.
- **Integration (api+worker):** force DLQ → list → detail attempts → replay → delivered with additional attempt rows.
