# delivery Specification

## Purpose

Deliver events to subscribers at least once. Workers claim due deliveries with `FOR UPDATE SKIP LOCKED` under time-bounded leases, sign payloads (v1 HMAC-SHA256), and POST through an isolated HTTP client with strict timeouts and SSRF protection. They record every attempt and schedule retries (1s → 5s → 25s → 2m → 10m) before dead-lettering.

Planned coverage: FR-DEL-001..007, NFR-REL-001 (no lost deliveries on crash), NFR-PERF-002 (≥ 20 deliveries/s locally), NFR-SEC-002 (signature golden tests).

## Requirements

### Requirement: Delivery capability pending implementation
The system SHALL NOT deliver events to subscribers until an approved OpenSpec change adds normative requirements to this spec and the change is implemented (planned: FR-DEL-001..007, NFR-REL-001, NFR-PERF-002, NFR-SEC-002).

#### Scenario: Current milestone
- **WHEN** the repository is at the persistence-and-api-keys milestone
- **THEN** no delivery worker or outbound webhook POST behavior is implemented
- **AND** future requirements SHALL be introduced only via `openspec/changes/<id>/` deltas merged into this spec
