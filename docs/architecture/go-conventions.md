# Go conventions

## Toolchain and style

- The Go version is pinned in `go.mod`. CI uses `go-version-file: go.mod`.
- Code is `gofmt`/`goimports` clean and `golangci-lint run` clean (config in `.golangci.yml`). Don't disable a linter inline without a `//nolint:<linter> // reason` comment.
- Every exported identifier has a doc comment that starts with its name (SRS §8.9). Each package has a `doc.go` or package comment stating its responsibility.

## Packages and naming

- Name packages for what they provide (`sign`, `store`), in lowercase with no underscores. Never `util`, `common`, `helpers`, or `models`.
- Avoid stutter: `store.Postgres`, not `store.PostgresStore`.
- Receivers are short and consistent (`s *Postgres`, `h *handler`).
- Error vars are `ErrX`. Error strings are lowercase with no trailing punctuation.
- Acronyms keep their case: `HTTPAddr`, `APIKeyID`, `URL`.

## Construction and dependencies

- Use constructor injection: `func NewX(deps...) *X`. Validate required deps and panic on nil in constructors (a programmer error).
- No package-level mutable state and no `init()` with side effects. The composition root is `cmd/courier`.
- Configuration arrives as typed structs from `internal/config`. Packages never read `os.Getenv` themselves.
- Accept interfaces and return concrete types. Declare interfaces where they are consumed.

## Context

- `ctx context.Context` is the first parameter of anything that does I/O or may block.
- Never store a context in a struct, and never pass `nil`. Use `context.Background()` only in `main` and tests.
- Respect cancellation in loops (`select { case <-ctx.Done(): ... }`).

## Concurrency

- Every goroutine has an owner, a way to stop, and an error path. Prefer `golang.org/x/sync/errgroup` for groups.
- The worker pool has a fixed size from config. Shutdown stops claiming, waits for in-flight attempts up to the shutdown timeout, then cancels.
- Don't share maps between goroutines without a mutex. Prefer passing ownership over channels.
- Run `go test -race` on every PR.

## HTTP (inbound)

- Use chi router, with middleware from `chi/middleware` where it fits (RequestID, RealIP, Recoverer).
- Handlers are methods on a struct holding their dependencies, or closures returned by a constructor. No globals.
- DTOs live in `internal/api` with explicit `json:"snake_case"` tags. Map them to and from domain types explicitly.
- Decode with `json.NewDecoder(http.MaxBytesReader(...))` and `DisallowUnknownFields()`, and reject trailing data.
- Write responses through the shared `writeJSON` / `writeError` helpers.

## Time, IDs, numbers

- Use UTC everywhere. Use `time.Duration` for durations in config and code, never bare ints except at the env-parsing edge (`COURIER_BACKOFF_MS`).
- Domain logic uses an injected `Clock` interface. Only `cmd/` and adapters call `time.Now()` directly.
- IDs are UUIDs (library chosen in the first persistence change) wrapped in typed IDs.

## Dependencies

Start with the standard library. Pre-approved dependencies:

- `github.com/go-chi/chi/v5`: router
- `github.com/jackc/pgx/v5`: Postgres driver/pool
- `github.com/redis/go-redis/v9`: Redis
- goose **or** golang-migrate: migrations (pick one)
- testcontainers-go **or** ory/dockertest: integration tests (pick one)
- `github.com/prometheus/client_golang`: metrics
- `golang.org/x/sync`: errgroup / singleflight

Anything else needs a one-line justification in the PR description.

## Generics and cleverness

Use generics only where they remove real duplication (e.g. a typed pagination helper). Prefer boring, explicit code that a reviewer can follow in one read.
