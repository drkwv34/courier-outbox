# Proposal

- **Id:** `restart-backoff-on-replay`
- **Status:** approved
- **SRS requirements:** FR-DLQ-002, FR-DEL-005

## Why

Replay of a `dead_lettered` delivery currently keeps `attempt_count` at 6. The next failed POST increments to 7 and dead-letters immediately, so a still-failing subscriber never gets the 1s → 5s → 25s → 2m → 10m schedule again. Attempt history is already append-only; operators need a fresh retry budget, not a seventh instant failure.

## What Changes

- `POST /v1/deliveries/{id}/replay` resets `attempt_count` to 0 while leaving existing `delivery_attempts` rows in place.
- After replay, the next failures follow the full backoff table until attempt 6 dead-letters again.
- OpenAPI and README describe the restarted budget. **BREAKING** for clients that assumed replay left `attempt_count` unchanged.

## Capabilities

### New Capabilities

- None.

### Modified Capabilities

- `dead-letter`: replay restarts the retry budget (`attempt_count=0`) and keeps attempt history.

## Impact

- **Capabilities:** `dead-letter`.
- **Endpoints / CLI:** `POST /v1/deliveries/{id}/replay` 202 body has `attempt_count: 0`.
- **Schema / migrations:** none; `attempt_count` is already an integer column.
- **Config:** none.
- **Packages:** `internal/domain` (`Replay`), `internal/store` (UPDATE), `internal/api` tests, OpenAPI, README, domain-model docs.
- **Dependencies:** none.

## Out of scope

- Changing the 6-attempt production schedule or `COURIER_BACKOFF_MS`.
- Deleting or rewriting attempt rows.
- Worker metrics, SSRF hostnames, Swagger UI, load script.
