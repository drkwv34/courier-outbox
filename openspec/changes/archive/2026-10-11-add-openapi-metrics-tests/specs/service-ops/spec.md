# service-ops — spec delta for add-openapi-metrics-tests

## ADDED Requirements

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
