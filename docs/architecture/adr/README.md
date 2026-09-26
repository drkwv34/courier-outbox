# Architecture decision records

Short, immutable records of significant decisions. To change a decision, add a new ADR that supersedes the old one. Don't edit accepted ADRs beyond status links.

File name: `NNNN-kebab-title.md`. Sections: Status, Context, Decision, Consequences.

| ADR | Title | Status |
|-----|-------|--------|
| [0001](0001-postgres-outbox-with-redis-leases.md) | Postgres outbox with Redis as accelerator | Accepted |
| [0002](0002-chi-router.md) | chi for HTTP routing | Accepted |

Pending decisions (SRS §11) should become ADRs in the change that resolves them: sqlc vs hand-written pgx, goose vs golang-migrate, testcontainers vs dockertest, API-key hash algorithm, and serve+worker process topology.
