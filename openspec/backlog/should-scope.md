# Should-scope backlog (proposal-only)

This note records **Should**-priority SRS items that are **not** in the v0.1.0 pin. It is an OpenSpec backlog, not a change to implement.

**Do not implement any item here** without a new OpenSpec change (`openspec new change …`) that is reviewed and applied separately. Day 9 `add-pin-ready-closeout` records this list only.

Sources: archived change proposals under `openspec/changes/archive/` and capability status in `openspec/specs/README.md`.

| ID / name | Summary | Recorded in |
|-----------|---------|-------------|
| FR-SUB-003 dial-time SSRF | Create/update URL policy ships (IP literals, scheme, userinfo). Resolved-IP blocking at dial (`net.Dialer.Control`) is still out of scope in `openspec/specs/subscriptions`. | `openspec/specs/subscriptions/spec.md`; `openspec/changes/archive/2026-10-11-add-openapi-metrics-tests/proposal.md` |
| `/docs` Swagger UI | Optional Should; `/openapi.json` is public. Skipped so no UI screenshot is required. | `openspec/specs/README.md` (`service-ops`); `openspec/changes/archive/2026-10-11-add-openapi-metrics-tests/proposal.md` |
| NFR-PERF-001 | Enqueue p99 ≤ 50ms. Not measured; never a CI job. | `openspec/specs/event-enqueue/spec.md`; `docs/architecture/testing-strategy.md` |
| NFR-PERF-002 | Load ≥ 20 deliveries/s. Worker correctness only in v0.1. | `openspec/specs/README.md` (`delivery`); `openspec/changes/archive/2026-10-09-add-delivery-worker-signing-retries/proposal.md` |
| FR-ERR-003 | Auto-disable a subscription after 50 consecutive HTTP 410 responses. DLQ list/detail/replay ships; auto-disable does not. | `openspec/specs/README.md` (`dead-letter`); `docs/architecture/external-integrations.md` |
| Purge / retention jobs | DLQ and attempt history have no retention sweeper. | `openspec/changes/archive/2026-10-10-add-dlq-replay-logs/proposal.md` |
| Contract tests in CI | spectral / libopenapi against `openapi/openapi.yaml`. OpenAPI is served; this extra CI gate is Should. | `docs/architecture/testing-strategy.md` |
| Compose smoke in CI | `scripts/demo.sh` is a reviewer/local path (NFR-PORT-001). A CI job that runs Compose smoke is “before release”, not a PR gate. | `docs/architecture/testing-strategy.md`; `openspec/specs/local-demo/spec.md` |
| Token-protected `/metrics` | Public Prometheus text with bounded labels. Scrape token / worker scrape port can land later without renaming series. | `openspec/changes/archive/2026-10-11-add-openapi-metrics-tests/proposal.md` |
| Admin HTTP for API keys | v1 key create is CLI-only (`courier keys create`). HTTP key management is not in the pin. | `openspec/changes/archive/2026-09-26-add-persistence-and-api-keys/proposal.md` |

Must-scope capabilities already specified under `openspec/specs/` (service-ops, api-keys, subscriptions, event-enqueue, delivery, dead-letter, local-demo) remain the pin contract.

None of the rows above are implemented by `add-pin-ready-closeout`.
