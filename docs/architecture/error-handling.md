# Error handling

SRS §8.3 says errors are sentinel + wrapped and are mapped to HTTP **only in the api layer**.

## Kinds of errors

| Kind | Where defined | Example |
|------|---------------|---------|
| **Domain sentinels** | `internal/domain/errors.go` | `ErrNotFound`, `ErrConflict`, `ErrValidation`, `ErrInvalidTransition`, `ErrLeaseLost` |
| **Typed domain errors** (need data) | `internal/domain` | `*ValidationError{Field, Reason}` whose `Unwrap()` returns `ErrValidation` |
| **Config errors** | `internal/config` | `ErrInvalid`, joined so all problems print at once |
| **Infrastructure errors** | returned by pgx, go-redis, net/http | never escape `store` / `worker` untranslated when they carry meaning |

A subscriber answering 500 is **not a Go error**. It is an attempt `Outcome` (data) that drives the state machine. Reserve `error` for "Courier could not do its job".

## Rules

1. **Wrap with context on the way up.** Write `fmt.Errorf("store: insert event: %w", err)`. The message says *what was being attempted*, in lowercase, with no trailing punctuation and no "failed to".
2. **Compare with `errors.Is` / `errors.As`.** Never match on `err.Error()` strings.
3. **Translate at the adapter boundary.** `store` maps `pgx.ErrNoRows` to `domain.ErrNotFound` and unique violation `23505` to `domain.ErrConflict` (or a more specific sentinel such as `ErrIdempotencyKeyReused`). Callers never import pgx to inspect errors.
4. **Handle or return, not both.** Log an error once, at the boundary that handles it (HTTP error mapper, worker loop). Intermediate layers wrap and return.
5. **Only the api layer knows HTTP.** No `http.Status*` outside `internal/api` and `cmd/`.
6. **Never leak internals.** For 5xx the client gets the generic message `"internal error"`. The full wrapped error goes to the log with `request_id`.
7. **Panics are for programmer bugs** (impossible states, nil deps at construction). `middleware.Recoverer` turns a handler panic into a 500. The worker loop recovers per job, logs, and continues.
8. **Every error path is testable.** Table tests assert `errors.Is(err, domain.ErrX)`.

## HTTP mapping (single function in `internal/api`)

The error envelope is `{"code": "...", "message": "..."}` (FR-ERR-001). Codes are stable snake_case and are part of the public contract, so they must be listed in OpenAPI.

| Condition | Status | `code` |
|-----------|--------|--------|
| `errors.Is(err, domain.ErrValidation)` / malformed JSON | 400 | `validation_failed` |
| Missing, invalid, or revoked API key | 401 | `unauthorized` |
| Resource missing **or owned by another key** | 404 | `not_found` |
| `domain.ErrConflict` | 409 | `conflict` |
| Body over 256 KiB (`http.MaxBytesError`) | 413 | `payload_too_large` |
| Rate limited | 429 | `rate_limited` |
| Dependency down during readiness | 503 | `unavailable` |
| Anything else | 500 | `internal` |

Validation errors may include field detail in `message` (e.g. `"idempotency_key: must be 1-128 characters"`). They never echo secrets or raw payloads.

An idempotent replay is **not** an error. The store returns `(result, replayed bool, err)`, and the handler sets `200` plus `Idempotent-Replay: true`.

## Worker error handling

- Classify each attempt as `Success` or `Failure{StatusCode?, Err}`. Timeouts, DNS errors, TLS errors, SSRF blocks, redirects, and non-2xx responses are all failures recorded on the attempt row (`error_message` truncated to a sane length).
- Errors talking to Postgres or Redis abort the current batch. The loop logs at `error`, backs off with jitter, and keeps running. Never crash the process for a transient dependency error.
- `domain.ErrLeaseLost` (fenced update affected 0 rows) means another worker owns the row. Log at `warn` and drop the result. Do not retry.
