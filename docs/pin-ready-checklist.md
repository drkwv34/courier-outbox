# Pin-ready quality bar — courier-outbox v0.1.0

Closeout evidence for Day 9 (`add-pin-ready-closeout`). Status is **met** or **not met** only. Commands and CI ids were captured 2026-10-13 UTC on branch `chore/pin-ready` (base `main` at `2f8de2417b9825a338bee061bec2c26a91fa66c9`).

## Must-scope triage

| Check | Result | Evidence |
|-------|--------|----------|
| Open GitHub issues | None | `gh issue list --state all --json number,title,state` → `[]` |
| `CI` on `main` | Green | [run 38192172768](https://github.com/drkwv34/courier-outbox/actions/runs/38192172768) **success** on `2f8de24` (`docs(openspec): archive add-readme-demo after merge`). Jobs: OpenSpec validate, Lint, Unit tests (race), Integration tests (race, containers), Build, compose config, image. |
| `TODO` / `FIXME` in product code | None | ripgrep over `*.go` / `*.sql` / `*.yml` / `*.yaml` (excluding docs and OpenSpec archive comments) |
| Domain layering | Domain does not import `net/http` | `internal/domain/doc.go`; ripgrep `"net/http"` under `internal/domain` |
| Product Must defects found | **None** | Must FRs are covered by archived OpenSpec changes and `openspec/specs/{service-ops,api-keys,subscriptions,event-enqueue,delivery,dead-letter,local-demo}` |
| Docs hygiene fixed this change | Stale “planned” enqueue/delivery headings; `/openapi.json` still “once implemented”; capability index said “query isolation later” after tenant-scoped queries shipped | `docs/architecture/overview.md`, `openspec/specs/README.md` |
| Deferred | Should-scope only (not bugs) | [`openspec/backlog/should-scope.md`](../openspec/backlog/should-scope.md) |

No leftover Must-scope product bugs were found. Nothing Must-scope was deferred.

## Quality bar

| Item | Status | Evidence |
|------|--------|----------|
| Descriptive repository name | **met** | GitHub repo `drkwv34/courier-outbox`; `go.mod` `module github.com/drkwv34/courier-outbox` |
| GitHub About blurb + topics | **not met** (manual) | One API attempt 2026-10-13: `gh repo edit drkwv34/courier-outbox --description "…" --add-topic …` → **HTTP 403** `Resource not accessible by integration` (`https://api.github.com/repos/drkwv34/courier-outbox`). `gh repo view --json description,repositoryTopics` still `{ "description": "", "repositoryTopics": null }`. **Do not retry in automation.** Set manually: description `Reliable outbound webhook delivery: signed payloads, retries with backoff, idempotency keys, and a dead-letter queue — in Go.` Topics: `go`, `webhooks`, `postgresql`, `redis`, `openapi`, `docker`, `github-actions`, `reliability`, `idempotency`. Same fallback already in `README.md` § GitHub repository metadata. |
| README case study order | **met** | `README.md` headings: courier-outbox (H1 + value story) → Problem → Why courier-outbox → Stack → Architecture → Run locally → Tests and CI → Worker and signature scheme → Consumer responsibilities → Trade-offs → GitHub repository metadata → Contributing → License |
| Trustworthy `docker compose up` / demo script path | **met** | `docker-compose.yml` (migrate, api, worker, db Postgres 16, redis, mock-subscriber); `README.md` § Run locally: `cp .env.example .env` then `docker compose up --build` then `./scripts/demo.sh`. Script is executable (`scripts/demo.sh`). |
| Incremental intent-bearing commits | **met** | Conventional commits on `main` (e.g. `feat(api):`, `feat(store):`, `test:`, `docs:`, `chore(openspec):`). This closeout uses several themed commits on `chore/pin-ready`. |
| Automated tests (unit + integration) | **met** | Unit: `go test -race ./...`. Integration: `go test -race -tags=integration ./...` (testcontainers Postgres 16 + Redis 7, ADR 0005). Docs: `docs/architecture/testing-strategy.md`. CI jobs `test` and `integration`. |
| Green Actions on `main` | **met** | Workflow `.github/workflows/ci.yml`. Latest `main` push: run **38192172768** success. README badge: `https://github.com/drkwv34/courier-outbox/actions/workflows/ci.yml/badge.svg?branch=main` |
| Compose | **met** | `docker-compose.yml`; `docker compose config -q` in CI build job and local docs |
| No secrets in history + `.env.example` | **met** | `.gitignore` ignores `.env` / `.env.*` with `!.env.example`; `.env.example` documents placeholders only. `git log --all --diff-filter=A --name-only` found no committed `.env`. Compose peppers/keys are documented local dummies. |
| Architecture section | **met** | `README.md` § Architecture (mermaid); `docs/architecture/README.md` and linked convention docs |
| MIT LICENSE | **met** | `LICENSE` (MIT, Copyright 2026 Christian Agila); `README.md` § License |
| Real Fission OpenSpec (skills, commands, archive) | **met** | CLI `@fission-ai/openspec` (`openspec/README.md`, CI job `openspec`). Skills: `.cursor/skills/openspec-{propose,apply-change,archive-change,sync-specs,update-change,explore}/SKILL.md`. Commands: `.cursor/commands/opsx-{propose,apply,archive,sync,update,explore}.md`. Archive history: `openspec/changes/archive/` (persistence through `2026-10-12-add-readme-demo`). This change: `openspec/changes/add-pin-ready-closeout/` (`skip_specs: true`) validated with `openspec validate add-pin-ready-closeout --strict`. |
| Rules + agentic docs first | **met** | Always-on rule `.cursor/rules/00-project-core.mdc`; path rules under `.cursor/rules/`; `AGENTS.md`; architecture docs in `docs/architecture/` (overview, layering, domain, errors, transactions, observability, integrations, security, testing, Go conventions, ADRs). |
| Resume / LinkedIn consistency | **met** | README opening value story is the same sentence as the intended About blurb. |

## About / topics attempt (single try)

```
$ gh repo edit drkwv34/courier-outbox --description "Reliable outbound webhook delivery: signed payloads, retries with backoff, idempotency keys, and a dead-letter queue — in Go." --add-topic go --add-topic webhooks --add-topic postgresql --add-topic redis --add-topic openapi --add-topic docker --add-topic github-actions --add-topic reliability --add-topic idempotency
HTTP 403: Resource not accessible by integration (https://api.github.com/repos/drkwv34/courier-outbox)
```

No second attempt.

## Optional `v0.1.0` tag

Attempted **after** merge to `main` (not on the PR head). Result is recorded in the Day 9 closeout report, not assumed here.
