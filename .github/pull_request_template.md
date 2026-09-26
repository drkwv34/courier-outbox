## What & why

<!-- One paragraph. Link the OpenSpec change: openspec/changes/<id>/ -->

- OpenSpec change: `<id>` (or "n/a: trivial/docs/chore")
- SRS requirements: <!-- FR-XXX-000, NFR-XXX-000 -->

## Checklist

- [ ] Scope matches the OpenSpec change / night's objective. No unrelated features.
- [ ] `go test -race ./...` and `golangci-lint run` pass locally
- [ ] `openapi/openapi.yaml` updated for any HTTP change
- [ ] New config documented in `.env.example` (placeholders only, no secrets)
- [ ] Errors mapped only in `internal/api`; no secrets or payload bodies logged
- [ ] No DB transaction held across outbound HTTP
- [ ] README "Consumer responsibilities" still accurate (FR-DOC-001): verify signature and reject timestamp skew > 5 min; dedupe on `X-Courier-Idempotency-Key`; 2xx only after durable accept; respond fast
- [ ] Commits are conventional and authored by the repo owner (no AI co-author trailers)

## How to verify

<!-- Commands / curl calls a reviewer can run. -->
