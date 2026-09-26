# AGENTS.md

Guide for coding agents (Cursor / Composer) and humans working on courier-outbox.

## Read first

1. `openspec/project.md`: what this is and the stack
2. `openspec/README.md`: **no non-trivial feature without an approved change**
3. `docs/architecture/README.md`: normative conventions (layering, errors, transactions, logging, integrations, testing, Go style)
4. `.cursor/rules/*.mdc`: condensed rules, auto-attached by path

## Layout

```
cmd/courier/            composition root (serve today; worker/migrate/keys later)
cmd/mock-subscriber/    local demo webhook target (never deployed)
internal/api/           HTTP transport + error→HTTP mapping (only place that knows status codes)
internal/config/        env → validated Config
internal/domain/        pure model, invariants, state machine, backoff, sentinel errors
internal/store/         Postgres/Redis adapters, SQL, transaction boundaries
internal/worker/        claim → sign → send → record loop, outbound HTTP client
internal/sign/          v1 HMAC-SHA256 signer/verifier
migrations/             forward-only SQL
openapi/                committed OpenAPI 3 contract
openspec/               specs (current truth) + changes (proposals)
docs/architecture/      conventions + ADRs
```

## Commands

```bash
go build ./...
go test -race ./...
golangci-lint run            # v2 config in .golangci.yml
docker compose config -q     # validate compose
docker compose up --build    # api + postgres + redis + mock-subscriber
```

## Working agreement

- **One change per branch/PR.** Map it to an OpenSpec change id and keep PRs small.
- **Conventional commits:** `feat(scope):`, `fix(scope):`, `docs:`, `test:`, `chore:`, `ci:`, `refactor:`. Make them small and intent-bearing.
- **Git identity is the repo owner's.** Never set an agent or bot as author or co-author, and never add AI `Co-authored-by` trailers.
- **No secrets.** Put placeholders in `.env.example` and nothing real anywhere.
- **Contract travels with code.** A handler change must update `openapi/openapi.yaml` and its tests in the same PR.
- **Definition of done** for a change: spec deltas are written, tasks are ticked, `go test -race ./...` and `golangci-lint run` are green, OpenAPI and `.env.example` are updated, and the change is archived after merge.
- **Stay in scope.** Implement only what the active change lists. Log follow-ups as new changes and don't sneak them in.

## Things that must never happen

- Holding a DB transaction open during an outbound HTTP call
- Following redirects or dialing private IPs when `SSRF_PROTECTION=true`
- Returning or logging signing secrets, API keys, or payload bodies
- Status codes or `net/http` imports inside `internal/domain`
- Claiming "exactly-once" anywhere. Courier is at-least-once.
