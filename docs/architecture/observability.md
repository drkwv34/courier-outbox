# Logging and observability

## Logging (SRS §8.5, NFR-OBS-001)

- Use `log/slog` with the JSON handler to stdout. The handler is built once in `cmd/courier` and **injected** as `*slog.Logger`. Library packages never call `slog.Default()` or `log.Printf`.
- Use typed attributes (`slog.String`, `slog.Int64`, `slog.Any("err", err)`). Never interpolate values into the message.
- Messages are short, lowercase, and constant (`"delivery attempt failed"`). Variable data goes in attributes, which keeps logs greppable.
- Enrich loggers with context through `logger.With(...)` at the start of a request or job, not by repeating fields on every call.

### Canonical keys

Use exactly these names. Log assertions and dashboards depend on them.

| Key | Meaning |
|-----|---------|
| `request_id` | chi `middleware.RequestID` value, also stored on attempts |
| `api_key_id` | Owning key **id** (never the key or its hash) |
| `event_id`, `delivery_id`, `subscription_id` | Resource ids |
| `attempt` | `attempt_count` for this try (1-based) |
| `status_code` | Subscriber HTTP status (omit if none) |
| `duration_ms` | Attempt or request duration |
| `outcome` | `delivered`, `retrying`, `dead_lettered` |
| `worker_id` | Lease owner id |
| `err` | Wrapped error |

### Levels

| Level | Use for |
|-------|---------|
| `debug` | Per-row detail useful when developing (claimed batch ids). Off in Compose by default. |
| `info` | Lifecycle (startup, shutdown, listening), each delivery outcome, DLQ transitions, replays |
| `warn` | Recoverable anomalies: lease lost, subscriber slow, config defaults applied in a risky way |
| `error` | Courier failed to do its job: DB unavailable, unexpected 500, panic recovered |

Subscriber failures are **info** with `outcome=retrying`, not `error`, because they are expected traffic.

### Redaction (never log)

- Raw API keys, key hashes, the `Authorization` header
- Signing secrets (plain or encrypted), `COURIER_ENCRYPTION_KEY`, `DATABASE_URL` / `REDIS_URL` with credentials
- Event payload bodies. Log `payload_bytes` instead.
- Full subscriber response bodies. Log at most a truncated, sanitized snippet at debug.
- Target URL query strings (they often carry tokens). Log scheme + host + path.

When adding a struct that holds secrets, implement `LogValue() slog.Value` to redact it.

## Correlation

- HTTP: `middleware.RequestID` generates or propagates `X-Request-Id`. Echo it in responses and attach it to the request logger.
- Worker: each attempt gets its own `request_id`, which is stored in `delivery_attempts.request_id` and sent to subscribers as `X-Request-Id`, so both sides can correlate.
- Access logs are one line per request: method, route pattern (not raw path), status, duration, request_id, api_key_id.

## Metrics (FR-API-003, Should)

Prometheus text on `GET /metrics`:

- `courier_deliveries_total{status="delivered|retrying|dead_lettered"}` (counter)
- `courier_attempt_duration_seconds` (histogram)
- `courier_dlq_total` (counter)
- Later, if useful: `courier_http_requests_total{route,status}` and `courier_claim_batch_size`

Label values must be bounded. Never use ids or URLs as labels.

## Health

- `GET /healthz` is liveness only and never touches dependencies.
- `GET /readyz` pings Postgres and Redis with short timeouts (≤ 1s each) and returns 503 plus the `unavailable` code if either fails.
