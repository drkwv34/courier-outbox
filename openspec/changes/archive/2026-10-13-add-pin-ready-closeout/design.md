# Design

## Context

See `proposal.md` for motivation. courier-outbox on `main` (`2f8de24`) already ships Must-scope product (API keys, subscriptions, enqueue/idempotency, delivery worker + HMAC + retries, DLQ/replay, OpenAPI + metrics, pin-quality README + Compose demo). GitHub has **zero open issues**. Latest `CI` run on `main` (workflow run `38192172768`) succeeded on `2f8de24`. Product packages stay untouched unless triage finds a small Must defect.

## Goals / Non-Goals

**Goals:**

- Evidence-first closeout: every quality-bar item in `docs/pin-ready-checklist.md` is **met** or **not met** with a concrete path, CI run URL, or command.
- Standing Should-scope backlog under `openspec/backlog/` that survives archive (not only this change’s proposal).
- Honest triage: if nothing Must-scope is broken, say so; still fix tiny docs inaccuracies that would fail the quality bar (stale “planned” flow headings; capability index “query isolation later”).

**Non-Goals:**

- Spec deltas for product capabilities (this change uses `skip_specs: true`).
- Implementing any Should item, or feature work in beacon-watch / semaphore-flags / kickoff-club / profile-readme.
- Rewriting README, Compose, or CI unless a Must defect is proven.

## Decisions

### 1. Skip spec deltas

- **Choice:** `skip_specs: true`. Closeout docs are not product behavior; inventing a `pin-ready` capability would fail the “durable system behavior” rule.
- **Alternative:** Add a `pin-ready` spec. Rejected — `openspec validate` would then require SHALL/WHEN/THEN for a checklist file, which is process not product.

### 2. Standing backlog path

- **Choice:** `openspec/backlog/should-scope.md` (proposal-only note; no tasks to implement). Archive moves the change folder but this file stays in-tree.
- **Alternative:** Only list Should items in this change’s `proposal.md`. Rejected — archive would bury the only backlog record.

### 3. Quality-bar evidence format

- **Choice:** Markdown tables with columns: item, status (`met` / `not met`), evidence. About/topics: try `gh repo edit` / PATCH once; on 403, mark **not met (manual)** and stop. Mirror kickoff-club Day 13.
- **Alternative:** Claim About is met because README already lists the blurb. Rejected — the bar is the GitHub repository metadata, not a README fallback.

### 4. Tiny Must-scope docs hygiene (not features)

- **Choice:** Drop “planned” from `docs/architecture/overview.md` enqueue/delivery headings (those flows shipped Days 4–5). Update `openspec/specs/README.md` so API-keys status no longer says “query isolation later” (tenant-scoped queries shipped with enqueue/DLQ) and mark Day 9 pin-ready closeout.
- **Alternative:** Leave them stale. Rejected — “rules + agentic docs first” and an honest architecture section would be weaker evidence.

### 5. Dial-time SSRF stays Should

- **Choice:** Do not implement `net.Dialer.Control` resolved-IP blocking. Current spec (`openspec/specs/subscriptions`) marks hostname resolution at dial time out of scope; create-time URL policy is the v0.1 Must. Record dial-time SSRF, `/docs`, load tests, auto-disable-410, and peers in the Should backlog.
- **Alternative:** Treat FR-SUB-003 dial-time as a leftover Must and implement it. Rejected — multiple archived changes explicitly deferred it; implementing it is Should-scope.

### 6. Tag and merge order

- **Choice:** Open a ready PR from `chore/pin-ready`. Merge only after all CI jobs succeed on the PR head. Merge locally as Christian Agila with `git merge --no-ff` (never the GitHub merge button/API). Archive on `main` as a direct commit (`openspec archive add-pin-ready-closeout --yes`; `--skip-specs` because skip_specs is already declared). Optional annotated `v0.1.0` on the post-archive `main` SHA if tagging is allowed.
- **Alternative:** Tag the PR head. Rejected — pin version should point at merged `main`.

## Risks / Trade-offs

- [GitHub About PATCH returns 403] → Record as **not met (manual)** in the checklist; do not loop.
- [No Must bugs exist] → Checklist triage section states “none found”; do not invent product work.
- [Co-author hook reintroduces trailers] → Local `commit-msg.cursor.co-author` is a no-op; verify every commit message before push.
- [Archive `--skip-specs` vs `--yes` only] → Prefer `--yes` first; add `--skip-specs` if the CLI still wants a spec sync that skip_specs already declined.
- [Concurrency-cancelled CI on the merge commit] → Judge the PR head, not the cancelled merge run.

## Migration Plan

Not applicable (no schema or API migration). Rollback is revert of the closeout commits on `main`.

## Open Questions

None that affect approach or tasks. Tag `v0.1.0` is optional per the Day 9 brief; attempt after archive if `git`/`gh` allow it.
