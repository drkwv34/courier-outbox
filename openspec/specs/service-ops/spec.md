# service-ops Specification

## Purpose

Operational surfaces that let operators and orchestrators run courier-outbox: liveness, readiness, the published API contract, and metrics. Readiness, `/openapi.json`, `/docs`, and `/metrics` arrive through later changes (FR-API-001..003).

## Requirements

### Requirement: Liveness probe
The system SHALL expose `GET /healthz` without authentication. It SHALL return `200` with body `{"status":"ok"}` whenever the HTTP server is running, and it SHALL NOT check external dependencies.

#### Scenario: Process is alive
- **WHEN** a client sends `GET /healthz`
- **THEN** the response status is 200
- **AND** the JSON body is `{"status":"ok"}`

### Requirement: Error envelope
The system SHALL encode every non-2xx HTTP response as JSON `{"code": string, "message": string}` (FR-ERR-001).

#### Scenario: Unknown route
- **WHEN** a client requests a path that is not routed
- **THEN** the response status is 404
- **AND** the JSON body has `code` = `not_found`
