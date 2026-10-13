# Tasks: add-pin-ready-closeout

## 1. OpenSpec planning

- [x] 1.1 Create change `add-pin-ready-closeout` with proposal, skipped specs (`skip_specs: true`), design, and this task list; verify `openspec status --change add-pin-ready-closeout` shows proposal/design/tasks done and specs skipped

## 2. Must-scope triage and docs hygiene

- [ ] 2.1 Triage leftover Must bugs (GitHub issues, latest CI on main, SRS Must vs shipped specs, `TODO`/`FIXME` in product code); record findings in `docs/pin-ready-checklist.md` and fix only small real defects if found (verify: `gh issue list --state open` is empty or listed, and any code fix has a test or command)
- [ ] 2.2 Drop stale “planned” labels from `docs/architecture/overview.md` enqueue/delivery headings (those flows shipped) and note `/openapi.json` is served; verify the headings no longer say “planned”
- [ ] 2.3 Update `openspec/specs/README.md` so API-keys status reflects tenant-scoped queries and Day 9 pin-ready closeout is marked; verify the capability table still describes current behaviour only

## 3. Should-scope backlog (do not implement)

- [ ] 3.1 Add `openspec/backlog/should-scope.md` listing dial-time SSRF, `/docs`, load tests (NFR-PERF-001/002), FR-ERR-003 auto-disable-after-410, purge/retention, spectral contract CI, Compose-smoke CI job, and related Should items as proposal-only; verify the file states explicitly that none of these are implemented in this change

## 4. Quality-bar checklist and GitHub metadata

- [ ] 4.1 Commit `docs/pin-ready-checklist.md` with every portfolio quality-bar item marked met / not met plus evidence (path, CI run URL, or command); verify all brief items are present
- [ ] 4.2 Attempt GitHub About description + topics once via API; on 403 record **not met (manual)** in the checklist and stop (verify: API response recorded; no retry loop)

## 5. Validation

- [ ] 5.1 Run `openspec validate add-pin-ready-closeout --strict`, `go test -race ./...`, and `golangci-lint run`; verify all succeed

## Workflow follow-up

- Open a ready (non-draft) PR to `main` from `chore/pin-ready`.
- Wait for CI on the PR head; fix and push if it fails; merge only when green. A concurrency-cancelled run on the merge commit is not a failure; judge the PR head.
- Merge locally as Christian Agila: `git checkout main && git pull && git merge --no-ff chore/pin-ready -m "Merge pull request #<n> from drkwv34/chore/pin-ready" && git push origin main`. Never use the GitHub merge button or API.
- After merge, archive on `main` as Christian Agila: `openspec archive add-pin-ready-closeout --yes` (add `--skip-specs` if the CLI still requests a spec sync).
- Confirm CI on `main`; optionally annotated-tag `v0.1.0` and create a GitHub release if the API allows.
