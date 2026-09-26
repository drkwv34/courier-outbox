# service-ops: spec delta for add-persistence-and-api-keys

## ADDED Requirements

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
