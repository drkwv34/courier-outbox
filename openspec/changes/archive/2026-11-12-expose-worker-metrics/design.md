# Design

## Context

See proposal.md for motivation. `internal/worker.Prometheus` already registers `courier_deliveries_total`, `courier_attempt_duration_seconds`, and `courier_dlq_total` and observes after `RecordAttempt`. `cmd/courier serve` mounts that registry on the API; `cmd/courier worker` never binds HTTP. Compose publishes only `api:8080`. Constraints: worker must not import `internal/api`; domain stays pure; no secrets in metrics labels; delivery remains at-least-once.

## Goals / Non-Goals

**Goals:**

- Worker process serves Prometheus text at `GET /metrics`.
- Live counters are scrapeable from Compose after one delivery.
- API no longer exposes the same names stuck at zero.

**Non-Goals:**

- Pushgateway, federation, or aggregating worker series into the API.
- Token auth on `/metrics`.
- Changing metric names or label sets.

## Decisions

- **`WORKER_METRICS_ADDR` defaults to `:9091`.** Validated with `net.SplitHostPort` like `HTTP_ADDR`. Empty is invalid so the demo always has a scrape. Alternative: empty disables HTTP. Rejected — Compose would stay unobservable unless every operator sets the var. Alternative: share `HTTP_ADDR`. Rejected — worker and API are separate processes and must not collide when colocated.
- **Tiny HTTP server in `cmd/courier` worker, not `internal/api`.** Mux with `/metrics` only, `promhttp` handler from the same `*worker.Prometheus` the pool already uses. Same timeouts as the API server. Shutdown on SIGINT/SIGTERM with the pool. Alternative: import chi in the worker. Rejected — stdlib mux is enough for one path. Alternative: worker imports `internal/api`. Forbidden by layering.
- **Remove delivery series from the API process.** `cmd/courier serve` stops passing `worker.NewPrometheus().Handler()`. API `GET /metrics` remains in OpenAPI only if we keep a process-local handler without courier delivery names; this change unmounts those series by not wiring the worker registry. Prefer unmounting `GET /metrics` on the API if nothing else is registered, and document the worker scrape as the operator target. Keep the OpenAPI `/metrics` path documenting that live delivery counters are on the worker listen address so the contract does not claim the API scrape is the source of truth. Alternative: leave names at zero with a comment. Rejected — that is the misleading scrape.
- **Compose publishes `${WORKER_METRICS_PORT:-9091}:9091`.** Worker env includes `WORKER_METRICS_ADDR=:9091`.
- **Tests first.** Integration: after one httptest delivery, scrape the worker registry/HTTP and assert `courier_deliveries_total{status="delivered"}` > 0. API scrape must not contain those names at zero. Unit: config parses the new addr; worker metrics handler still serves names.

## Risks / Trade-offs

- **[Risk] Two scrapes confuse operators.** → Mitigation: README and OpenAPI say live delivery counters are on the worker; API does not register them.
- **[Risk] Worker HTTP bind fails and the pool never starts.** → Mitigation: fail fast at startup like other config errors; log listen addr at info.
- **[Trade-off] API `/metrics` no longer shows courier series.** Operators who already pointed Prometheus at `:8080/metrics` must retarget the worker port. That is required to stop the zero-series lie.

## Migration Plan

- Deploy worker binary that binds `WORKER_METRICS_ADDR`. Point Prometheus at that scrape. Rollback: old worker has no HTTP; old API again shows names at zero.

## Test plan

- **Unit (config):** default `:9091`; invalid addr fails with `WORKER_METRICS_ADDR`.
- **Unit (worker):** existing observer tests stay; handler still public.
- **Integration:** after one successful delivery, worker `/metrics` counters > 0; API `/metrics` does not present `courier_deliveries_total` at 0.
- **Compose/docs:** `.env.example`, README, `docker-compose.yml` publish the port.
