# 0003. goose with embedded SQL migrations

- **Status:** Accepted
- **Date:** 2026-09-26
- **Source:** SRS NFR-MAINT-002, §11; change `add-persistence-and-api-keys`

## Context

Migrations must be forward-only, plain SQL, and runnable from the single `courier` binary, including inside the distroless image that has no shell and no extra files. The candidates were golang-migrate and goose.

## Decision

Use `github.com/pressly/goose/v3` through its `Provider` API. The `.sql` files in `migrations/` are compiled in with `embed.FS`, and `courier migrate` applies them. Each file has a `-- +goose Up` section and **no** Down section. Queries use hand-written pgx v5; goose talks to Postgres through pgx's `database/sql` driver.

## Consequences

- The binary is self-contained: `docker compose run --rm migrate` needs nothing beyond `DATABASE_URL`.
- One file per migration (golang-migrate wants separate up/down files), and annotations let a migration opt out of its transaction (`-- +goose NO TRANSACTION`) when it needs `CREATE INDEX CONCURRENTLY`.
- Applied versions live in `goose_db_version`. Re-running is a no-op.
- goose and pgx are pinned to the newest releases that support the Go version in `go.mod`. Bumping Go is a separate change.
