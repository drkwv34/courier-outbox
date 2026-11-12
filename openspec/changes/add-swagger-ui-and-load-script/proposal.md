# Proposal

- **Id:** `add-swagger-ui-and-load-script`
- **Status:** approved
- **SRS requirements:** FR-API-001, NFR-PERF-002; SRS Should embed Swagger UI at `/docs`

## Why

`GET /docs` returns 404, so operators have no in-process OpenAPI UI. `scripts/` contains only `demo.sh`, so NFR-PERF-002's load-script verify is unmet even though enqueue throughput is already fine.

## What Changes

- Serve embedded Swagger UI at public `GET /docs` (and `/docs/`) from vendored assets with no runtime CDN, pointing at the existing `/openapi.json`.
- Add `scripts/load.sh` that enqueues N events against a running API and prints enqueue rate; document usage in the README.
- CI smokes both: `/docs` returns 200 HTML, and the load script is syntax-checked (and runnable with a small N when the stack is up).

## Capabilities

### New Capabilities

- None.

### Modified Capabilities

- `service-ops`: public Swagger UI at `/docs`; OpenAPI document lists that path as unauthenticated.
- `local-demo`: documented load script in `scripts/`.

## Impact

- **Capabilities:** `service-ops`, `local-demo`.
- **Endpoints / CLI:** `GET /docs`. No CLI subcommand.
- **Schema / migrations:** none.
- **Config:** none.
- **Packages:** `internal/api` (embed + route), `openapi/openapi.yaml`, `scripts/load.sh`, README, GitHub Actions smoke, vendored Swagger UI assets.
- **Dependencies:** vendored swagger-ui-dist static files (Apache-2.0), no new Go module.

## Out of scope

- Worker metrics, hostname SSRF, replay backoff.
- Hosting Swagger UI from an external CDN at runtime.
- Replacing `demo.sh`.
