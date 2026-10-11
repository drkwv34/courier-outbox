# Change: OpenAPI serving, Prometheus metrics, and hard-edge tests

- **Id:** `add-openapi-metrics-tests`
- **Status:** approved
- **SRS requirements:** FR-API-001, FR-API-003, NFR-OBS-002, NFR-MAINT-001; harden NFR-REL-001, NFR-REL-002, NFR-SEC-002 where coverage is thin

## Why

The HTTP handlers and committed `openapi/openapi.yaml` exist, but operators cannot fetch the contract at `/openapi.json` and Prometheus never exposes delivery outcomes. Integration coverage for concurrent idempotent enqueue, lease reclaim after a crashed worker, and SSRF URL-policy edges is thinner than SRS §7 requires (FR-API-001, FR-API-003, NFR-OBS-002, NFR-MAINT-001).

## What Changes

- Serve `GET /openapi.json` (public, 200) generated at boot from the committed YAML so the live contract stays a single source of truth. Paths, methods, Bearer auth, and status codes stay in sync with every live handler (keys are CLI-only; HTTP: subscriptions, events, deliveries list/detail/replay, healthz/readyz, plus the new ops paths). No Swagger UI at `/docs` in this change.
- Serve `GET /metrics` as Prometheus text, public (documented). Register and expose at least `courier_deliveries_total{status=}`, `courier_attempt_duration_seconds`, and `courier_dlq_total`. The worker observes these after a successful attempt record. After one successful delivery in the integration suite, `/metrics` contains those names.
- Expand race-tagged integration tests: concurrent idempotent enqueue (UNIQUE `api_key_id+idempotency_key`), worker crash / lease reclaim so pending deliveries are not lost, SSRF URL-policy unit gaps (CIDR boundaries, public IPv6), and OpenAPI/metrics smoke. Auth probes stay on unrouted `/v1/__probe`.
- golangci-lint stays clean on touched packages. No CI weakening. No goose migration.

## Capabilities

### New Capabilities

- None. Service operations already exist as a stub for OpenAPI/metrics.

### Modified Capabilities

- `service-ops`: add served OpenAPI JSON (FR-API-001) and Prometheus metrics (FR-API-003, NFR-OBS-002). Healthz/readyz stay as they are.

## Impact

- **Capabilities:** `service-ops` (OpenAPI + metrics).
- **Endpoints / CLI:** `GET /openapi.json`, `GET /metrics`. No CLI changes. Auth tests remain on `/v1/__probe`.
- **Schema / migrations:** none.
- **Config:** none required. Metrics are public on the API process. Worker observations use an injected observer (no new env var unless a follow-up exposes worker scrape).
- **Packages:** `openapi/` (embed YAML), `internal/api` (handlers), `internal/worker` (observer + Prometheus registry), `cmd/courier` (wire JSON + metrics), `internal/domain` (URL-policy unit gaps only), OpenAPI YAML, README / `.env.example` notes.
- **Dependencies:** `github.com/prometheus/client_golang` (pre-approved), `gopkg.in/yaml.v3` (YAML→JSON at boot).

## Out of scope

- Swagger UI at `/docs` (optional Should; skipped so no UI screenshot is required).
- Pin-quality README rewrite or demo seed script (Day 8).
- Dial-time SSRF `net.Dialer.Control` (subscriptions spec still marks hostname resolution at dial time out of scope; tonight fills URL-policy unit gaps only).
- gRPC; multi-region; exactly-once claims; About/topics API.
- Changes to kickoff-club, beacon-watch, semaphore-flags, or the profile repo.
- Metrics persistence, token-protected `/metrics`, HTTP request/claim-batch series.
