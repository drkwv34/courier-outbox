# Change: Persistence foundation, API keys, and readiness

- **Id:** `add-persistence-and-api-keys`
- **Status:** approved
- **SRS requirements:** FR-AUTH-001, FR-AUTH-002, FR-AUTH-003 (foundation only), FR-API-002, NFR-SEC-001, NFR-MAINT-002

## Why

Every later capability (subscriptions, enqueue, delivery, DLQ) needs a Postgres schema, a migration path, and an authenticated `/v1` surface keyed by `api_key_id`. Today the service only answers `/healthz`: it cannot connect to Postgres or Redis, has no schema, and cannot tell one producer from another. This change lands the persistence foundation and the producer-auth bootstrap so the feature changes that follow only have to add behaviour.

## What changes

- Forward-only SQL migrations (goose, embedded in the binary) for `api_keys`, plus staged tables `subscriptions`, `events`, `deliveries`, `delivery_attempts` with the constraints and indexes from SRS §6. Staged tables have no application code yet.
- `courier migrate` applies pending migrations. Compose runs it as a one-shot `migrate` service before `api` starts.
- `courier keys create --name <name>` generates a key `co_<prefix>_<secret>`, stores only `prefix` and a peppered HMAC-SHA256 hash, and prints the raw key exactly once.
- Bearer authentication middleware guards everything under `/v1`. A missing, malformed, unknown, revoked, or wrong key returns `401` with `{"code":"unauthorized"}`. `/v1` has no routes yet, so an authenticated request gets `404`.
- `GET /readyz` checks Postgres and Redis and returns `200` or `503`.
- Configuration now requires and validates `DATABASE_URL`, `REDIS_URL`, and `COURIER_API_KEY_PEPPER` at startup.

## Impact

- **Capabilities:** `api-keys` (new requirements), `service-ops` (readiness).
- **Endpoints / CLI:** `GET /readyz`; `/v1/*` behind Bearer auth; `courier migrate`; `courier keys create`.
- **Schema / migrations:** `000001`–`000005` (api_keys, subscriptions, events, deliveries, delivery_attempts).
- **Config:** `DATABASE_URL`, `REDIS_URL` (now required), `COURIER_API_KEY_PEPPER` (new, ≥ 32 bytes).
- **Packages:** `internal/config`, `internal/domain`, `internal/store`, `internal/api`, `cmd/courier`, `migrations`.
- **Dependencies:** pgx v5, goose v3, go-redis v9, testcontainers-go (tests only).

## Out of scope

- Admin HTTP endpoints for key management (v1 is CLI-only, per FR-AUTH-001) and key revocation commands.
- Any subscription, enqueue, worker, signing, retry, or DLQ behaviour. The staged tables carry no logic.
- SSRF policy beyond the existing config flags.
- `/openapi.json`, `/docs`, `/metrics`.
