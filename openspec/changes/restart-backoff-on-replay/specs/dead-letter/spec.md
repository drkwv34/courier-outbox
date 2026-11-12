# dead-letter — spec delta for restart-backoff-on-replay

## MODIFIED Requirements

### Requirement: Replay dead-lettered delivery
`POST /v1/deliveries/{id}/replay` SHALL return 202 only when the owned delivery is `dead_lettered`. Replay MUST set status `pending`, set `next_attempt_at` to the database clock, clear lease fields, and reset `attempt_count` to 0 so the full backoff schedule runs again. It MUST NOT delete attempt rows. A non-dead-letter MUST be 409 with `code` `invalid_transition` (FR-DLQ-002).

#### Scenario: Replay returns pending
- **WHEN** an owned `dead_lettered` delivery is POSTed to `/v1/deliveries/{id}/replay`
- **THEN** the response is 202
- **AND** status is `pending`
- **AND** `attempt_count` is 0
- **AND** existing attempt rows remain
- **AND** `lease_owner` and `lease_until` are null

#### Scenario: Replay of pending is conflict
- **WHEN** an owned `pending` or `delivered` delivery is POSTed to replay
- **THEN** the response is 409 with `code` `invalid_transition`
- **AND** status is unchanged

#### Scenario: Replay missing is not found
- **WHEN** the id is missing or owned by another API key
- **THEN** the response is 404 with `code` `not_found`

#### Scenario: Replay of a still-failing target uses the full schedule
- **WHEN** a replayed delivery's subscriber keeps returning non-2xx
- **THEN** the worker records a new append-only attempt for each try
- **AND** backoff follows 1s → 5s → 25s → 2m → 10m (or the test-only override)
- **AND** the delivery is `dead_lettered` again only after six failures in that cycle
