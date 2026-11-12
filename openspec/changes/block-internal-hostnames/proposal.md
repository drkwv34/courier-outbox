# Proposal

- **Id:** `block-internal-hostnames`
- **Status:** approved
- **SRS requirements:** FR-SUB-003, NFR-SEC-002

## Why

With `SSRF_PROTECTION=true`, create/update only block IP literals. Hostnames such as `localhost` and `metadata.google.internal` are accepted, and the outbound dialer does not check the resolved IP, so the worker can reach loopback, link-local, and private addresses.

## What Changes

- At subscription create and update, when `SSRF_PROTECTION` is true, resolve hostnames and reject any address in the existing deny-list (loopback, private, link-local, CGNAT, unspecified, multicast, broadcast, IPv4-mapped forms). Lookup failure fails closed.
- Literal-IP behaviour is unchanged.
- At dial time, `net.Dialer.Control` rejects the same ranges on the resolved IP so DNS rebinding cannot bypass create-time checks.
- When `SSRF_PROTECTION` is false (Compose), hostname and private destinations remain allowed after scheme checks.

## Capabilities

### New Capabilities

- None.

### Modified Capabilities

- `subscriptions`: create/update SSRF policy covers hostname resolution, not only IP literals.
- `delivery`: outbound client enforces the deny-list on the resolved IP at dial time.

## Impact

- **Capabilities:** `subscriptions`, `delivery`.
- **Endpoints / CLI:** `POST`/`PATCH /v1/subscriptions` reject blocked hostnames with 400 `validation_failed`. Worker POSTs fail the attempt with a blocked-destination error when the resolved IP is denied.
- **Schema / migrations:** none.
- **Config:** none new; `SSRF_PROTECTION` already exists.
- **Packages:** `internal/domain` (lookup hook + deny-list reuse), `internal/api` (wire lookup), `internal/worker` (dialer Control, client constructor), README.
- **Dependencies:** none new.

## Out of scope

- Changing the IP deny-list ranges.
- Worker metrics, Swagger UI, load script, replay backoff.
- Token-protected metrics or exactly-once delivery.
