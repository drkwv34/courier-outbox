# service-ops — spec delta for add-swagger-ui-and-load-script

## ADDED Requirements

### Requirement: Swagger UI at docs
The system SHALL expose `GET /docs` and `GET /docs/` without authentication. The response MUST be 200 HTML that renders the served OpenAPI document using embedded assets, not a runtime CDN (FR-API-001 Should).

#### Scenario: Docs is public HTML
- **WHEN** a client sends `GET /docs` with no Authorization header
- **THEN** the response status is 200
- **AND** Content-Type is HTML
- **AND** the body references `/openapi.json` and does not load script or stylesheet hosts outside this process

## MODIFIED Requirements

### Requirement: OpenAPI document is served
The system SHALL expose `GET /openapi.json` without authentication. It SHALL return 200 with `Content-Type: application/json` whose document is generated from the committed `openapi/openapi.yaml`. The document MUST list every live HTTP path, method, Bearer auth on `/v1`, and status code the handlers emit (FR-API-001).

#### Scenario: Public openapi json
- **WHEN** a client sends `GET /openapi.json` with no Authorization header
- **THEN** the response status is 200
- **AND** the body is a JSON object with `openapi` and `paths`
- **AND** `paths` includes `/healthz`, `/readyz`, `/openapi.json`, `/metrics`, `/docs`, `/v1/subscriptions`, `/v1/events`, and `/v1/deliveries`

#### Scenario: OpenAPI matches handler auth
- **WHEN** a reviewer compares `/v1` operations in the served document to the router
- **THEN** every `/v1` operation requires bearer auth
- **AND** `/healthz`, `/readyz`, `/openapi.json`, `/docs`, and `/metrics` do not
