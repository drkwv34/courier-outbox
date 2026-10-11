# Tasks: add-openapi-metrics-tests

## 1. Contract

- [x] 1.1 Add `GET /openapi.json` and `GET /metrics` to `openapi/openapi.yaml`, bump `info.version` to 0.5.0, and confirm every live handler path (healthz, readyz, subscriptions, events, deliveries list/detail/replay) still matches. Verify the YAML lists public ops paths without bearer and `/v1` operations with bearerAuth.

## 2. OpenAPI serving

- [x] 2.1 Embed `openapi/openapi.yaml`, convert YAML→JSON at serve startup, and mount public `GET /openapi.json`. Verify httptest: 200, `application/json`, document contains `openapi` plus `/healthz`, `/v1/events`, `/v1/deliveries`, `/openapi.json`, and `/metrics`. Startup MUST fail if the YAML is invalid.

## 3. Metrics

- [x] 3.1 Add a process-local Prometheus registry in `internal/worker` with `courier_deliveries_total{status}`, `courier_attempt_duration_seconds`, and `courier_dlq_total`. Observe after a successful `RecordAttempt`. Verify unit tests: delivered/retrying/dead_lettered labels, histogram observed, nil observer is a no-op, labels never include ids or URLs.
- [x] 3.2 Mount public `GET /metrics` on the API router from the registry handler and wire `cmd/courier serve` (OpenAPI JSON + metrics) plus the worker observer. Verify httptest `/metrics` is 200 Prometheus text containing the three names at zero; `.env.example` and README note that the endpoint is public.

## 4. Hard-edge tests

- [x] 4.1 Expand SSRF URL-policy unit cases (CIDR boundaries, `fd00::/8`, public IPv6 allow, hostname with port) and verify `go test -race ./internal/domain` covers them. Do not add dial-time Control.
- [x] 4.2 Expand concurrent idempotent enqueue and worker crash/lease-reclaim coverage (pending row not lost; survivor delivers after `lease_until`). Verify `go test -race -tags=integration` for those packages. Auth probes stay on `/v1/__probe`.
- [x] 4.3 Add an integration smoke: after one successful httptest delivery, `GET /openapi.json` is 200 and `GET /metrics` contains the three metric names. Verify with `-race -tags=integration`.

## 5. Verification

- [x] 5.1 `openspec validate add-openapi-metrics-tests --strict` passes.
- [x] 5.2 `go test -race ./...`, `go test -race -tags=integration ./...`, and `golangci-lint run` are green.

## Workflow follow-up

- After the PR is merged locally onto `main`, archive with `openspec archive add-openapi-metrics-tests --yes` (add `--skip-specs` if main specs were already synced in the PR) as a direct commit on `main`.
- Confirm CI on the PR head (a concurrency `cancelled` run on the merge commit is not a failure).
