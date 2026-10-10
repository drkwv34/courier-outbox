# Tasks: add-dlq-replay-logs

## 1. Domain

- [ ] 1.1 Add `ParseDeliveryID`, `Delivery.Replay()` (`dead_lettered` → `pending`, clear lease, keep `attempt_count`), and list-filter value types. Verify table tests: legal replay, illegal replay from pending/retrying/delivered returns `ErrInvalidTransition`, malformed ids wrap `ErrValidation`.

## 2. Store

- [ ] 2.1 Confirm no new goose migration is required. Add `ListDeliveries` (status, subscription_id, event type, created_after/before, limit/offset, tenant via events join), `GetDelivery` (attempts oldest-first), and `ReplayDelivery` (fenced UPDATE; missing → `ErrNotFound`; wrong status → `ErrInvalidTransition`). Verify tagged Postgres tests for filters, pagination, isolation, attempt order, and replay.

## 3. API

- [ ] 3.1 Mount `GET /v1/deliveries`, `GET /v1/deliveries/{id}`, and `POST /v1/deliveries/{id}/replay`. Map `ErrInvalidTransition` to 409 `invalid_transition`. Verify httptest: list filters/validation, detail 200/400/404, replay 202/409/404, unauthorized, no payload or secrets in bodies. Auth probes stay on `/v1/__probe`.
- [ ] 3.2 Update `openapi/openapi.yaml` for the three routes and the 409 code. Verify the documented paths, query params, and error envelope match the handlers.

## 4. Docs and end-to-end

- [ ] 4.1 Expand README Consumer responsibilities (HMAC, 5-minute skew, at-least-once dedupe, 2xx after durable accept, fast response) and note DLQ list + replay for operators. Update the status blurb. Verify the PR template still checkboxes FR-DOC-001.
- [ ] 4.2 Add a tagged integration test: force DLQ with a failing mock and short backoff, list via `GET /v1/deliveries?status=dead_lettered`, GET detail shows prior attempts, replay, worker delivers on a success mock, status `delivered` with new attempt rows after the old failures. Verify `go test -race -tags=integration` covers this path.

## 5. Verification

- [ ] 5.1 `openspec validate add-dlq-replay-logs --strict` passes.
- [ ] 5.2 `go test -race ./...`, `go test -race -tags=integration ./...`, and `golangci-lint run` are green.

## Workflow follow-up

- After the PR is merged locally onto `main`, archive with `openspec archive add-dlq-replay-logs --yes` (add `--skip-specs` if main specs were already synced in the PR) as a direct commit on `main`.
- Confirm CI on the PR head (a concurrency `cancelled` run on the merge commit is not a failure).
