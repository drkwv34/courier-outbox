# Tasks: add-event-enqueue-idempotency

## 1. Domain

- [x] 1.1 Add `ParseEventType`, `ParseIdempotencyKey`, `ParseEventID`, `NewEvent`, payload-object check, and `MatchesEventType` in `internal/domain`. Verify table tests cover type regex/length, key 1–128, non-object payload, RFC 3339 `occurred_at`, and wildcard vs exact matching.

## 2. Store

- [x] 2.1 Add Postgres `inTx` plus `EnqueueEvent` (ON CONFLICT DO NOTHING, matching pending deliveries, bounded retry when the winner is not yet visible) and `GetEvent` filtered by `api_key_id`. Verify tagged integration tests on real Postgres: matching/disabled/non-matching deliveries, sequential replay, 20-goroutine duplicate enqueue → one event, and foreign get is `ErrNotFound`. Confirm no new goose migration is required (UNIQUE and delivery indexes already exist).

## 3. API

- [x] 3.1 Implement `POST /v1/events` and `GET /v1/events/{id}`: 201 vs 200 + `Idempotent-Replay: true`, 400/413 mapping, owner GET includes payload, foreign GET 404. Verify httptest coverage for validation, replay header, oversized body, malformed id, and isolation. Wire `Events` in `cmd/courier` and confirm `go build ./...`.
- [x] 3.2 Update `openapi/openapi.yaml` for both operations and event schemas. Verify the contract lists 201/200/400/401/413 for POST and 200/400/401/404 for GET.
- [x] 3.3 Add tagged API integration tests against real Postgres for enqueue + matching deliveries, sequential replay, parallel duplicate enqueue, cross-key GET 404, and 413. Verify `go test -race -tags=integration ./internal/api/... ./internal/store/...`.

## 4. Docs

- [x] 4.1 Update README status and curl examples for `POST /v1/events` and `GET /v1/events/{id}` only. Verify documented calls match OpenAPI paths. Leave `.env.example` unchanged (no new knobs).

## 5. Verification

- [x] 5.1 `openspec validate add-event-enqueue-idempotency --strict` passes.
- [ ] 5.2 `go test -race ./...`, `go test -race -tags=integration ./...`, and `golangci-lint run` are green.

## Workflow follow-up

- After the PR is merged locally onto `main`, archive with `openspec archive add-event-enqueue-idempotency --yes` (add `--skip-specs` if main specs were already synced in the PR) as a direct commit on `main`.
- Confirm CI on the PR head (a concurrency `cancelled` run on the merge commit is not a failure).
