# migrations

Forward-only SQL migrations for PostgreSQL 16+ (SRS NFR-MAINT-002).

The tool choice (goose vs golang-migrate) lands with the first schema change.
Record it in an ADR under `docs/architecture/adr/` when it does.

Rules:

- **Forward-only.** Never edit or delete a migration after it has merged to `main`. Fix mistakes with a new migration.
- **Naming:** `NNNNNN_short_snake_description.sql` with zero-padded, monotonically increasing numbers (e.g. `000001_create_api_keys.sql`).
- **One concern per file.** A table and its indexes can share a file. Unrelated tables cannot.
- **Plain SQL.** No Go-coded migrations unless a data backfill truly needs them.
- **Constraints are part of the design.** Put uniqueness (e.g. `UNIQUE (api_key_id, idempotency_key)`), foreign keys, `CHECK` on status enums, and `NOT NULL` in the database, not only in Go.
- **Indexes follow query shapes.** SRS §6 already names `(status, next_attempt_at)` and `(subscription_id, created_at)` on `deliveries`.
- **Safe on a populated table.** Use `CREATE INDEX CONCURRENTLY` where the tool allows it, and add a column as nullable before backfilling.
