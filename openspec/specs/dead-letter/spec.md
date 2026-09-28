# dead-letter Specification

## Purpose

Make exhausted deliveries queryable and manually replayable without losing history. Replay resets the delivery to pending, and attempt records stay append-only.

Planned coverage: FR-DLQ-001 (list with filters + pagination), FR-DLQ-002 (replay → 202, history retained), FR-ERR-003 (410/404 still retried; optional auto-disable after 50 consecutive 410s).

## Requirements

### Requirement: Dead-letter capability pending implementation
The system SHALL NOT expose dead-letter list or replay behavior until an approved OpenSpec change adds normative requirements to this spec and the change is implemented (planned: FR-DLQ-001, FR-DLQ-002, FR-ERR-003).

#### Scenario: Current milestone
- **WHEN** the repository is at the persistence-and-api-keys milestone
- **THEN** no DLQ HTTP API or replay worker behavior is implemented
- **AND** future requirements SHALL be introduced only via `openspec/changes/<id>/` deltas merged into this spec
