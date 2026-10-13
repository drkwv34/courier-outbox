# Proposal

- **Id:** `add-pin-ready-closeout`
- **Status:** approved
- **SRS requirements:** FR-DOC-001, NFR-PORT-001 (evidence only); leftover Must FRs already specified under `openspec/specs/`

## Why

courier-outbox is at the final pin-ready closeout (Day 9). Reviewers need evidence that Must-scope product work is complete, leftover Must bugs are triaged, Should-scope items are recorded without being implemented, and every portfolio quality-bar item is marked met or not met with file/CI/command evidence.

## What Changes

- Triage leftover **Must** bugs only (open GitHub issues, failing/flaky CI, SRS Must gaps, `TODO`/`FIXME` in product code). Fix small real defects; list anything deferred. No new product features.
- Commit `docs/pin-ready-checklist.md` covering every portfolio quality-bar item with **met** / **not met** plus evidence (path, CI run URL, or command).
- Record Should-scope items as a standing OpenSpec backlog note under `openspec/backlog/should-scope.md` (proposal-only; do **not** implement them).
- Attempt GitHub About description + topics once via API; if 403, document the manual fallback in the checklist (do not retry loops).
- Optional annotated tag `v0.1.0` on the merged `main` commit (and a GitHub release if the API allows).
- Keep `main` green after merge; archive this change on `main` with the Fission-AI OpenSpec CLI.

No **BREAKING** API or schema changes are planned. Product capabilities already specified under `openspec/specs/` stay as-is unless a Must-scope defect requires a tiny fix that does not change requirements.

## Capabilities

### New Capabilities

- _(none — this change is pin-ready closeout documentation and triage, not a new product capability)_

### Modified Capabilities

- _(none — no spec-level product behavior changes; Should items stay unimplemented)_

This change sets `skip_specs: true` in `.openspec.yaml` because it is docs/process closeout: a quality-bar checklist, a Should-scope backlog note, and Must-only defect triage. Inventing a `pin-ready` product spec would not describe system behavior.

## Impact

- `docs/pin-ready-checklist.md` (new)
- `openspec/backlog/should-scope.md` (new standing backlog; not a spec)
- Possibly small Must-scope docs hygiene (stale “planned” labels on shipped enqueue/delivery flows in `docs/architecture/overview.md`; capability index still says “query isolation later” after tenant-scoped queries shipped)
- `openspec/specs/README.md` Day 9 status
- GitHub repository About description + topics (best-effort, one API attempt)
- Optional `v0.1.0` tag after merge
- No Should-scope implementation, no beacon-watch / semaphore-flags / kickoff-club / profile-readme feature work, no history rewrite

## Out of scope

- New courier product features beyond Must triage.
- Should-scope builds (dial-time SSRF, `/docs`, load tests, auto-disable-after-410, purge/retention, spectral contract CI, Compose-smoke CI job).
- Beacon-watch / semaphore-flags / kickoff-club / profile-readme feature work (state advance to beacon-watch day 2 after close only).
- Exactly-once claims.
- GitHub About/topics API retry loops after one 403.
- History rewrite or `Co-authored-by` trailers.
