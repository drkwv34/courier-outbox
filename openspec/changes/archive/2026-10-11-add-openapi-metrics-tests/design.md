# Design: add-openapi-metrics-tests

## Context

See proposal.md for motivation. Handlers for subscriptions, events, deliveries, healthz, and readyz already exist; `openapi/openapi.yaml` is committed but not served. No Prometheus registry is wired. Constraints: domain stays pure; api is the only package that knows HTTP status codes; worker must not import api; no transaction across outbound HTTP; secrets and payload bodies are never logged; delivery remains at-least-once. Dial-time SSRF stays out of this change (subscriptions spec).

## Goals / Non-Goals

**Goals:**

- Single-source OpenAPI YAML, JSON served at boot.
- Process-local Prometheus registry; `/metrics` public on the API.
- Worker observes attempt outcomes through an injected interface.
- Hard-edge tests: concurrent enqueue, crash/lease reclaim, URL-policy boundaries, openapi/metrics smoke.

**Non-Goals:**

- `/docs` Swagger UI.
- Worker scrape port / `METRICS_ADDR`.
- Dial-time `Control` hook.
- New goose migration or env vars.

## Decisions

- **YAML is the source of truth.** Embed `openapi/openapi.yaml` via a tiny `openapi` package (`go:embed`) next to the YAML (cmd cannot embed `../`). Convert once at process start with `gopkg.in/yaml.v3` unmarshal → `json.Marshal`. Invalid YAML fails serve startup. Alternative: commit a sibling `.json`. Rejected — drift risk. Alternative: convert per request. Rejected — the document is static.
- **No `/docs`.** Optional Should; skipping avoids a UI screenshot in the Definition of done. Alternative: embed swagger-ui. Deferred to a later change.
- **`GET /metrics` is public** on the API chi router (`promhttp.HandlerFor` a dedicated registry, not the global default). Document in README / OpenAPI / `.env.example`. Alternative: bearer or a scrape token. Rejected tonight — observability.md allows public or simple token; public matches `/healthz`.
- **Custom registry in `internal/worker`.** `worker.Prometheus` implements `worker.Observer` (`ObserveAttempt(status, duration)`). After `RecordAttempt` succeeds, increment `courier_deliveries_total{status}`, observe `courier_attempt_duration_seconds`, and if status is `dead_lettered` increment `courier_dlq_total`. Pre-create `status` children `delivered|retrying|dead_lettered` so HELP/TYPE and series exist at zero. Histogram buckets cover the 5s attempt budget. Alternative: metrics in `internal/api`. Rejected — worker would import api, violating layering. Alternative: prometheus default registry. Rejected — parallel tests collide.
- **API Deps take `OpenAPIJSON []byte` and `Metrics http.Handler`.** Nil/empty means the route is not mounted (unit tests that do not care). `cmd/courier serve` always supplies both. The API process registers the same metric names even though it does not run the worker, so Compose `GET localhost:8080/metrics` contains the names. Live counters increment in the worker process; integration tests share one `*worker.Prometheus` between router and pool.
- **No new config.** Worker does not bind HTTP. Operators who want live delivery series from a split worker process can follow up with `METRICS_ADDR`.
- **Tests.** Expand, do not rewrite, existing tagged coverage: assert `TestEvents_ParallelDuplicate` / store parallel still enforce one event row; add worker crash-mid-lease (claim, abandon, expire, survivor delivers); add URL-policy CIDR boundary and public IPv6 cases; add `GET /openapi.json` 200 + `/metrics` after a delivered attempt. Auth stays on `/v1/__probe`.
- **OpenAPI YAML** version bump to 0.5.0; add `/openapi.json` and `/metrics` (prometheus text, no JSON schema required beyond a string content type). Existing `/v1` operations stay; audit that they still match handlers.

## Risks / Trade-offs

- **[Risk] YAML 1.1 boolean/float conversion corrupts OpenAPI keys.** → Mitigation: our document uses quoted enums and `3.0.3` as a string; add a boot/unit assertion that `openapi` and `/v1/events` survive round-trip.
- **[Risk] API `/metrics` stays at zero in Compose because the worker is another process.** → Mitigation: pre-register series so names are present (brief requirement). Integration test shares the registry and asserts observations after a 2xx. Follow-up: worker scrape address.
- **[Trade-off] Public `/metrics`.** Series have bounded labels only (no ids/URLs). Token auth can land later without changing names.
- **[Trade-off] No dial-time SSRF tonight.** Create-time IP-literal policy remains; Compose still sets `SSRF_PROTECTION=false` for the mock. Documented as a known gap in README.

## Migration Plan

- No schema change. Deploy API binary that serves the two ops paths; workers gain observer wiring. Rollback: old binary 404s `/openapi.json` and `/metrics`; deliveries are unaffected.

## Test plan

- **Unit (api):** `/openapi.json` 200, JSON has expected paths, unauthenticated; `/metrics` 200 with the three names at zero; `/v1/__probe` still 401/404 as today.
- **Unit (domain):** URL-policy table adds 172.16/12 and CGNAT boundaries, `fd00::/8`, public IPv6 allow, hostname-with-port.
- **Unit (worker):** Observer records delivered / retrying / dead_lettered; histogram observes duration; nil observer is a no-op.
- **Integration:** concurrent enqueue (20 goroutines, one event); lease reclaim after abandoned claim; OpenAPI + metrics after one successful httptest delivery (shared registry, `AllowHTTP`).
