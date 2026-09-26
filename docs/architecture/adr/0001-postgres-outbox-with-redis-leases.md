# 0001. Postgres outbox with Redis as accelerator

- **Status:** Accepted
- **Date:** 2026-09-26
- **Source:** SRS §2.7

## Context

Courier must never lose an accepted event, must support a queryable dead-letter queue with replay, and must survive worker crashes. Candidates:

1. Redis only (lists or streams, delay ZSET)
2. Postgres only (outbox table + `FOR UPDATE SKIP LOCKED`)
3. Postgres as system of record plus Redis for coordination

## Decision

Option 3. **Postgres is the system of record** for events, deliveries, attempts, and the DLQ. Workers poll Postgres for due rows with `FOR UPDATE SKIP LOCKED` and hold leases in row columns (`lease_owner`, `lease_until`). **Redis** holds short-lived, TTL'd coordination state: per-subscription outbound rate limits and optional advisory lease fencing.

## Consequences

- Enqueue is transactional: the event and its deliveries commit together, which is easy to demonstrate and test.
- The DLQ is just a status. Listing and replay are plain SQL, and history is retained.
- Losing Redis degrades fairness or throughput only. No data is lost and nothing gets stuck.
- Polling adds latency (bounded by the poll interval) and database load. This is acceptable at the SRS scale target (tens of events/s). `LISTEN/NOTIFY` can reduce latency later without changing the model.
- Delivery semantics are at-least-once. This is documented as a consumer contract, not hidden.
