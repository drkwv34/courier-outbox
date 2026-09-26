# Architecture docs

These are the normative conventions for courier-outbox, derived from SRS §8. Humans and agents follow them, and `.cursor/rules/` holds the condensed, auto-attached version. If code and docs disagree, fix one of them in the same PR.

| Doc | Covers |
|-----|--------|
| [overview.md](overview.md) | System context, components, request and delivery flows |
| [patterns-layering.md](patterns-layering.md) | Package layout, dependency rule, patterns we use and avoid |
| [domain-model.md](domain-model.md) | Entities, invariants, delivery state machine, backoff schedule |
| [error-handling.md](error-handling.md) | Sentinels, wrapping, HTTP mapping, error codes |
| [transactionality.md](transactionality.md) | TX boundaries, idempotent writes, SKIP LOCKED claims, leases |
| [observability.md](observability.md) | slog fields, levels, correlation ids, redaction, metrics |
| [external-integrations.md](external-integrations.md) | Outbound HTTP client, timeouts, retries, SSRF, Postgres/Redis clients |
| [security.md](security.md) | API keys, signing secrets, encryption at rest, config hygiene |
| [testing-strategy.md](testing-strategy.md) | Unit / integration / smoke, what runs in CI |
| [go-conventions.md](go-conventions.md) | Go idioms, concurrency, dependencies, style |
| [adr/](adr/) | Architecture decision records |

Changing a convention? Open an OpenSpec change (or at least an ADR) first. Don't let the code drift silently.
