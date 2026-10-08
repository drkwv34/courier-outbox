# event-enqueue Specification

## Purpose

Accept events from producers and persist, in a single database transaction, the event plus one pending delivery per matching enabled subscription. Idempotency keys scoped to the API key guarantee that concurrent duplicate enqueues produce exactly one event.

Coverage: FR-EVT-001 (enqueue, 201/200 replay, 413), FR-EVT-002 (UNIQUE idempotency under concurrency), FR-EVT-003 (get event + delivery summaries), NFR-REL-002. NFR-PERF-001 (enqueue p99) is deferred.

## Requirements

### Requirement: Enqueue event
The system SHALL accept `POST /v1/events` from an authenticated producer. The JSON body MUST include `type`, object `payload`, and `idempotency_key`. Optional `occurred_at` is RFC 3339; when omitted the system MUST store the current time (FR-EVT-001).

#### Scenario: Event is created
- **WHEN** a client POSTs a valid body with a new idempotency key
- **THEN** the response status is 201
- **AND** the body is `{ "event_id": "<uuid>", "delivery_ids": [ ... ] }`
- **AND** an `events` row exists for that `api_key_id` and idempotency key

#### Scenario: Occurred_at defaults to now
- **WHEN** a client omits `occurred_at`
- **THEN** the stored event has `occurred_at` set by the server
- **AND** the response status is 201

#### Scenario: Occurred_at is persisted
- **WHEN** a client POSTs a valid RFC 3339 `occurred_at`
- **THEN** the stored event's `occurred_at` equals that timestamp

### Requirement: Pending deliveries for matching subscriptions
In the same transaction as the event insert, the system SHALL insert one delivery per enabled subscription owned by the same API key whose `event_types` match the event `type`. `"*"` MUST match every type. Disabled or non-matching subscriptions MUST NOT receive a delivery. Each new delivery MUST have `status=pending`, `attempt_count=0`, and `next_attempt_at` equal to the database clock (FR-EVT-001).

#### Scenario: Matching enabled subscription gets a pending delivery
- **WHEN** the API key owns an enabled subscription with `event_types` that include the event type or `"*"`
- **THEN** `delivery_ids` contains one id for that subscription
- **AND** that delivery has status `pending` and `attempt_count` 0

#### Scenario: Disabled subscription is skipped
- **WHEN** the only matching subscription is disabled
- **THEN** the event is created
- **AND** `delivery_ids` is an empty array

#### Scenario: Non-matching type is skipped
- **WHEN** an enabled subscription lists only a different event type
- **THEN** no delivery is created for that subscription

#### Scenario: Wildcard matches all types
- **WHEN** an enabled subscription has `event_types` of `["*"]`
- **THEN** a pending delivery is created for any valid event type

### Requirement: Idempotent replay
When the authenticated API key reuses an `idempotency_key`, the system SHALL NOT insert a second event. It MUST return the original `event_id` and its `delivery_ids` with status 200 and header `Idempotent-Replay: true`. Replay is not an error (FR-EVT-001, FR-EVT-002).

#### Scenario: Sequential duplicate returns replay
- **WHEN** a client POSTs the same `idempotency_key` twice for the same API key
- **THEN** the first response is 201
- **AND** the second response is 200 with header `Idempotent-Replay: true`
- **AND** both bodies share the same `event_id` and `delivery_ids`
- **AND** only one `events` row exists for that key and idempotency key

### Requirement: Concurrent duplicate enqueue
`UNIQUE (api_key_id, idempotency_key)` MUST guarantee one event per key pair. Concurrent POSTs with the same pair MUST yield exactly one event and one delivery set; losers MUST receive the 200 replay, never 500 and never a second event (FR-EVT-002, NFR-REL-002).

#### Scenario: Parallel duplicate enqueue
- **WHEN** twenty concurrent requests POST the same `idempotency_key` for one API key
- **THEN** exactly one `events` row exists for that pair
- **AND** each response is 201 or 200 with the same `event_id`
- **AND** no response is 500
- **AND** the deliveries for that event equal the matching-subscription set of a single enqueue

### Requirement: Enqueue validation
The system SHALL reject invalid enqueue bodies with 400 and `code` `validation_failed`. `type` MUST be 1–128 characters matching `^[a-z0-9]+([._-][a-z0-9]+)*$`. `payload` MUST be a JSON object. `idempotency_key` MUST be 1–128 characters. `occurred_at`, when present, MUST be RFC 3339 (FR-EVT-001).

#### Scenario: Missing type
- **WHEN** a client POSTs without `type` or with an empty `type`
- **THEN** the response status is 400 and `code` is `validation_failed`

#### Scenario: Invalid type token
- **WHEN** a client POSTs `type` that does not match the allowed pattern
- **THEN** the response status is 400 and `code` is `validation_failed`

#### Scenario: Non-object payload
- **WHEN** a client POSTs `payload` that is not a JSON object
- **THEN** the response status is 400 and `code` is `validation_failed`

#### Scenario: Idempotency key length
- **WHEN** a client POSTs `idempotency_key` empty or longer than 128 characters
- **THEN** the response status is 400 and `code` is `validation_failed`

#### Scenario: Bad occurred_at
- **WHEN** a client POSTs `occurred_at` that is not RFC 3339
- **THEN** the response status is 400 and `code` is `validation_failed`

#### Scenario: Unknown field rejected
- **WHEN** a client POSTs a body containing a field other than `type`, `payload`, `idempotency_key`, and `occurred_at`
- **THEN** the response status is 400 and `code` is `validation_failed`

### Requirement: Payload size limit
The system SHALL reject an enqueue request whose body exceeds 256 KiB with 413 and `code` `payload_too_large` before JSON decoding (FR-EVT-001).

#### Scenario: Oversized body
- **WHEN** a client POSTs a body larger than 256 KiB
- **THEN** the response status is 413 and `code` is `payload_too_large`

### Requirement: Get event
`GET /v1/events/{id}` SHALL return the event owned by the calling API key, including payload, plus delivery summaries with `id`, `subscription_id`, `status`, `attempt_count`, and `next_attempt_at`. A malformed id MUST be 400, not 404 (FR-EVT-003).

#### Scenario: Owner gets event and deliveries
- **WHEN** the owning key GETs a previously enqueued event id
- **THEN** the response status is 200
- **AND** the body includes the event payload and delivery summaries

#### Scenario: Get unknown id
- **WHEN** a client GETs a well-formed UUID that does not exist for this key
- **THEN** the response status is 404 and `code` is `not_found`

#### Scenario: Get malformed id
- **WHEN** a client GETs `/v1/events/not-a-uuid`
- **THEN** the response status is 400 and `code` is `validation_failed`

### Requirement: Per-API-key event isolation
Every event read and enqueue SHALL be scoped to the authenticated `api_key_id`. An event owned by another key MUST be indistinguishable from a missing id: `404` with `code` `not_found`. The payload MUST NOT be returned to any key other than the owner (FR-EVT-003, FR-AUTH-003).

#### Scenario: Foreign get is not found
- **WHEN** key A enqueues an event and key B GETs that event id
- **THEN** the response status is 404 and `code` is `not_found`
- **AND** the response body does not include the payload

#### Scenario: Idempotency keys are per API key
- **WHEN** key A and key B POST the same `idempotency_key` with valid bodies
- **THEN** each receives 201 with a distinct `event_id`

### Requirement: Enqueue requires authentication
`POST /v1/events` and `GET /v1/events/{id}` SHALL use the existing Bearer API-key auth. Missing, invalid, or revoked keys MUST return 401 `unauthorized` identical to other `/v1` routes.

#### Scenario: Missing key is unauthorized
- **WHEN** a client POSTs `/v1/events` without `Authorization`
- **THEN** the response status is 401 and `code` is `unauthorized`
