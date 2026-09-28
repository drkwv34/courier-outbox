# event-enqueue Specification

## Purpose

Accept events from producers and persist, in a single database transaction, the event plus one pending delivery per matching enabled subscription. Idempotency keys scoped to the API key guarantee that concurrent duplicate enqueues produce exactly one event.

Planned coverage: FR-EVT-001 (enqueue, 201/200 replay, 413), FR-EVT-002 (UNIQUE idempotency under concurrency), FR-EVT-003 (get event + delivery summaries), NFR-REL-002, NFR-PERF-001.

## Requirements

### Requirement: Event enqueue capability pending implementation
The system SHALL NOT accept producer event enqueue until an approved OpenSpec change adds normative requirements to this spec and the change is implemented (planned: FR-EVT-001..003, NFR-REL-002, NFR-PERF-001).

#### Scenario: Current milestone
- **WHEN** the repository is at the persistence-and-api-keys milestone
- **THEN** no `POST /v1/events` (or equivalent) enqueue API is implemented
- **AND** future requirements SHALL be introduced only via `openspec/changes/<id>/` deltas merged into this spec
