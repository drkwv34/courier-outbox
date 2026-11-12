# service-ops — spec delta for expose-worker-metrics

## MODIFIED Requirements

### Requirement: Prometheus metrics
The worker process SHALL expose `GET /metrics` without authentication as Prometheus text on a configurable listen address. It MUST register `courier_deliveries_total` (counter, `status` label), `courier_attempt_duration_seconds` (histogram), and `courier_dlq_total` (counter). After the worker records one successful delivery, those counters MUST be nonzero on that scrape. The API process MUST NOT register those delivery series. Docker Compose MUST publish the worker scrape address (FR-API-003, NFR-OBS-002).

#### Scenario: Metrics endpoint is public
- **WHEN** a client sends `GET /metrics` to the worker listen address with no Authorization header
- **THEN** the response status is 200
- **AND** the body contains `courier_deliveries_total`, `courier_attempt_duration_seconds`, and `courier_dlq_total`

#### Scenario: Names present after a delivery
- **WHEN** a worker records a 2xx attempt as `delivered`
- **THEN** `GET /metrics` on the worker process includes `courier_deliveries_total`
- **AND** it includes `courier_attempt_duration_seconds`
- **AND** it includes `courier_dlq_total`
- **AND** `courier_deliveries_total{status="delivered"}` is greater than 0

#### Scenario: Status label is bounded
- **WHEN** deliveries are recorded as delivered, retrying, or dead_lettered
- **THEN** `courier_deliveries_total` uses only those `status` values
- **AND** metric labels do not include ids or URLs

#### Scenario: API does not present zero delivery series
- **WHEN** a client sends `GET /metrics` to the API process after a successful worker delivery
- **THEN** the body does not include `courier_deliveries_total` at zero
- **AND** operators scrape the worker listen address for live delivery counters

#### Scenario: Compose publishes the worker scrape
- **WHEN** an operator runs the documented Docker Compose stack
- **THEN** the worker `GET /metrics` port is published to the host
- **AND** README documents that live delivery counters are on the worker scrape, not the API
