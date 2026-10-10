# delivery — spec delta for add-dlq-replay-logs

## MODIFIED Requirements

### Requirement: Consumer contract documentation
The README MUST document the signature scheme and consumer responsibilities: verify HMAC, reject timestamp skew greater than 5 minutes, treat delivery as at-least-once and dedupe on `X-Courier-Idempotency-Key`, return 2xx only after durable accept, and respond quickly (FR-DOC-001). It MUST also note that operators can list `dead_lettered` deliveries and replay them.

#### Scenario: README states at-least-once
- **WHEN** a reviewer reads the README worker and consumer sections
- **THEN** the signature headers and signed string are described
- **AND** delivery is stated as at-least-once, not exactly-once

#### Scenario: README states consumer responsibilities
- **WHEN** a reviewer reads the README Consumer responsibilities section
- **THEN** HMAC verification, 5-minute skew rejection, idempotency-key dedupe, 2xx-after-durable-accept, and fast response are listed
- **AND** dead-letter list and replay are mentioned for operators
