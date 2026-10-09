# Tasks: add-delivery-worker-signing-retries

## 1. Domain, signing, config

- [x] 1.1 Add domain backoff table, `RecordSuccess`/`RecordFailure`, lease fields on `Delivery`, `Attempt` value type, and `ErrLeaseLost`/`ErrInvalidTransition`. Verify table tests cover delays 1s/5s/25s/2m/10m, dead-letter at attempt 6, and illegal transitions off `delivered`.
- [x] 1.2 Implement `internal/sign` v1 HMAC-SHA256 (`Sign`/`Verify`, 5-minute skew). Verify golden vectors with a fixed secret/timestamp/body, skew rejection, and a fuzz round-trip.
- [x] 1.3 Parse worker knobs and test-only `COURIER_BACKOFF_MS` in `internal/config`. Verify defaults (lease 30s, concurrency 4, claim limit 8, poll 500ms) and fail-fast on invalid values.

## 2. Store

- [x] 2.1 Confirm no new goose migration is required (lease columns and `delivery_attempts` already exist). Add `ClaimDue` (`FOR UPDATE SKIP LOCKED`, skip disabled subscriptions, set lease, return payload/URL/secret_enc) and `RecordAttempt` (insert attempt then fenced update; 0 rows → `ErrLeaseLost`). Verify tagged Postgres tests: claim due vs future vs leased, skip disabled, success/failure/dead-letter status, attempt rows, lease reclaim after expiry, and two claimers never sharing one unexpired lease.

## 3. Worker

- [x] 3.1 Add the isolated outbound client (timeouts, no redirects, 8 KiB cap, User-Agent) and the claim → decrypt → sign → POST → record loop. Never hold a TX across HTTP. Verify httptest: 2xx delivers; 3xx and >5s mocks fail; headers include the v1 signature that the mock verifies.
- [x] 3.2 Add tagged worker integration tests against real Postgres: mock 500 then 200 → `delivered` with two attempt rows; slow mock → failure attempt; claim without complete then reclaim past `lease_until`; two concurrent workers emit one POST in the lease window. Verify logs include `delivery_id`/`event_id`/`attempt` and omit secrets and payload bodies.

## 4. CLI, compose, docs, probes

- [x] 4.1 Add `courier worker`, wire store/envelope/config, and add a Compose `worker` service on the same image. Verify `go build ./...` and `docker compose config -q`.
- [x] 4.2 Update README (worker, signature scheme, at-least-once consumer contract) and `.env.example` (new knobs; `COURIER_BACKOFF_MS` marked test-only). Move auth probes from `/v1/deliveries` to `/v1/__probe`. Verify documented commands match the CLI and probe tests 401/404 on `/v1/__probe`.

## 5. Verification

- [x] 5.1 `openspec validate add-delivery-worker-signing-retries --strict` passes.
- [x] 5.2 `go test -race ./...`, `go test -race -tags=integration ./...`, and `golangci-lint run` are green.

## Workflow follow-up

- After the PR is merged locally onto `main`, archive with `openspec archive add-delivery-worker-signing-retries --yes` (add `--skip-specs` if main specs were already synced in the PR) as a direct commit on `main`.
- Confirm CI on the PR head (a concurrency `cancelled` run on the merge commit is not a failure).
