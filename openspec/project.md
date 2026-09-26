# Project context: courier-outbox

## Purpose

courier-outbox is a reliable outbound webhook delivery service. Producers enqueue events over an authenticated HTTP API. Courier persists each event transactionally alongside one delivery row per matching subscription. Workers then claim due deliveries, sign the payloads (HMAC-SHA256), POST them to subscriber endpoints with strict timeouts, retry on the schedule 1s → 5s → 25s → 2m → 10m, and dead-letter what is still failing. Delivery is **at-least-once**, never exactly-once.

## Tech stack

- Go (version pinned in `go.mod`), stdlib-first.
- `github.com/go-chi/chi/v5` for routing and middleware.
- PostgreSQL 16+ as the system of record: events, deliveries, attempts, DLQ.
- Redis 7+ for lease fencing and per-subscription outbound rate limits (never the only copy of state).
- `log/slog` JSON logs, Prometheus metrics (planned).
- Docker Compose for local; GitHub Actions for CI; golangci-lint.
- Integration tests with testcontainers-go ([ADR 0005](../docs/architecture/adr/0005-testcontainers-integration-tests.md)).

## Conventions

Normative conventions live in `docs/architecture/` and are enforced for agents by `.cursor/rules/`. Start at `AGENTS.md` in the repo root. Highlights:

- Layout: `cmd/courier`, `internal/{api,worker,domain,store,sign}`, `migrations/`, `openapi/`.
- The domain is pure. Errors are sentinels wrapped with context and mapped to HTTP only in `internal/api`.
- Config comes from env and is validated at startup. Raw SQL or sqlc, never an ORM.
- Commits are conventional (`feat(scope):`, `fix:`, `docs:`, `chore:`, `test:`, `ci:`), small, and intent-bearing.

## Source documents

- SRS: `02-courier-outbox-srs.md` (lives outside the repo). The FR/NFR ids quoted in specs come from it.
- Capability index: `openspec/specs/README.md`.

## Domain vocabulary

Producer, subscription, event, delivery, attempt, lease, DLQ (dead letter), idempotency key, signing secret. Definitions are in `docs/architecture/domain-model.md`.
