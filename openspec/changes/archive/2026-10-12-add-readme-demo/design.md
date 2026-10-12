# Design

## Context

See proposal.md for motivation. Delivery, signing, enqueue, and mock-subscriber already work in Compose. README still has `TODO(readme)` and an “OpenAPI + metrics” status line. `docs/architecture/testing-strategy.md` names `scripts/smoke.sh`, which does not exist. Compose does not pass `WORKER_*` from `.env` even though `internal/config` and `.env.example` document them. GitHub Settings API is 403; About/topics must be README-only. Constraints: no new HTTP surface unless required for smoke; never log keys, signing secrets, or payload bodies; never claim exactly-once.

## Goals / Non-Goals

**Goals:**

- Pin-quality README a reviewer can quote as the reliability story.
- `scripts/demo.sh` that encodes NFR-PORT-001 against live Compose.
- `.env.example` and Compose env aligned with `internal/config`.
- About + topics copied in README, not mutated via API.

**Non-Goals:**

- New api/mock-subscriber routes or a received-webhooks store.
- CI job that runs the Compose smoke (testing-strategy still says “before release”).
- Dial-time SSRF, `/docs`, worker scrape port, tags.

## Decisions

- **Script path `scripts/demo.sh`.** Brief’s first example; testing-strategy will point here instead of the missing `scripts/smoke.sh`. Alternative: a Compose one-shot profile. Rejected — reviewers already run `docker compose up --build`; a host script talking to published ports is simpler to read and debug.
- **Verify via mock logs, not a new mock API.** `mock-subscriber` already JSON-logs `idempotency_key` (event id) and `has_signature`. The script polls `docker compose logs mock-subscriber` for a line whose key matches the enqueue `event_id` and `has_signature` is true, with a ~5s deadline from enqueue. Alternative: `GET /received`. Rejected — new feature surface. Alternative: poll `GET /v1/deliveries` only. Rejected — that proves worker state, not that the mock received a signed POST.
- **JSON parsing with Python 3 stdlib.** Always available in this environment; jq is optional. The script MUST NOT echo the raw API key or `signing_secret` after the create step (print “created” + ids only).
- **Target URL `http://mock-subscriber:9090/hooks`.** Worker runs on the Compose network; Compose already sets `SSRF_PROTECTION=false` and `ALLOW_HTTP_CALLBACKS=true` for this path.
- **CI badge** `https://github.com/drkwv34/courier-outbox/actions/workflows/ci.yml/badge.svg` linking the same workflow (name `CI`, default branch `main`).
- **About/topics in a dedicated README subsection** with the exact strings from the Day 8 brief. No `gh api` / Settings calls.
- **Compose `WORKER_*` pass-through** on the shared `x-courier-env` anchor so documented knobs work. Defaults match `internal/config`. No new names.
- **No OpenAPI or handler change.** Demo uses existing endpoints.

## Risks / Trade-offs

- **[Risk] Script races Compose boot.** → Mitigation: wait on `/healthz` and `/readyz` before `keys create`; fail with “run docker compose up --build first” if the wait expires.
- **[Risk] Log scrape misses a line if the mock log format changes.** → Mitigation: match slog JSON field names already emitted (`idempotency_key`, `has_signature`, `method`). Keep the check in one function.
- **[Risk] `docker compose run keys create` prints the raw key.** → Mitigation: parse `key:` from that output in-process; never `echo` it; do not log curl bodies.
- **[Trade-off] ~5s is wall-clock from enqueue, not from claim.** Worker poll default is 500ms; first attempt is immediate. Five seconds is the NFR budget, not a load SLO.
- **[Trade-off] Compose smoke is local/reviewer, not a PR CI job.** Unit/integration CI stays the merge gate. A cancelled merge-commit run is not a failure.

## Migration Plan

- Docs and a script only. Rollback: revert the commit. No schema or API version bump.

## Open Questions

None. About/topics stay README-only; smoke stays a host script after Compose is up.
