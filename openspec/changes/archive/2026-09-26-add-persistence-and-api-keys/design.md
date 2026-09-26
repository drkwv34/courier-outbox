# Design: add-persistence-and-api-keys

## Context

This is the first change that talks to Postgres and Redis, so it has to settle the open questions the scaffold deferred (SRS §11, ADR index): the migration tool, the SQL access style, the API-key hash, and the integration-test harness.

## Decisions

- **Migrations: goose v3, SQL files embedded with `embed.FS`** (ADR 0003). The binary carries its own schema, so `courier migrate` works in the distroless image with no extra files. Files are forward-only: they have a `-- +goose Up` section and no Down section.
- **SQL access: hand-written pgx v5** for now. The api-keys queries are two statements, which doesn't justify a sqlc codegen step yet. The enqueue/claim changes can revisit this.
- **API-key hash: HMAC-SHA256 keyed by `COURIER_API_KEY_PEPPER`** (ADR 0004). Keys carry 256 bits of randomness, so a slow KDF adds latency on every request without adding meaningful resistance. The pepper means a leaked database alone is not enough to test candidate keys.
- **Key format:** `co_<prefix>_<secret>`. The prefix is 8 lowercase base32 chars (40 bits) and the secret is 52 lowercase base32 chars (256 bits). The whole raw key is hashed. The prefix is stored in clear with a `UNIQUE` index and used as the lookup handle; it is display-safe.
- **Integration tests: testcontainers-go** with the postgres and redis modules, behind the `integration` build tag (ADR 0005).
- **`/v1` is mounted behind the auth middleware as a whole**, so unmatched `/v1/*` paths also require a key and nothing under `/v1` is reachable anonymously by accident.
- **Readiness** pings each dependency with a 2s timeout. On failure it returns `503` with the standard error envelope and names the failing dependency. Driver errors go to logs only.

## Data model / SQL sketch

SRS §6, with these additions:

- `api_keys.prefix` is `UNIQUE`, and `key_hash` is `bytea` with a 32-byte length `CHECK`.
- `subscriptions.headers` is JSONB for the optional static headers from FR-SUB-001.
- `deliveries.status` has a `CHECK` on the four states. The indexes are `(status, next_attempt_at)` and `(subscription_id, created_at)`, plus `UNIQUE (event_id, subscription_id)` so one event cannot fan out twice to the same subscription.
- `delivery_attempts.request_id` comes from FR-DEL-006.
- Tenant-owned tables reference `api_keys(id)`. Foreign keys use `ON DELETE RESTRICT` because keys are revoked, not deleted.

## Failure modes

- **Postgres down at request time:** the auth lookup fails, so the request gets `500 internal` and the error is logged. `/readyz` reports `503`, so orchestrators stop routing traffic to the instance.
- **Prefix collision on create** (odds of 2^-40 per pair): the `UNIQUE` violation is translated to `domain.ErrConflict` and the CLI reports it. Re-running the command generates a fresh key.
- **Revoked key:** returns `401`, indistinguishable from an unknown key.
- **Migrate run twice:** a no-op. goose tracks applied versions in `goose_db_version`.

## Test plan

- **Unit:** key generation format and entropy source injection, parse accept/reject table, hash determinism and pepper sensitivity, and constant-time verify. Also config validation (missing/invalid URLs, short pepper), the auth middleware matrix (no header, wrong scheme, malformed, unknown, wrong secret, revoked, store error, valid), and readiness with fake checks.
- **Integration (tagged):** migrate on a fresh Postgres 16 container creates all tables, and a second migrate is a no-op. Store round-trip for create/lookup, `ErrNotFound`, and `ErrConflict` on a duplicate prefix. A real-store auth test covers `401` on a bad key and a pass-through on a good key. Redis ping.
