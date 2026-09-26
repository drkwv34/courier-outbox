# Tasks: add-persistence-and-api-keys

## 1. Spec & contract
- [x] 1.1 Spec deltas written (`api-keys`, `service-ops`)
- [x] 1.2 ADRs for migration tool, key hash, and integration harness
- [x] 1.3 `openapi/openapi.yaml` updated (`/readyz`, bearer security scheme, 401 response)

## 2. Implementation
- [x] 2.1 Config: `DATABASE_URL`, `REDIS_URL`, `COURIER_API_KEY_PEPPER` validated at startup
- [x] 2.2 Migrations `000001`–`000005` embedded, plus `courier migrate`
- [x] 2.3 Domain: API key generate / parse / hash / verify, plus unit tests
- [x] 2.4 Store: Postgres pool, api-key create and lookup, Redis ping, plus integration tests
- [x] 2.5 API: Bearer auth middleware on `/v1`, `/readyz`, plus handler tests
- [x] 2.6 CLI: `courier keys create --name`
- [x] 2.7 Compose: one-shot `migrate` service gating `api`

## 3. Wrap-up
- [x] 3.1 `.env.example`, README, and AGENTS.md updated
- [x] 3.2 CI integration job enabled
- [x] 3.3 `go test -race ./...`, integration tests, and `golangci-lint run` green
- [x] 3.4 Change archived and `openspec/specs/` updated after merge
