# Proposal

- **Id:** `expose-worker-metrics`
- **Status:** approved
- **SRS requirements:** FR-API-003, NFR-OBS-002

## Why

`courier worker` records delivery counters in its own process, but it does not listen for HTTP, and Compose publishes only the API's `GET /metrics`. After real deliveries that scrape stays at zero, so operators following the README cannot observe Prometheus counters (FR-API-003, NFR-OBS-002).

## What Changes

- `courier worker` serves public Prometheus text at `GET /metrics` on a configurable listen address (`WORKER_METRICS_ADDR`).
- Docker Compose publishes that scrape port so a local demo can scrape live counters after one delivery.
- After a worker records a successful delivery, worker `/metrics` shows nonzero `courier_deliveries_total`, `courier_attempt_duration_seconds`, and `courier_dlq_total` as applicable.
- The API process MUST NOT register the same delivery series at zero. API `GET /metrics`, if still mounted, must not present misleading all-zero courier delivery counters.
- README and `.env.example` document the worker scrape address and that live delivery counters live on the worker, not the API.

## Capabilities

### New Capabilities

- None.

### Modified Capabilities

- `service-ops`: Prometheus exposition is scrapeable on the worker process; live delivery counters MUST increment there after a recorded attempt. The API must not advertise those series at zero.

## Impact

- **Capabilities:** `service-ops`.
- **Endpoints / CLI:** `courier worker` binds HTTP `GET /metrics`. Compose publishes `WORKER_METRICS_ADDR`. API `GET /metrics` no longer duplicates worker delivery series at zero.
- **Schema / migrations:** none.
- **Config:** `WORKER_METRICS_ADDR` (default `:9091`), documented in `.env.example` and README. Compose publishes the port.
- **Packages:** `internal/config`, `cmd/courier` (worker HTTP), `internal/worker` (existing registry), `internal/api` (stop wiring worker series onto the API), docker-compose, OpenAPI description, README.
- **Dependencies:** none new (`prometheus/client_golang` already in use).

## Out of scope

- Token-protected metrics, aggregating worker series into the API process, or new metric names.
- Swagger UI, load script, hostname SSRF, and replay backoff (separate changes).
- Claiming exactly-once delivery.
