# Change: Subscription CRUD and signing secrets

- **Id:** `add-subscriptions-signing-secrets`
- **Status:** archived
- **SRS requirements:** FR-SUB-001, FR-SUB-002, FR-SUB-003, FR-ERR-002, NFR-SEC-002

## Why

Producers cannot register webhook endpoints yet: `/v1` authenticates but has no routed handlers, and the staged `subscriptions` table has no application code. Event enqueue and delivery both need a tenant-owned, URL-validated subscription with a signing secret that is shown once and encrypted at rest. This change lands that surface so later days can enqueue and deliver without inventing CRUD, isolation, or SSRF policy.

## What Changes

- Authenticated subscription HTTP API under `/v1`: create, list, get one, update (including `rotate_secret=true`), and disable.
- Create and rotate return a 32-byte signing secret **once**. GET/list/update-without-rotate never include it (FR-SUB-001, FR-SUB-002, FR-ERR-002).
- The secret is stored as AES-256-GCM ciphertext (`nonce || ciphertext`) keyed by `COURIER_ENCRYPTION_KEY` (32 bytes, base64). The process refuses to start if the key is missing or the wrong length.
- Target URL validation: absolute `https`, or `http` only when `ALLOW_HTTP_CALLBACKS=true`; no userinfo; empty host rejected. When `SSRF_PROTECTION=true` (secure default), IP literals in loopback, link-local, private, CGNAT, unspecified, multicast, and broadcast ranges (including IPv4-mapped IPv6) are rejected (FR-SUB-003, NFR-SEC-002). Dial-time SSRF remains a delivery-worker concern.
- Every subscription query filters by the caller's `api_key_id`. A foreign or missing id returns `404 not_found`, never a leaky forbidden (FR-AUTH-003 at query level).
- Errors follow the existing typed-error / HTTP-mapping table. Logs never contain signing secrets, encryption keys, or payload bodies.
- Config: `COURIER_ENCRYPTION_KEY` (required), `SSRF_PROTECTION` (default true), `ALLOW_HTTP_CALLBACKS` (default false). Compose keeps the existing relaxed SSRF/HTTP values for the mock subscriber and supplies a local-only encryption-key placeholder.
- OpenAPI, `.env.example`, and README are updated only for the subscription surface that exists after this change.

**BREAKING** for local operators: `COURIER_ENCRYPTION_KEY` becomes required at process start (serve and `keys create` both load config).

## Capabilities

### New Capabilities

- None. Subscriptions already exist as a stub capability.

### Modified Capabilities

- `subscriptions`: replace the "pending implementation" stub with normative CRUD, secret-once, encryption-at-rest, URL/SSRF policy, and per-key isolation requirements.

## Impact

- **Capabilities:** `subscriptions` (stub → implemented).
- **Endpoints:** `POST /v1/subscriptions`, `GET /v1/subscriptions`, `GET /v1/subscriptions/{id}`, `PATCH /v1/subscriptions/{id}`, `POST /v1/subscriptions/{id}/disable`. All behind existing Bearer auth.
- **Schema / migrations:** no new tables. The existing `subscriptions` table (`000002`) is sufficient (`signing_secret_enc bytea`, `api_key_id`, JSONB `event_types`/`headers`).
- **Config:** `COURIER_ENCRYPTION_KEY` required (≥ 32 decoded bytes); `SSRF_PROTECTION` and `ALLOW_HTTP_CALLBACKS` parsed with secure defaults.
- **Packages:** `internal/config`, `internal/domain` (subscription model, URL policy, envelope encryption), `internal/store` (CRUD filtered by `api_key_id`), `internal/api` (handlers, JSON decode, error mapping), `cmd/courier` (wire store + policy + encryptor), `openapi/openapi.yaml`, `.env.example`, README.
- **Dependencies:** none new (stdlib `crypto/aes` + `crypto/cipher` + `net/netip`).

## Out of scope

- Delivery attempts, worker loop, outbound HTTP POST to subscribers, dial-time SSRF.
- HMAC signing of event payloads (`internal/sign` remains a stub).
- `POST /v1/events`, enqueue, idempotency (Day 4).
- DLQ, replay, attempt-log API.
- Full OpenAPI surface beyond subscriptions; `/openapi.json`, `/docs`, `/metrics`.
- Admin HTTP for API keys; key revocation commands.
- Decrypting signing secrets anywhere except tests that prove ciphertext is not plaintext. The worker will decrypt later.
