# service-ops Specification

## Purpose

Operational surfaces that let operators and orchestrators run courier-outbox: liveness, readiness, the published API contract, and metrics. `GET /openapi.json` and public `GET /metrics` are implemented (FR-API-001, FR-API-003). Swagger UI at `/docs` is still optional.

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

### Requirement: Readiness probe
The system SHALL expose `GET /readyz` without authentication. It SHALL ping Postgres and Redis, each with a bounded timeout. When every dependency answers, it SHALL return `200` with `{"status":"ready"}`. Otherwise it SHALL return `503` with `code` = `unavailable` and a message naming the failing dependencies, and it SHALL NOT include driver error details (FR-API-002).

#### Scenario: Dependencies healthy
- **WHEN** Postgres and Redis both answer a ping
- **THEN** `GET /readyz` returns 200 with `{"status":"ready"}`

#### Scenario: Redis down
- **WHEN** Redis does not answer within the timeout
- **THEN** `GET /readyz` returns 503 with `code` = `unavailable`
- **AND** the message names `redis`

### Requirement: Schema migrations
The system SHALL ship forward-only SQL migrations embedded in the binary and apply them with `courier migrate`. Re-running the command on an up-to-date database SHALL be a no-op (NFR-MAINT-002).

#### Scenario: Fresh database
- **WHEN** `courier migrate` runs against an empty database
- **THEN** the tables `api_keys`, `subscriptions`, `events`, `deliveries`, and `delivery_attempts` exist

#### Scenario: Idempotent re-run
- **WHEN** `courier migrate` runs a second time
- **THEN** it applies nothing and exits 0

### Requirement: Startup configuration validation
The process SHALL fail fast at startup when `DATABASE_URL` is not a `postgres://` or `postgresql://` URL, when `REDIS_URL` is not a `redis://` or `rediss://` URL, or when `COURIER_API_KEY_PEPPER` is shorter than 32 bytes. It SHALL report all problems together.

#### Scenario: Missing database URL
- **WHEN** `DATABASE_URL` is unset
- **THEN** the process exits non-zero with an error naming `DATABASE_URL`

### Requirement: OpenAPI document is served
The system SHALL expose `GET /openapi.json` without authentication. It SHALL return 200 with `Content-Type: application/json` whose document is generated from the committed `openapi/openapi.yaml`. The document MUST list every live HTTP path, method, Bearer auth on `/v1`, and status code the handlers emit (FR-API-001).

#### Scenario: Public openapi json
- **WHEN** a client sends `GET /openapi.json` with no Authorization header
- **THEN** the response status is 200
- **AND** the body is a JSON object with `openapi` and `paths`
- **AND** `paths` includes `/healthz`, `/readyz`, `/openapi.json`, `/metrics`, `/v1/subscriptions`, `/v1/events`, and `/v1/deliveries`

#### Scenario: OpenAPI matches handler auth
- **WHEN** a reviewer compares `/v1` operations in the served document to the router
- **THEN** every `/v1` operation requires bearer auth
- **AND** `/healthz`, `/readyz`, `/openapi.json`, and `/metrics` do not

### Requirement: Prometheus metrics
The system SHALL expose `GET /metrics` without authentication as Prometheus text. It MUST register `courier_deliveries_total` (counter, `status` label), `courier_attempt_duration_seconds` (histogram), and `courier_dlq_total` (counter). After a worker records a successful delivery, those names MUST appear in the exposition (FR-API-003, NFR-OBS-002).

#### Scenario: Metrics endpoint is public
- **WHEN** a client sends `GET /metrics` with no Authorization header
- **THEN** the response status is 200
- **AND** the body contains `courier_deliveries_total`, `courier_attempt_duration_seconds`, and `courier_dlq_total`

#### Scenario: Names present after a delivery
- **WHEN** a worker records a 2xx attempt as `delivered`
- **THEN** `GET /metrics` on that process includes `courier_deliveries_total`
- **AND** it includes `courier_attempt_duration_seconds`
- **AND** it includes `courier_dlq_total`

#### Scenario: Status label is bounded
- **WHEN** deliveries are recorded as delivered, retrying, or dead_lettered
- **THEN** `courier_deliveries_total` uses only those `status` values
- **AND** metric labels do not include ids or URLs
