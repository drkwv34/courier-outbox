# Architecture decision records

Short, immutable records of significant decisions. To change a decision, add a new ADR that supersedes the old one. Don't edit accepted ADRs beyond status links.

File name: `NNNN-kebab-title.md`. Sections: Status, Context, Decision, Consequences.

| ADR | Title | Status |
|-----|-------|--------|
| [0001](0001-postgres-outbox-with-redis-leases.md) | Postgres outbox with Redis as accelerator | Accepted |
| [0002](0002-chi-router.md) | chi for HTTP routing | Accepted |
| [0003](0003-goose-embedded-migrations.md) | goose with embedded SQL migrations | Accepted |
| [0004](0004-api-key-hmac-sha256-pepper.md) | API keys hashed with HMAC-SHA256 and a server pepper | Accepted |
| [0005](0005-testcontainers-integration-tests.md) | testcontainers-go for integration tests | Accepted |

Pending decisions (SRS §11) should become ADRs in the change that resolves them: sqlc vs hand-written pgx (hand-written pgx for now, see 0003) and serve+worker process topology.
