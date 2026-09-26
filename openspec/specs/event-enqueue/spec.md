# event-enqueue Specification

## Purpose

Accept events from producers and persist, in a single database transaction, the event plus one pending delivery per matching enabled subscription. Idempotency keys scoped to the API key guarantee that concurrent duplicate enqueues produce exactly one event.

Planned coverage: FR-EVT-001 (enqueue, 201/200 replay, 413), FR-EVT-002 (UNIQUE idempotency under concurrency), FR-EVT-003 (get event + delivery summaries), NFR-REL-002, NFR-PERF-001.

## Requirements

_None yet. Requirements are added by the change that implements this capability._
