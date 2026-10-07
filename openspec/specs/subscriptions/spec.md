# subscriptions Specification

## Purpose

Let producers register webhook endpoints (target URL, event-type filters, enabled flag, static headers) and hold a per-subscription signing secret. The secret is returned once and stored encrypted at rest. Target URLs pass scheme validation and an SSRF policy.

Coverage: FR-SUB-001 (create + secret once), FR-SUB-002 (list/get/update/disable/rotate), FR-SUB-003 (URL validation + SSRF guard), FR-ERR-002 (never re-expose secrets).

## Requirements

### Requirement: Create subscription
The system SHALL accept `POST /v1/subscriptions` from an authenticated producer and persist a subscription owned by that API key. The JSON body MUST include `target_url`. `event_types` defaults to `["*"]`, `headers` to `{}`, `description` to `""`, and `enabled` to true (FR-SUB-001).

#### Scenario: Subscription is created
- **WHEN** a client POSTs a valid `target_url` with a valid API key
- **THEN** the response status is 201
- **AND** the body includes `id`, `target_url`, `event_types`, `headers`, `enabled`, `description`, `created_at`, `updated_at`, and `signing_secret`
- **AND** a `subscriptions` row exists for that `api_key_id`

#### Scenario: Missing target URL
- **WHEN** a client POSTs `{}` or omits `target_url`
- **THEN** the response status is 400 and `code` is `validation_failed`

#### Scenario: Wildcard must stand alone
- **WHEN** a client POSTs `event_types` that includes `"*"` plus another type
- **THEN** the response status is 400 and `code` is `validation_failed`

#### Scenario: Empty event_types rejected
- **WHEN** a client POSTs `"event_types": []`
- **THEN** the response status is 400 and `code` is `validation_failed`

### Requirement: Signing secret shown once
On create, and on update with `rotate_secret` true, the system SHALL generate 32 cryptographically random bytes, return them as standard base64 in `signing_secret`, and SHALL NOT persist the plaintext. GET, list, disable, and update-without-rotate MUST omit `signing_secret` (FR-SUB-001, FR-ERR-002).

#### Scenario: Secret present only on create
- **WHEN** a client creates a subscription and then GETs it by id
- **THEN** the create body contains `signing_secret`
- **AND** the GET body has no `signing_secret` field

#### Scenario: Secret present only on rotate
- **WHEN** a client PATCHes `{"rotate_secret": true}`
- **THEN** the response includes a new `signing_secret` different from the create value
- **AND** a subsequent GET still omits `signing_secret`

### Requirement: Signing secret encrypted at rest
The system SHALL store the signing secret as AES-256-GCM ciphertext (nonce concatenated with ciphertext) keyed by `COURIER_ENCRYPTION_KEY`. Startup MUST fail when that key is missing or does not decode to 32 bytes.

#### Scenario: Ciphertext is not plaintext
- **WHEN** a subscription is created
- **THEN** `signing_secret_enc` is not the raw secret bytes and is not the base64 secret string

#### Scenario: Missing encryption key
- **WHEN** `COURIER_ENCRYPTION_KEY` is empty or decodes to fewer than 32 bytes
- **THEN** process start fails with a configuration error naming the variable

### Requirement: List and get subscriptions
`GET /v1/subscriptions` SHALL return the caller's subscriptions as `{ "items": [...] }` ordered by `created_at` ascending. `GET /v1/subscriptions/{id}` SHALL return one subscription. Both omit `signing_secret` (FR-SUB-002).

#### Scenario: Empty list
- **WHEN** an authenticated client lists subscriptions and owns none
- **THEN** the response status is 200 and `items` is an empty array

#### Scenario: Get unknown id
- **WHEN** a client GETs a well-formed UUID that does not exist for this key
- **THEN** the response status is 404 and `code` is `not_found`

#### Scenario: Get malformed id
- **WHEN** a client GETs `/v1/subscriptions/not-a-uuid`
- **THEN** the response status is 400 and `code` is `validation_failed`

### Requirement: Update subscription
`PATCH /v1/subscriptions/{id}` SHALL apply any provided subset of `target_url`, `event_types`, `headers`, `description`, `enabled`, and `rotate_secret`. Omitted fields MUST stay unchanged. Unknown JSON fields MUST be rejected (FR-SUB-002).

#### Scenario: Partial update
- **WHEN** a client PATCHes only `description`
- **THEN** the response status is 200
- **AND** `description` is the new value and `target_url` is unchanged
- **AND** `signing_secret` is absent

#### Scenario: Unknown field rejected
- **WHEN** a client PATCHes a body containing a field other than the allowed set
- **THEN** the response status is 400 and `code` is `validation_failed`

### Requirement: Disable subscription
`POST /v1/subscriptions/{id}/disable` SHALL set `enabled` to false. Disabling an already-disabled subscription MUST succeed. Disabled subscriptions remain listable and gettable (FR-SUB-002).

#### Scenario: Disable is idempotent
- **WHEN** a client POSTs disable twice on the same id
- **THEN** both responses are 200 with `enabled` false

### Requirement: Target URL scheme and shape
`target_url` MUST be an absolute URL with scheme `https`, or `http` only when `ALLOW_HTTP_CALLBACKS` is true. The URL MUST have a host and MUST NOT contain userinfo. Other schemes MUST be rejected (FR-SUB-003). Default `ALLOW_HTTP_CALLBACKS` is false.

#### Scenario: HTTPS accepted
- **WHEN** a client creates a subscription with `https://example.com/hooks`
- **THEN** the response status is 201

#### Scenario: HTTP rejected by default
- **WHEN** `ALLOW_HTTP_CALLBACKS` is false and a client submits `http://example.com/hooks`
- **THEN** the response status is 400 and `code` is `validation_failed`

#### Scenario: HTTP allowed when configured
- **WHEN** `ALLOW_HTTP_CALLBACKS` is true and a client submits `http://example.com/hooks`
- **THEN** the URL is accepted (subject to the SSRF policy)

#### Scenario: Userinfo rejected
- **WHEN** a client submits `https://user:pass@example.com/hooks`
- **THEN** the response status is 400 and `code` is `validation_failed`

#### Scenario: Non-http scheme rejected
- **WHEN** a client submits `ftp://example.com/hooks` or `javascript:alert(1)`
- **THEN** the response status is 400 and `code` is `validation_failed`

### Requirement: SSRF protection on callback IP literals
When `SSRF_PROTECTION` is true (the default), the system SHALL reject `target_url` whose host is an IP literal in loopback, private, link-local, CGNAT, unspecified, multicast, or broadcast ranges, including IPv4-mapped IPv6 forms. When `SSRF_PROTECTION` is false, IP literals that pass scheme checks are accepted. Hostname resolution at dial time is out of scope (FR-SUB-003, NFR-SEC-002).

#### Scenario: Loopback IPv4 denied
- **WHEN** `SSRF_PROTECTION` is true and a client submits `http://127.0.0.1/hooks` (HTTP allowed)
- **THEN** the response status is 400 and `code` is `validation_failed`

#### Scenario: Link-local metadata denied
- **WHEN** `SSRF_PROTECTION` is true and a client submits `https://169.254.169.254/`
- **THEN** the response status is 400 and `code` is `validation_failed`

#### Scenario: Private IPv4 denied
- **WHEN** `SSRF_PROTECTION` is true and a client submits `https://10.0.0.1/hooks` or `https://192.168.1.1/hooks` or `https://172.16.0.1/hooks`
- **THEN** the response status is 400 and `code` is `validation_failed`

#### Scenario: Loopback IPv6 and mapped forms denied
- **WHEN** `SSRF_PROTECTION` is true and a client submits `https://[::1]/hooks` or `https://[::ffff:127.0.0.1]/hooks`
- **THEN** the response status is 400 and `code` is `validation_failed`

#### Scenario: Public IP accepted
- **WHEN** `SSRF_PROTECTION` is true and a client submits `https://8.8.8.8/hooks`
- **THEN** the URL is accepted

#### Scenario: Protection off allows private IP
- **WHEN** `SSRF_PROTECTION` is false and HTTP callbacks are allowed and a client submits `http://127.0.0.1:9090/hooks`
- **THEN** the URL is accepted

### Requirement: Per-API-key isolation
Every subscription read and write SHALL filter by the authenticated `api_key_id`. A subscription owned by another key MUST be indistinguishable from a missing id: `404` with `code` `not_found`, never a distinct forbidden status (FR-AUTH-003).

#### Scenario: Foreign get is not found
- **WHEN** key A creates a subscription and key B GETs that id
- **THEN** the response status is 404 and `code` is `not_found`

#### Scenario: Foreign update is not found
- **WHEN** key B PATCHes or disables key A's subscription id
- **THEN** the response status is 404 and `code` is `not_found`

#### Scenario: List never includes another key's rows
- **WHEN** key A and key B each own a subscription
- **THEN** key A's list contains only A's subscription
