# Tasks: add-readme-demo

## 1. README case study

- [ ] 1.1 Rewrite README: drop `TODO(readme)`, set pin-ready demo status, keep problem/why/stack/architecture, document Compose + `scripts/demo.sh`, Consumer responsibilities (HMAC, 5-minute skew, at-least-once dedupe on `X-Courier-Idempotency-Key`, 2xx after durable accept, 5s budget), Trade-offs that never claim exactly-once, CI badge on `ci.yml`, and README-only About + topics. Verify no `TODO(readme)` and no “exactly-once” claim.

## 2. Demo seed

- [ ] 2.1 Add `scripts/demo.sh` (executable): wait for `/healthz`+`/readyz`, `keys create`, subscribe to `http://mock-subscriber:9090/hooks`, enqueue one event, poll mock logs for a signed POST whose idempotency key is the event id within ~5s. Verify it exits 0 against a live Compose stack and does not print the API key or signing secret after creation.
- [ ] 2.2 Point `docs/architecture/testing-strategy.md` smoke row at `scripts/demo.sh` and verify the documented command matches the script path.

## 3. Config audit

- [ ] 3.1 Audit `.env.example` against `internal/config` and Compose; pass documented `WORKER_*` through `x-courier-env`. Verify every `config.Load` name appears with a placeholder and `docker compose config -q` succeeds.

## 4. Spec index

- [ ] 4.1 Mark Day 8 done in `openspec/specs/README.md` and note `local-demo` / FR-DOC-001. Verify the capability table still lists current behaviour only (new `local-demo` folder lands at archive).

## 5. Verification

- [ ] 5.1 `openspec validate add-readme-demo --strict` passes.
- [ ] 5.2 `go test -race ./...` and `golangci-lint run` stay green (no Go behaviour change expected).

## Workflow follow-up

- After the PR is merged locally onto `main`, archive with `openspec archive add-readme-demo --yes` as a direct commit on `main`.
- Confirm CI on the PR head (a concurrency `cancelled` run on the merge commit is not a failure).
