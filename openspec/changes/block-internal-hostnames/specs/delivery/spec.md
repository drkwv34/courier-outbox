# delivery — spec delta for block-internal-hostnames

## ADDED Requirements

### Requirement: Dial-time SSRF check
When `SSRF_PROTECTION` is true, the outbound client MUST reject a connection whose resolved IP is in the same deny-list as create-time policy, using a dialer control hook. This MUST apply even when DNS at create time returned a public address (FR-SUB-003, NFR-SEC-002).

#### Scenario: Dial to loopback is blocked
- **WHEN** `SSRF_PROTECTION` is true and the worker dials a subscriber whose resolved address is `127.0.0.1`
- **THEN** the POST does not succeed
- **AND** the attempt is recorded as a failure

#### Scenario: Dial-time rebinding is blocked
- **WHEN** `SSRF_PROTECTION` is true and DNS at dial time resolves the hostname to a private or loopback address
- **THEN** the dialer control hook rejects the connection
- **AND** no request is sent to that address

#### Scenario: Protection off allows private dial
- **WHEN** `SSRF_PROTECTION` is false
- **THEN** the outbound client may dial private and loopback addresses that pass scheme checks
