# subscriptions — spec delta for block-internal-hostnames

## MODIFIED Requirements

### Requirement: SSRF protection on callback IP literals
When `SSRF_PROTECTION` is true (the default), the system SHALL reject `target_url` whose host is an IP literal in loopback, private, link-local, CGNAT, unspecified, multicast, or broadcast ranges, including IPv4-mapped IPv6 forms, and SHALL reject hostnames that resolve to those ranges. Lookup failure MUST fail closed. When `SSRF_PROTECTION` is false, those destinations are accepted after scheme checks (FR-SUB-003, NFR-SEC-002).

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

#### Scenario: Loopback hostname denied
- **WHEN** `SSRF_PROTECTION` is true and a client submits `https://localhost/hooks`
- **THEN** the response status is 400 and `code` is `validation_failed`

#### Scenario: Metadata hostname denied
- **WHEN** `SSRF_PROTECTION` is true and a client submits `https://metadata.google.internal/`
- **THEN** the response status is 400 and `code` is `validation_failed`

#### Scenario: Public hostname accepted
- **WHEN** `SSRF_PROTECTION` is true and a client submits `https://example.com/hooks` and lookup returns a public address
- **THEN** the URL is accepted
