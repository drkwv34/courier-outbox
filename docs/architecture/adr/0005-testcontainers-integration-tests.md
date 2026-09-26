# 0005. testcontainers-go for integration tests

- **Status:** Accepted
- **Date:** 2026-09-26
- **Source:** SRS §7, §11; change `add-persistence-and-api-keys`

## Context

Store, worker, and idempotency tests need real Postgres 16 and Redis 7. The options were testcontainers-go, ory/dockertest, or CI service containers plus a DSN env var.

## Decision

Use `github.com/testcontainers/testcontainers-go` with its `postgres` and `redis` modules. Integration tests carry `//go:build integration`, start their own containers, apply the real embedded migrations, and run with `go test -race -tags=integration ./...`.

## Consequences

- The same command works on a laptop with Docker and on `ubuntu-latest` in CI, with no service definitions to keep in sync.
- The default `go test ./...` stays container-free and fast.
- testcontainers adds a sizeable test-only dependency tree. It doesn't ship in the binary.
- Each package that has integration tests starts its own containers, which costs a few seconds per package but keeps tests isolated.
