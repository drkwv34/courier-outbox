# Tasks

## 1. Failing regression

- [x] 1.1 Add an integration assertion that after one successful httptest delivery, worker `/metrics` has `courier_deliveries_total{status="delivered"}` greater than 0, and verify the test fails while the worker still has no HTTP scrape and the API still registers zeros.
- [x] 1.2 Update OpenAPI `/metrics` text so it no longer claims the API process registers delivery series at zero, and verify `openapi/openapi.yaml` describes the worker scrape as the live source.

## 2. Worker scrape

- [x] 2.1 Add `WORKER_METRICS_ADDR` to `internal/config` (default `:9091`, `SplitHostPort` validation) and verify config unit tests for default and invalid addr.
- [x] 2.2 Serve `GET /metrics` from `courier worker` using the same `*worker.Prometheus` the pool observes, and verify a unit/httptest scrape of that handler after `ObserveAttempt(delivered)` is nonzero.
- [x] 2.3 Stop wiring the worker registry onto `cmd/courier serve`, and verify API `/metrics` tests no longer expect `courier_deliveries_total` at zero.

## 3. Compose and docs

- [x] 3.1 Publish the worker metrics port in `docker-compose.yml`, document `WORKER_METRICS_ADDR` in `.env.example` and README, and verify `docker compose config -q` succeeds.

## 4. Verification

- [x] 4.1 `openspec validate expose-worker-metrics --strict` passes.
- [x] 4.2 `go test -race ./...` and `go test -race -tags=integration ./internal/api ./internal/worker ./internal/config` are green for this change.

## Workflow follow-up

- After the PR is merged locally onto `main`, archive with `openspec archive expose-worker-metrics --yes` (add `--skip-specs` if main specs were already synced in the PR) as a direct commit on `main`.
