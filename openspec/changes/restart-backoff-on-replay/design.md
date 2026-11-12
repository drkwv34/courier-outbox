# Design

## Context

See proposal.md for motivation. `Delivery.Replay()` currently keeps `AttemptCount`. The store UPDATE sets status/lease/`next_attempt_at` but not `attempt_count`, so the domain copy is ignored. `BackoffAfterFailure` dead-letters at `attempt_count >= 6`. Attempt rows are append-only. Domain rule "attempt_count never decreases" is updated by this change: the counter is the current retry cycle, history lives in `delivery_attempts`.

## Goals / Non-Goals

**Goals:**

- Replay sets `attempt_count=0` so the next six failures use the full schedule.
- Attempt history is retained.

**Non-Goals:**

- A separate `retry_cycle` column.
- Replaying `delivered` rows.

## Decisions

- **Reset `attempt_count` to 0 in `Delivery.Replay()` and persist it in `ReplayDelivery` SQL.** Alternative: keep the counter and add `budget_base`. Rejected — extra column for the same operator-visible field. Alternative: document that replay does not restart. Rejected by the brief.
- **GET detail still lists all attempt rows oldest-first.** Operators see history even when `attempt_count` is 0.
- **No migration.** Integer column already exists; 0 is valid (same as enqueue).

## Risks / Trade-offs

- **[Breaking] Clients that stored `attempt_count` after replay as 6 now see 0.** → Mitigation: OpenAPI and README.
- **[Trade-off] `attempt_count` is no longer lifetime HTTP tries.** The attempts array is.

## Migration Plan

- Deploy API + worker. In-flight dead letters replayed after deploy get a full budget. Rollback restores keep-count behaviour.

## Test plan

- **Unit (domain):** Replay from dead_lettered yields pending, attempt_count 0, lease cleared, attempts not in this struct.
- **Unit (api):** 202 body `attempt_count=0`.
- **Integration:** exhaust 6 failures, replay, keep failing, wait until dead_lettered again with `attempt_count=6` and 12 attempt rows (not 7).
