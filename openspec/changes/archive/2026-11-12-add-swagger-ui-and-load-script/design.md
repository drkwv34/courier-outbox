# Design

## Context

See proposal.md for motivation. `GET /openapi.json` is already served from the committed YAML. `GET /docs` 404s. `scripts/demo.sh` is the only seed script. Constraints: no runtime CDN; domain stays pure; public ops paths stay unauthenticated; no secrets in the load script output after key creation.

## Goals / Non-Goals

**Goals:**

- Embedded Swagger UI at `/docs`.
- `scripts/load.sh` documented and CI-smoked.

**Non-Goals:**

- Try-it-out against live `/v1` from a browser cookie (API keys stay Bearer).
- A Go load binary.

## Decisions

- **Vendor swagger-ui-dist** (`swagger-ui-bundle.js`, `swagger-ui.css`, index.html) under `internal/api/docsui/` with `go:embed`. index.html loads `/openapi.json` from the same origin. Alternative: CDN. Rejected by the brief. Alternative: new Go module for swgui. Rejected — extra dep, assets are enough.
- **Routes:** `GET /docs` and `GET /docs/` return index HTML; `/docs/swagger-ui-bundle.js` and `/docs/swagger-ui.css` are the assets. chi FileServer with prefix strip, plus explicit `/docs` without trailing slash.
- **`scripts/load.sh`:** bash + curl. Creates a key if `KEY` unset (`docker compose run --rm --no-deps api keys create` when `COMPOSE=1`, else `courier keys create`). Creates a subscription to `TARGET_URL` (default mock). Enqueues `COUNT` (default 100) events with unique idempotency keys. Prints count, elapsed, enqueue rps. Never prints the full API key after the create line (match demo.sh redaction where practical).
- **CI:** `bash -n scripts/load.sh` plus `go test` covering `/docs` 200 HTML. No new flaky compose job required for the syntax/HTML smoke; Compose usage stays in README.

## Risks / Trade-offs

- **[Risk] Large JS blob in git.** → Mitigation: only bundle + css; no source maps.
- **[Trade-off] Try-it-out needs a pasted Bearer key.** Acceptable; keys are not in cookies.

## Migration Plan

- New binary serves `/docs`. Old binary 404s it. Load script is additive.

## Test plan

- **Unit:** `GET /docs` 200, text/html, contains openapi.json, no cdn.jsdelivr / unpkg.
- **OpenAPI YAML** lists `/docs` public.
- **CI:** `bash -n scripts/load.sh`.
