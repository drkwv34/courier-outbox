# Tasks: add-subscriptions-signing-secrets

## 1. Config

- [x] 1.1 Parse `COURIER_ENCRYPTION_KEY` (standard or raw base64 → 32 bytes), `SSRF_PROTECTION` (default true), and `ALLOW_HTTP_CALLBACKS` (default false) in `internal/config`; fail startup with `ErrInvalid` on missing/short key or invalid bools. Verify with unit tests that errors name the variable and never echo key material.
- [x] 1.2 Document the three variables in `.env.example` (placeholders only) and supply a local-only encryption-key default in `docker-compose.yml`. Verify `docker compose config -q` succeeds.

## 2. Domain

- [x] 2.1 Add `Subscription` fields, `NewSubscription` / event-type / header validation, and `GenerateSigningSecret` (32 bytes, injected `io.Reader`). Verify table tests cover empty types, `"*"` mixed with others, reserved headers, and entropy failure.
- [x] 2.2 Add AES-256-GCM `Envelope` (`nonce || ciphertext`) that copies the key and never logs plaintext. Verify Seal/Open round-trip, wrong key, and truncated blob tests.
- [x] 2.3 Add `URLPolicy` with the SSRF deny-list from design.md (IPv4, IPv6, mapped forms, scheme/userinfo). Verify the allow/deny matrix unit tests named in the spec scenarios.

## 3. Store

- [x] 3.1 Implement `CreateSubscription`, `ListSubscriptions`, `GetSubscription`, `UpdateSubscription`, and `DisableSubscription` on Postgres, each filtering by `api_key_id`, mapping no-rows to `ErrNotFound`. Verify tagged integration tests on real Postgres: create/list/get/update/disable, ciphertext ≠ plaintext, and foreign `api_key_id` is `ErrNotFound`.

## 4. API

- [x] 4.1 Add JSON decode helper (256 KiB cap, `DisallowUnknownFields`) and `writeDomainError` mapping validation/not-found/conflict/413/500. Verify handler tests for unknown fields, oversized body, and generic 500 messages.
- [x] 4.2 Implement `POST/GET /v1/subscriptions`, `GET/PATCH /v1/subscriptions/{id}`, `POST /v1/subscriptions/{id}/disable`; return `signing_secret` only on create and rotate. Verify httptest coverage for secret-once, rotate, disable idempotence, malformed UUID, and cross-key 404.
- [x] 4.3 Update `openapi/openapi.yaml` for the five operations and subscription schemas, and retarget existing auth-passthrough tests from `/v1/subscriptions` to `/v1/events`. Verify OpenAPI describes 401/400/404 and that `GET /v1/subscriptions` with a valid key is no longer expected to 404.
- [x] 4.4 Add tagged API integration tests against real Postgres: create → list → get → update → rotate → disable, secret not re-readable, isolation 404. Verify `go test -race -tags=integration ./internal/api/...`.

## 5. Composition and docs

- [x] 5.1 Wire `Envelope`, `URLPolicy`, and the subscription store into `cmd/courier` `serve`. Verify the binary still builds (`go build ./...`) and refuses to start without `COURIER_ENCRYPTION_KEY`.
- [x] 5.2 Update README status, local curl examples, and env list for subscription CRUD only. Verify the documented create/list/get/disable calls match the OpenAPI paths.

## 6. Verification

- [x] 6.1 `openspec validate add-subscriptions-signing-secrets --strict` passes.
- [x] 6.2 `go test -race ./...`, `go test -race -tags=integration ./...`, and `golangci-lint run` are green.

## Workflow follow-up

- Archive with `openspec archive add-subscriptions-signing-secrets --yes` (add `--skip-specs` if main specs were already synced in the PR) as a direct commit on `main` after merge.
- Confirm CI on the merge commit (a concurrency `cancelled` run is not a failure).
