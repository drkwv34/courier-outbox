# dead-letter — spec delta for add-dlq-replay-logs

## ADDED Requirements

### Requirement: List deliveries
The system SHALL list deliveries owned by the authenticated API key at `GET /v1/deliveries`. Query filters MUST include `status` (including `dead_lettered`), `subscription_id`, event `type`, `created_after`, and `created_before`. Results MUST paginate with `limit` and `offset` (FR-DLQ-001, FR-ERR-001).

#### Scenario: Filter dead letters
- **WHEN** an operator GETs `/v1/deliveries?status=dead_lettered` with a valid API key
- **THEN** the response is 200
- **AND** every item has `status` `dead_lettered`
- **AND** deliveries owned by another API key are omitted

#### Scenario: Filter by subscription, type, and time
- **WHEN** the query includes `subscription_id`, `type`, and a created-at range
- **THEN** only matching owned deliveries are returned

#### Scenario: Offset pagination
- **WHEN** more owned deliveries exist than `limit`
- **THEN** the response includes `limit`, `offset`, and `total`
- **AND** a later `offset` returns the next page without duplicating the first page's ids

#### Scenario: Invalid filter
- **WHEN** `status` is not a known delivery status, or `subscription_id` is not a UUID, or a time bound is not RFC 3339
- **THEN** the response is 400 with `code` `validation_failed`

### Requirement: Get delivery with attempt history
`GET /v1/deliveries/{id}` SHALL return the owned delivery and its append-only attempts oldest to newest, each with `status_code`, `error_message`, `duration_ms`, `request_id`, and `created_at`. The response MUST NOT include signing secrets or the event payload. A malformed id MUST be 400; a missing or foreign id MUST be 404 `not_found` (FR-DEL-006 visibility, FR-ERR-001).

#### Scenario: Detail includes attempts
- **WHEN** an owned delivery has two attempt rows
- **THEN** GET returns 200 with those attempts in created_at order
- **AND** the body has no signing secret and no event payload

#### Scenario: Missing delivery is not found
- **WHEN** the id is a well-formed UUID that does not exist for this API key
- **THEN** the response is 404 with `code` `not_found`

#### Scenario: Malformed id is validation failed
- **WHEN** the path id is not a UUID
- **THEN** the response is 400 with `code` `validation_failed`

### Requirement: Replay dead-lettered delivery
`POST /v1/deliveries/{id}/replay` SHALL return 202 only when the owned delivery is `dead_lettered`. Replay MUST set status `pending`, set `next_attempt_at` to the database clock, and clear lease fields. It MUST NOT reset `attempt_count` or delete attempt rows. A non-dead-letter MUST be 409 with `code` `invalid_transition` (FR-DLQ-002).

#### Scenario: Replay returns pending
- **WHEN** an owned `dead_lettered` delivery is POSTed to `/v1/deliveries/{id}/replay`
- **THEN** the response is 202
- **AND** status is `pending`
- **AND** `attempt_count` is unchanged
- **AND** existing attempt rows remain
- **AND** `lease_owner` and `lease_until` are null

#### Scenario: Replay of pending is conflict
- **WHEN** an owned `pending` or `delivered` delivery is POSTed to replay
- **THEN** the response is 409 with `code` `invalid_transition`
- **AND** status is unchanged

#### Scenario: Replay missing is not found
- **WHEN** the id is missing or owned by another API key
- **THEN** the response is 404 with `code` `not_found`

### Requirement: Delivery routes require authentication
`GET /v1/deliveries`, `GET /v1/deliveries/{id}`, and `POST /v1/deliveries/{id}/replay` SHALL use Bearer API-key auth. Missing, invalid, or revoked keys MUST return 401 `unauthorized` identical to other `/v1` routes.

#### Scenario: Missing key is unauthorized
- **WHEN** any of the three delivery routes is called without a valid API key
- **THEN** the response is 401 with `code` `unauthorized`

## REMOVED Requirements

### Requirement: Dead-letter capability pending implementation
**Reason:** Replaced by list, detail, and replay requirements in this change.
**Migration:** Use `GET /v1/deliveries`, `GET /v1/deliveries/{id}`, and `POST /v1/deliveries/{id}/replay`.
