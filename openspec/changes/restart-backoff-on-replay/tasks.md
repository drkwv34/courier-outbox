# Tasks

## 1. Failing regression

- [ ] 1.1 Change domain and handler tests to expect `attempt_count=0` after replay, and add an integration case that a still-failing target is not dead-lettered on the first post-replay attempt.

## 2. Domain and store

- [ ] 2.1 Make `Delivery.Replay()` set `AttemptCount` to 0 and persist `attempt_count = 0` in `ReplayDelivery`, verifying store tests keep attempt rows.

## 3. Contract and docs

- [ ] 3.1 Update OpenAPI replay text, README, and domain-model docs so they state the retry budget restarts and history stays append-only.

## 4. Verification

- [ ] 4.1 `openspec validate restart-backoff-on-replay --strict` passes.
- [ ] 4.2 `go test -race ./internal/domain ./internal/api ./internal/store` and tagged integration for deliveries are green.

## Workflow follow-up

- After the PR is merged locally onto `main`, archive with `openspec archive restart-backoff-on-replay --yes` (add `--skip-specs` if main specs were already synced in the PR) as a direct commit on `main`.
