# delivery Specification

## Purpose

Deliver events to subscribers at least once. Workers claim due deliveries with `FOR UPDATE SKIP LOCKED` under time-bounded leases, sign payloads (v1 HMAC-SHA256), and POST through an isolated HTTP client with strict timeouts and SSRF protection. They record every attempt and schedule retries (1s → 5s → 25s → 2m → 10m) before dead-lettering.

Coverage: FR-DEL-001..007, NFR-REL-001 (no lost deliveries on crash), NFR-SEC-002 (signature golden tests). NFR-PERF-002 (≥ 20 deliveries/s locally) is still outstanding.

## Requirements

### Requirement: Claim due deliveries
The system SHALL claim due deliveries in status `pending` or `retrying` whose `next_attempt_at` is at or before the database clock and whose lease is null or expired, using `FOR UPDATE SKIP LOCKED` with a batch limit. The claim transaction MUST set `lease_owner` and `lease_until` (default 30s) and MUST commit before any outbound HTTP (FR-DEL-001).

#### Scenario: Due pending delivery is claimed
- **WHEN** a pending delivery has `next_attempt_at` in the past and no lease
- **THEN** a worker claim sets `lease_owner` and a `lease_until` 30s ahead
- **AND** the claim transaction is committed before the subscriber POST starts

#### Scenario: Future next_attempt_at is not claimed
- **WHEN** a retrying delivery has `next_attempt_at` in the future
- **THEN** it is not returned by claim

#### Scenario: Unexpired lease is not claimed
- **WHEN** a due delivery has `lease_until` still in the future
- **THEN** another worker does not claim it

### Requirement: Signature scheme
The system SHALL sign each POST with HMAC-SHA256 over `{unix_seconds}.{raw_body_bytes}` using the subscription signing secret. Headers MUST be `X-Courier-Idempotency-Key` (event id), `X-Courier-Timestamp` (unix seconds), and `X-Courier-Signature` `v1=<hex>` (FR-DEL-002, NFR-SEC-002).

#### Scenario: Golden signature
- **WHEN** a fixed secret, timestamp, and body are signed
- **THEN** the signature header is `v1=` plus the expected hex HMAC-SHA256
- **AND** verification with the same inputs succeeds

#### Scenario: Mock verifies signature
- **WHEN** a worker POSTs a claimed event to a mock that verifies the v1 signature
- **THEN** the mock accepts the request
- **AND** the delivery is recorded as delivered

#### Scenario: Timestamp skew rejected
- **WHEN** a verifier is given a timestamp more than 5 minutes from now
- **THEN** verification fails

### Requirement: Isolated HTTP client
The outbound client MUST use a 2s dial timeout, 2s TLS handshake timeout, 5s total request timeout, `User-Agent: courier-outbox/1.0`, MUST NOT follow redirects, and MUST read at most 8 KiB of the response then drain it. A request exceeding 5s MUST count as a failed attempt (FR-DEL-003).

#### Scenario: Slow mock is a failed attempt
- **WHEN** the subscriber takes longer than 5s to respond
- **THEN** the attempt is recorded as a failure
- **AND** the delivery is not marked delivered

#### Scenario: Redirect is not followed
- **WHEN** the subscriber returns a 3xx redirect
- **THEN** the client does not follow it
- **AND** the attempt is recorded as a failure

### Requirement: Successful delivery
A subscriber HTTP 2xx response SHALL mark the delivery `delivered` with `delivered_at` set to the database clock and MUST release the lease (FR-DEL-004).

#### Scenario: Two-hundred succeeds
- **WHEN** the subscriber returns 200
- **THEN** delivery status is `delivered`
- **AND** `delivered_at` is set
- **AND** `lease_owner` and `lease_until` are null

### Requirement: Retry schedule
After a non-2xx, timeout, or network failure the system SHALL increment `attempt_count` and set status `retrying` with delay 1→1s, 2→5s, 3→25s, 4→2m, 5→10m. Attempt count 6 or greater SHALL set status `dead_lettered` with no further automatic retry (FR-DEL-005).

#### Scenario: First failure retries in one second
- **WHEN** the first POST fails
- **THEN** `attempt_count` is 1
- **AND** status is `retrying`
- **AND** `next_attempt_at` is now plus 1s (or the test-only override)

#### Scenario: Exhaustion dead-letters
- **WHEN** a delivery fails on the attempt that brings `attempt_count` to 6
- **THEN** status is `dead_lettered`
- **AND** no further automatic retry is scheduled

#### Scenario: Five-hundred then two-hundred delivers
- **WHEN** a mock returns 500 then 200 and the backoff override is short
- **THEN** the delivery ends as `delivered`
- **AND** two `delivery_attempts` rows exist

### Requirement: Test-only backoff override
When `COURIER_BACKOFF_MS` is set, every retry delay MUST use that many milliseconds instead of the production schedule. The override MUST be documented as test-only (FR-DEL-005).

#### Scenario: Override shortens delays
- **WHEN** `COURIER_BACKOFF_MS` is 10
- **THEN** a failed attempt schedules `next_attempt_at` 10ms ahead
- **AND** `.env.example` labels the variable as test-only

### Requirement: Attempt log
Each HTTP try SHALL insert one `delivery_attempts` row with `delivery_id`, nullable `status_code`, nullable `error_message`, `duration_ms`, `request_id`, and `created_at`. The lease MUST be released when the attempt is recorded (FR-DEL-006).

#### Scenario: Failed attempt is logged
- **WHEN** a subscriber returns 500
- **THEN** a `delivery_attempts` row exists with `status_code` 500
- **AND** `duration_ms` is non-negative
- **AND** the lease is released

#### Scenario: Timeout attempt has error_message
- **WHEN** the subscriber exceeds the 5s budget
- **THEN** the attempt row has a non-empty `error_message`
- **AND** `status_code` may be null

### Requirement: Lease expiry reclaim
A delivery whose worker does not complete before `lease_until` SHALL become claimable by another worker. Delivery is at-least-once: a crash after POST and before record MAY duplicate a send (FR-DEL-007, NFR-REL-001).

#### Scenario: Abandoned claim is reclaimed
- **WHEN** a worker claims a delivery and does not complete, and the clock is past `lease_until`
- **THEN** another worker claims that delivery
- **AND** the pending delivery is not lost

### Requirement: Concurrent workers do not double-claim
Two workers MUST NOT both hold an unexpired lease on the same delivery. `SKIP LOCKED` MUST ensure at most one in-flight POST per delivery during a lease window.

#### Scenario: Two workers one lease window
- **WHEN** two workers poll while one due delivery exists
- **THEN** only one worker POSTs during that lease window
- **AND** the other claim returns no row for that id

### Requirement: Disabled subscriptions are skipped
Workers MUST NOT POST to a disabled subscription. Claim MUST skip deliveries whose subscription is `enabled=false`.

#### Scenario: Disabled subscription is not posted
- **WHEN** the only due delivery belongs to a disabled subscription
- **THEN** claim returns no work for it
- **AND** no HTTP POST is made

### Requirement: Delivery logs redact secrets
Structured logs for each attempt MUST include `delivery_id`, `event_id`, and `attempt`. They MUST NOT include signing secrets, API keys, or payload bodies (NFR-OBS-001).

#### Scenario: Outcome log has ids not secrets
- **WHEN** a delivery attempt completes
- **THEN** the log line includes `delivery_id`, `event_id`, and `attempt`
- **AND** it does not include the signing secret or payload body

### Requirement: Worker command and compose
The courier binary SHALL provide a `worker` subcommand that runs the claim/send/record loop. Docker Compose MUST start a worker using the same image as the API.

#### Scenario: Worker subcommand exists
- **WHEN** an operator runs `courier worker`
- **THEN** the process claims and delivers due rows until shutdown

### Requirement: Consumer contract documentation
The README MUST document the signature scheme and consumer responsibilities: verify HMAC, reject timestamp skew greater than 5 minutes, treat delivery as at-least-once and dedupe on `X-Courier-Idempotency-Key`, return 2xx only after durable accept, and respond quickly (FR-DOC-001).

#### Scenario: README states at-least-once
- **WHEN** a reviewer reads the README worker and consumer sections
- **THEN** the signature headers and signed string are described
- **AND** delivery is stated as at-least-once, not exactly-once
