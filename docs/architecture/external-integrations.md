# External integrations

Courier talks to three things: **subscriber endpoints** (untrusted, arbitrary URLs), **PostgreSQL**, and **Redis**. Subscribers are the dangerous one.

## Outbound webhook HTTP client (FR-DEL-003)

Build it in exactly one constructor in `internal/worker`. Never use `http.DefaultClient` or `http.Get`.

| Setting | Value | Why |
|---------|-------|-----|
| `net.Dialer.Timeout` | 2s | Slow or blackholed hosts can't pin workers |
| `Transport.TLSHandshakeTimeout` | 2s | Same |
| `Client.Timeout` (total) | **5s** | Hard budget per attempt, including body read |
| `Transport.ResponseHeaderTimeout` | 5s | Defense in depth |
| `Transport.Proxy` | `nil` | Env proxies must not bypass SSRF checks |
| `Transport.MaxIdleConnsPerHost` | small (e.g. 4) | Fairness across subscribers |
| `CheckRedirect` | return `http.ErrUseLastResponse` | **Redirects are not followed.** A 3xx counts as a failed attempt. This removes the redirect-to-internal-IP SSRF class entirely. |
| Response body | read at most **8 KiB**, then drain and close | Protects memory and allows connection reuse |
| `User-Agent` | `courier-outbox/1.0` | Identifiable traffic |

Request headers: `Content-Type: application/json`, `X-Courier-Idempotency-Key`, `X-Courier-Timestamp`, `X-Courier-Signature: v1=<hex>`, `X-Request-Id`, and then the subscription's static headers. Static headers can never override the `X-Courier-*` headers, `Host`, or `Content-Length`.

Always pass a per-attempt `context.WithTimeout` so shutdown cancels in-flight requests.

## SSRF protection (FR-SUB-003, NFR-SEC-002)

The policy is enforced **twice**:

1. **At subscription create and update:** parse the URL. Require scheme `https`, or `http` only when `ALLOW_HTTP_CALLBACKS=true`. Reject userinfo, empty host, and non-default schemes. If the host is an IP literal, check it against the deny-list.
2. **At dial time:** a `net.Dialer.Control` (or custom `DialContext`) checks the **resolved** IP of every connection. This defeats DNS rebinding and hostnames that resolve to private ranges.

Denied when `SSRF_PROTECTION=true`:

- Loopback `127.0.0.0/8`, `::1`
- Private `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `fc00::/7`
- Link-local `169.254.0.0/16` (includes cloud metadata `169.254.169.254`), `fe80::/10`
- CGNAT `100.64.0.0/10`, "this network" `0.0.0.0/8`, unspecified `::`
- Multicast and broadcast
- IPv4-mapped IPv6 forms of all of the above (normalize with `netip.Addr.Unmap()`)

Defaults: `SSRF_PROTECTION=true` and `ALLOW_HTTP_CALLBACKS=false`. Docker Compose sets both permissive values **explicitly** so the mock subscriber on the Compose network works. The deny-list is table data with table-driven unit tests, including IPv6 and mapped forms. A blocked dial is recorded as a failed attempt with `error_message="ssrf: destination blocked"`.

## Retries

| Integration | Retry policy |
|-------------|--------------|
| Subscriber POST | **Only** via the durable schedule (1s → 5s → 25s → 2m → 10m → DLQ). No in-process retry loop, and every try is an attempt row. |
| Postgres query in the request path | None. Return the error, and the client gets 500 (or 503 on readiness). |
| Postgres/Redis in the worker loop | The loop backs off with jitter (e.g. 500ms → 5s cap) and keeps running. |
| Startup connectivity | Bounded wait (e.g. 30s) for Compose ordering, then fail fast. |

Retries always have a cap and jitter. Never retry non-idempotent writes blindly.

## Circuit behaviour and fairness

- There is no circuit-breaker library. The retry schedule already sheds load from broken subscribers.
- Per-subscription outbound rate limit uses Redis (token bucket with TTL keys). If Redis is unavailable, fall back to a conservative in-process limit rather than failing deliveries.
- Should: auto-disable a subscription after 50 consecutive 410 responses (FR-ERR-003). Log at `warn` and surface it in the API.
- The worker pool size and claim batch size are config values. One slow subscriber can occupy at most one worker for 5s per attempt.

## PostgreSQL client

- `pgxpool` is built once in `cmd/courier` from `DATABASE_URL` and passed to `store.NewPostgres`.
- Every query takes the caller's `ctx`. Request handlers inherit the HTTP request context, and the worker uses per-batch deadlines.
- Set pool limits explicitly (`MaxConns`) and set `application_name=courier`.
- Use raw SQL or sqlc (SRS §8.8). SQL lives in `internal/store` (or `internal/store/queries/*.sql` if sqlc is adopted).

## Redis client

- `go-redis/v9` is built once from `REDIS_URL`, with short timeouts (dial 1s, read/write 500ms).
- Every key has a TTL and a prefix: `courier:<purpose>:<id>`.
- Redis being down degrades throughput or fairness. It never blocks enqueue or loses data.

## Mock subscriber

`cmd/mock-subscriber` exists only for Compose and tests. Integration tests use `httptest.Server` instead, so they control status codes, latency, and signature verification per case.
