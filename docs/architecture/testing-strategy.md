# Testing strategy

Test the hard edges thoroughly: idempotency, the retry schedule, leases, signatures, and SSRF. Don't chase line coverage on wiring.

## Layers

| Layer | Scope | Tooling | Runs |
|-------|-------|---------|------|
| **Unit** | `domain`, `sign`, `config`, SSRF policy, backoff, handlers with fake stores | `testing`, `httptest`, hand-written fakes | `go test -race ./...` on every PR |
| **Fuzz** | `sign` (and URL parsing once added) | native `go test -fuzz` | Seed corpus runs as part of unit tests. Long fuzzing is manual. |
| **Integration** | `store` against real Postgres/Redis, worker against `httptest.Server` | testcontainers-go ([ADR 0005](adr/0005-testcontainers-integration-tests.md)), build tag `integration` | `go test -race -tags=integration ./...` in a CI job with Docker |
| **Contract** | OpenAPI document validity, handler/OpenAPI agreement | spectral or libopenapi (Should) | CI once the OpenAPI surface exists |
| **Smoke / E2E** | `docker compose up`, enqueue, mock receives a signed POST within 5s (NFR-PORT-001) | `scripts/smoke.sh` (curl + jq) | CI job before release, and locally |
| **Load** | ≥ 20 deliveries/s (NFR-PERF-002), enqueue p99 ≤ 50ms (NFR-PERF-001) | `scripts/` load script, Go benchmarks | Manual, results noted in README. Never in CI. |

## Mandatory hard-edge tests (from SRS §7 / §12)

- Backoff: table test of `attempt_count` → delay, including DLQ at 6.
- Signature: golden vectors (fixed secret, timestamp, body → expected hex), plus a fuzz round-trip of sign then verify.
- SSRF: allow/deny table covering IPv4, IPv6, mapped IPv6, metadata IP, and DNS names resolving to private IPs (via an injected resolver).
- Idempotent enqueue under concurrency: N goroutines, same key, exactly one event, and N−1 replays.
- Lease expiry: claim, abandon, reclaim after `lease_until`, and the second worker delivers.
- Crash safety: no pending delivery is lost when a worker stops mid-batch.
- DLQ replay: force DLQ, replay, delivered, and old attempts still present.
- Auth matrix: no key, bad key, revoked key, and another tenant's resource (404).
- Secret non-exposure: GET after create never contains the secret.

## Conventions

- **Table-driven** tests with named cases, and `t.Parallel()` where there's no shared state.
- **Fakes over mocks.** Hand-write small fakes that implement the consumer-declared interface. No mocking framework unless one is justified.
- **No `time.Sleep` for synchronization.** Use an injected clock, channels, or poll with a deadline (`require.Eventually`-style helper).
- **Subscribers in tests are `httptest.Server`**, which gives per-test control over status, latency, and signature checks.
- Integration tests get an isolated database (fresh container or per-test schema) and apply real migrations.
- Golden files live in `testdata/`.
- Assert on `errors.Is`, HTTP status, and the `code` field, not on message strings.
- The test name describes the behaviour: `TestEnqueue_DuplicateKeyReplays`.

## CI gates (target, SRS §7)

lint → unit tests (race) → integration tests (race, tagged) → build binary → docker image build → compose config validation.

CI runs lint, unit tests (race), integration tests (race, tagged; testcontainers starts Postgres 16 and Redis 7), build, `docker compose config`, and the image build.
