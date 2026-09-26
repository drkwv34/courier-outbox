# Patterns and layering

## Dependency rule

```
cmd/courier ──► internal/api ────┐
            ├─► internal/worker ─┼──► internal/domain   (imports nothing internal)
            ├─► internal/store ──┘
            └─► internal/config
internal/worker ──► internal/sign   (stdlib only)
```

- `domain` imports only the standard library (plus a UUID package if we adopt one). It never imports `api`, `store`, `worker`, `net/http`, `database/sql`, pgx, or Redis.
- `api` and `worker` depend on **interfaces they declare themselves**, never on concrete `store` types. `cmd/courier` wires in the concrete store.
- `store` may import `domain` (to return domain types and sentinels). It never imports `api` or `worker`.
- `sign` is a leaf. The worker and the mock subscriber (for verification) may import it.
- `internal/` packages never import `cmd/`. Circular imports are a design smell. When one appears, move the shared type into `domain`.

## Where logic lives

| Kind of logic | Home | Example |
|---------------|------|---------|
| Business rules and invariants | `domain` | idempotency key length, event-type matching, `NextDelay(attempt)` |
| Transactional use cases (multi-row writes) | `store`, as one use-case-shaped method | `EnqueueEvent(ctx, params)` inserts the event and its deliveries in one TX |
| Transport concerns | `api` | decode, auth, status codes, headers such as `Idempotent-Replay` |
| Scheduling and I/O orchestration | `worker` | claim → sign → send → record |
| Crypto | `sign` | `Sign(secret, ts, body)` |

A store method may run a transaction and call pure domain functions to make decisions. It must not make business decisions inline in SQL that the domain cannot unit-test. The exception is filters that are clearly queries, such as "enabled subscriptions matching type X".

Handlers stay thin: **decode → validate (domain) → call interface → map result/error → encode**. A handler with more than roughly 40 lines is a signal to push logic down.

## Interfaces

- Define an interface **in the consuming package** and keep it small, e.g. `api.EventStore` with only the methods the handlers call.
- Return concrete structs from constructors (`store.NewPostgres(...) *Postgres`).
- Don't create an interface with a single implementation unless a test needs a fake.

## Patterns we use

- **Transactional outbox.** The event and its deliveries are committed atomically, and workers drain from Postgres.
- **Competing consumers.** Multiple workers claim with `FOR UPDATE SKIP LOCKED`.
- **Lease + fencing token.** `lease_owner` and `lease_until` go on the row, and every completion update is conditioned on `lease_owner`.
- **Idempotency key.** A producer-scoped `UNIQUE` constraint, with replay returning the original result.
- **Ports and adapters (light).** Consumer-declared interfaces, with store and HTTP client as adapters.
- **Constructor injection.** Dependencies are passed into `New...` functions. There are no service locators or DI frameworks.
- **Injected clock and randomness.** Domain and worker take a `Clock` / `io.Reader`, so tests are deterministic.
- **Table-driven policies.** The backoff schedule and SSRF deny-list are data, not branching code.

## Patterns we avoid

- ORMs or query builders that hide SQL, and generic `Repository[T]` abstractions.
- Package-level mutable singletons (global DB pool, global logger) outside `cmd/`.
- `utils`, `common`, `helpers` packages. Name packages for what they provide.
- Event sourcing, CQRS buses, and microservice splits. This is one deployable.
- In-process retry loops around subscriber POSTs. Retries are durable and scheduled via the DB.
- Reflection-heavy magic (struct-tag routers, auto-wiring).

## Contract-first HTTP

`openapi/openapi.yaml` is updated **in the same PR** as any handler change. The handler, the OpenAPI entry, and a handler test form one unit of work.
