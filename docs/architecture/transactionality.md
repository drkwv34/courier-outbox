# Transactionality

Postgres is the system of record. Correctness comes from **constraints plus short transactions plus fenced updates**, not from locks held across network calls.

## Ownership of transactions

- `internal/store` owns every `BEGIN`/`COMMIT`. Callers invoke use-case-shaped methods (`EnqueueEvent`, `ClaimDue`, `RecordAttempt`, `ReplayDelivery`), and each method is one transaction.
- Internally, use a helper such as `func (s *Postgres) inTx(ctx context.Context, fn func(pgx.Tx) error) error`. It rolls back on error or panic and commits otherwise.
- Never pass a `pgx.Tx` across package boundaries. Never let `api` or `worker` start a transaction.
- **Never hold a transaction open across an outbound HTTP call, a Redis round-trip that can block, or a sleep.**

## Isolation

- The default is `READ COMMITTED`. Anomalies are prevented with `UNIQUE` constraints, `ON CONFLICT`, `FOR UPDATE SKIP LOCKED`, and conditional `UPDATE ... WHERE`. No `SERIALIZABLE` retry loops.
- If a future change truly needs stronger isolation, it must say so in its `design.md` and include a retry-on-`40001` test.

## Enqueue (FR-EVT-001/002)

One transaction:

```sql
INSERT INTO events (id, api_key_id, type, payload, idempotency_key, occurred_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (api_key_id, idempotency_key) DO NOTHING
RETURNING id;
-- 0 rows → SELECT the existing event + its deliveries → replay (200)
-- 1 row  → INSERT one delivery per matching enabled subscription → 201
```

Two concurrent identical requests produce one event. The loser sees the conflict and returns the winner's result. The concurrent integration test is mandatory.

## Claim → send → record (FR-DEL-001, FR-DEL-007)

```sql
-- Claim (own short TX)
UPDATE deliveries d
SET lease_owner = $worker_id, lease_until = now() + $lease, updated_at = now()
WHERE d.id IN (
  SELECT id FROM deliveries
  WHERE status IN ('pending', 'retrying')
    AND next_attempt_at <= now()
    AND (lease_until IS NULL OR lease_until < now())
  ORDER BY next_attempt_at
  FOR UPDATE SKIP LOCKED
  LIMIT $batch
)
RETURNING d.*;
```

The worker commits the claim and only then POSTs. Recording is a second short TX that inserts the attempt and runs `UPDATE deliveries ... WHERE id = $id AND lease_owner = $worker_id`. If that update touches 0 rows, the lease was lost (`domain.ErrLeaseLost`). The attempt row is still recorded for audit.

This yields **at-least-once** semantics. A crash after a successful POST but before the record step means the delivery is sent again after the lease expires. Consumers dedupe on `X-Courier-Idempotency-Key`. Do not "fix" this with longer transactions.

## Clocks

- **The database clock is authoritative** for anything compared in SQL (`next_attempt_at`, `lease_until`). The domain computes durations, and SQL applies `now() + $delay`.
- Go-side `time.Now()` is only for logs, metrics, and attempt duration measurement, and goes through an injected `Clock` in testable code.

## Idempotent writes

- Every mutating endpoint is either naturally idempotent (PATCH to the same values) or keyed (enqueue).
- Replay (`POST /v1/deliveries/:id/replay`) is idempotent. Replaying a delivery already in `pending` is a no-op success. Replaying a `delivered` one is `409 conflict`.
- Attempts are append-only. Never `UPDATE` or `DELETE` them outside the retention purge.

## Redis

- Redis holds **no durable truth**. Flushing Redis may cause duplicate sends or relaxed rate limits but never lost or stuck deliveries.
- Postgres lease columns are authoritative. Redis-side locks and limits are advisory and have TTLs.

## Migrations

These are forward-only and live under `migrations/`. See `migrations/README.md`. The schema carries the invariants: `NOT NULL`, `CHECK (status IN (...))`, `UNIQUE`, foreign keys with deliberate `ON DELETE` behaviour.
