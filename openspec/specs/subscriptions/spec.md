# subscriptions Specification

## Purpose

Let producers register webhook endpoints (target URL, event-type filters, enabled flag, static headers) and hold a per-subscription signing secret. The secret is returned once and stored encrypted at rest. Target URLs pass scheme validation and an SSRF policy.

Planned coverage: FR-SUB-001 (create + secret once), FR-SUB-002 (list/get/update/disable/rotate), FR-SUB-003 (URL validation + SSRF guard), FR-ERR-002 (never re-expose secrets).

## Requirements

### Requirement: Subscriptions capability pending implementation
The system SHALL NOT expose subscription CRUD or signing-secret rotation until an approved OpenSpec change adds normative requirements to this spec and the change is implemented (planned: FR-SUB-001..003, FR-ERR-002).

#### Scenario: Current milestone
- **WHEN** the repository is at the persistence-and-api-keys milestone
- **THEN** no subscription HTTP API is implemented
- **AND** future requirements SHALL be introduced only via `openspec/changes/<id>/` deltas merged into this spec
