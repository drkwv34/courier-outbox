# Tasks: <change-id>

<!-- Ordered; each top-level item should roughly map to one commit. -->

## 1. Spec & contract
- [ ] 1.1 Spec deltas written and validated
- [ ] 1.2 `openapi/openapi.yaml` updated (if HTTP surface changes)

## 2. Implementation
- [ ] 2.1 Migration(s)
- [ ] 2.2 Domain types / rules + unit tests
- [ ] 2.3 Store implementation + integration tests
- [ ] 2.4 API handlers + error mapping + handler tests

## 3. Wrap-up
- [ ] 3.1 `.env.example` and README updated
- [ ] 3.2 `go test ./...` and `golangci-lint run` green
- [ ] 3.3 Change archived and `openspec/specs/` updated after merge
