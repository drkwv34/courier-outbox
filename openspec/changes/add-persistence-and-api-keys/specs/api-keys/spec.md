# api-keys: spec delta for add-persistence-and-api-keys

## ADDED Requirements

### Requirement: Create API key via CLI
The system SHALL provide `courier keys create --name <name>`. The command SHALL generate a random API key of the form `co_<prefix>_<secret>` and persist only its `prefix` and a hash of the full key. It SHALL print the raw key exactly once (FR-AUTH-001, NFR-SEC-001).

#### Scenario: Key is created and shown once
- **WHEN** an operator runs `courier keys create --name ci`
- **THEN** stdout contains the raw key, its id, and its prefix
- **AND** the `api_keys` row stores the prefix and a 32-byte hash, and does not store the raw key

#### Scenario: Name is required
- **WHEN** an operator runs `courier keys create` without `--name`
- **THEN** the command exits non-zero and creates no row

### Requirement: API keys are hashed at rest with a server pepper
The system SHALL store `HMAC-SHA256(COURIER_API_KEY_PEPPER, raw_key)` and SHALL compare hashes in constant time. It SHALL refuse to start when the pepper is shorter than 32 bytes (NFR-SEC-001).

#### Scenario: Database contents alone cannot verify a key
- **WHEN** the same raw key is hashed with two different peppers
- **THEN** the hashes differ

#### Scenario: Short pepper rejected
- **WHEN** `COURIER_API_KEY_PEPPER` is shorter than 32 bytes
- **THEN** startup fails with a configuration error naming the variable

### Requirement: Bearer authentication on the v1 API
Every request under `/v1` SHALL present `Authorization: Bearer <api_key>`. A request with a missing header, a non-Bearer scheme, a malformed key, an unknown prefix, a wrong secret, or a revoked key SHALL receive `401` with `{"code":"unauthorized"}` and a `WWW-Authenticate: Bearer` header. All six cases SHALL be indistinguishable to the client (FR-AUTH-002).

#### Scenario: Missing key
- **WHEN** a client sends `GET /v1/anything` without an `Authorization` header
- **THEN** the response status is 401 and `code` is `unauthorized`

#### Scenario: Wrong secret for a known prefix
- **WHEN** a client presents a key whose prefix exists but whose secret differs
- **THEN** the response status is 401 and `code` is `unauthorized`

#### Scenario: Revoked key
- **WHEN** a client presents a key whose `revoked_at` is set
- **THEN** the response status is 401 and `code` is `unauthorized`

#### Scenario: Valid key passes through
- **WHEN** a client presents a valid, unrevoked key
- **THEN** the request reaches the `/v1` router with the authenticated `api_key_id` in the request context
- **AND** an unrouted `/v1` path returns 404 `not_found`, not 401

### Requirement: Tenant ownership in the schema
Every tenant-owned table (`subscriptions`, `events`) SHALL reference `api_keys(id)` through a non-null `api_key_id` foreign key, and events SHALL be unique per `(api_key_id, idempotency_key)`. This is the storage foundation for FR-AUTH-003 and FR-EVT-002; query-level isolation is enforced by the changes that add those queries.

#### Scenario: Schema enforces ownership
- **WHEN** migrations have been applied
- **THEN** `subscriptions.api_key_id` and `events.api_key_id` are `NOT NULL` foreign keys to `api_keys`
- **AND** a unique constraint exists on `events (api_key_id, idempotency_key)`
