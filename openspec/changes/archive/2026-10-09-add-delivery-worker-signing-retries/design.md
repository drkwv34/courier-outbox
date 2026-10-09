# Design: add-delivery-worker-signing-retries

## Context

See proposal.md for motivation. Day 2 already created `deliveries` (`lease_owner`, `lease_until`, `delivered_at`, CHECK pairing leases and requiring `delivered_at` iff status is `delivered`) and `delivery_attempts` (`status_code` nullable, `error_message` nullable, `duration_ms`, `request_id`). Day 4 insert path writes `pending` rows with `attempt_count=0` and `next_attempt_at=now()`. Constraints: domain stays pure (no `net/http`); store owns every TX; **never hold a TX across outbound HTTP**; secrets and payload bodies are never logged; delivery is at-least-once.

## Goals / Non-Goals

**Goals:**

- Claim in a short TX, POST with no TX open, record in a second short TX fenced by `lease_owner`.
- Production backoff as a domain table; inject a test-only millisecond override from config.
- One outbound `http.Client` constructor; golden + fuzz coverage for v1 HMAC.

**Non-Goals:**

- DLQ HTTP API, delivery-log listing, dial-time SSRF IP blocking, metrics, Redis as the lease source of truth (see proposal out of scope).

## Decisions

- **No new goose migration.** `000004` and `000005` already encode SRS §6 lease columns and the attempts table. Alternative: a no-op migration named for this change. Rejected — forward-only files must change schema.
- **`courier worker` as a separate process.** Compose adds a `worker` service (`command: ["worker"]`) on the `api` image. Alternative: `--worker` flag on `serve`. Rejected as the default so API and worker scale independently; a single binary still offers both subcommands for demo. `serve` does not start workers.
- **Claim SQL** follows `docs/architecture/transactionality.md`: `UPDATE ... WHERE id IN (SELECT ... FOR UPDATE SKIP LOCKED LIMIT $n)` with `status IN ('pending','retrying')`, `next_attempt_at <= now()`, `(lease_until IS NULL OR lease_until < now())`, and `JOIN subscriptions ON enabled = true`. Returning columns include event payload, target URL, static headers, and `signing_secret_enc` so the worker does not open a second read after commit. Alternative: claim ids then SELECT. Rejected to avoid a TOCTOU gap if the subscription is disabled between statements.
- **Record TX:** `INSERT delivery_attempts` always, then `UPDATE deliveries SET ... lease_owner=NULL, lease_until=NULL WHERE id=$1 AND lease_owner=$2`. Zero rows from the UPDATE → `domain.ErrLeaseLost`; the attempt row still stands. Domain `RecordSuccess` / `RecordFailure` compute the next status, `attempt_count`, delay, and `delivered_at`; SQL applies `now() + $delay`.
- **Backoff:** `domain.DefaultBackoff` is `[1s, 5s, 25s, 2m, 10m]`. After failure `n` (1-based `attempt_count`), delay is `schedule[n-1]`; `n >= 6` dead-letters. `COURIER_BACKOFF_MS` (optional int, default unset) replaces every delay with that many milliseconds. Parsed in `internal/config`, injected into the worker; domain never reads env. Documented test-only in `.env.example`.
- **Signing:** `internal/sign` stdlib only. Canonical string `{unix_seconds}.{raw_body}`. `Sign(secret, ts, body) []byte` hex-encodes HMAC-SHA256. `Verify` uses `hmac.Equal` and rejects `|now-ts| > 5m`. Headers set by the worker: `Content-Type: application/json`, `X-Courier-*`, `X-Request-Id`, `User-Agent: courier-outbox/1.0`, then subscription static headers that cannot override reserved names (already rejected at subscription write). Decrypt the secret immediately before `Sign`; zero the plaintext buffer afterwards when practical.
- **HTTP client** (one constructor in `internal/worker`): `Dialer.Timeout=2s`, `TLSHandshakeTimeout=2s`, `Client.Timeout=5s`, `ResponseHeaderTimeout=5s`, `Proxy: nil`, `CheckRedirect` returns `http.ErrUseLastResponse`, max 8 KiB body then `io.Copy(Discard)`. Per-attempt `context.WithTimeout(5s)`.
- **Pool:** `WORKER_CONCURRENCY` (default 4) goroutines; each claims up to `WORKER_CLAIM_LIMIT` (default 8) then processes sequentially so one slow subscriber occupies one goroutine. Empty claim → wait `WORKER_POLL_INTERVAL` (default 500ms). `WORKER_LEASE_TTL` default 30s. `WORKER_ID` default `hostname-pid`. Shutdown: cancel the poll loop, wait in-flight POSTs up to the existing 10s process shutdown budget, then exit.
- **Auth probes:** change `/v1/deliveries` (used as an unrouted 404) to `/v1/__probe` in unit and integration auth tests so Day 6 can mount `/v1/deliveries` without stealing the probe.
- **Interfaces:** worker declares `Claimer` / `Recorder` (or one `Store`) with `ClaimDue` and `RecordAttempt`. `cmd/courier` passes `*store.Postgres`. Envelope and signer are injected. Clock for signing timestamps is injected (`func() time.Time`) so goldens stay fixed.
- **OpenAPI:** no producer HTTP surface change; leave `openapi/openapi.yaml` as-is except if a probe-related note exists (it does not). Worker is CLI/ops, not the public contract.

## Risks / Trade-offs

- **[Risk] Crash after 2xx POST before record → duplicate send.** → Mitigation: document at-least-once; consumers dedupe on `X-Courier-Idempotency-Key`. Do not lengthen the TX.
- **[Risk] Lease shorter than 5s HTTP budget + record.** → Mitigation: default lease 30s ≫ 5s; record is a short UPDATE.
- **[Risk] `COURIER_BACKOFF_MS` used in production.** → Mitigation: README / `.env.example` mark it test-only; empty default is the production table.
- **[Trade-off] Claim skips disabled subscriptions rather than failing the delivery.** Rows stay `pending` until re-enabled; that matches FR-SUB-002 "workers skip" without burning retries.
- **[Trade-off] Postgres leases only.** Redis remains unused for fencing tonight; ADR 0001 still holds for later rate limits.

## Migration Plan

- Schema already migrated; `courier migrate` is a no-op on up-to-date databases.
- Deploy: new binary + Compose `worker` service. Rollback: stop workers; pending/retrying rows remain valid outbox state.
- Do not drop lease columns or `delivery_attempts`.

## Test plan

- **Unit (domain):** table-driven backoff including DLQ at 6; invalid transitions off `delivered`.
- **Unit (sign):** golden vectors (fixed secret/timestamp/body); Verify success; skew > 5 min fails; fuzz sign→verify.
- **Unit (worker client):** httptest 2xx; 500; 3xx not followed; slow handler >5s fails; User-Agent and signature headers present.
- **Unit (config):** worker knobs defaults; `COURIER_BACKOFF_MS` parsed; invalid values fail fast.
- **Integration (tagged, real Postgres, httptest subscriber):** 500 then 200 → delivered, two attempt rows, mock verified signature; slow mock → failure attempt; claim without complete then reclaim after `lease_until`; two concurrent workers, one POST in the lease window.
- **API tests:** unauthorized/authorized probes use `/v1/__probe`.
