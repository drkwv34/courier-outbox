# Spec Delta: delivery

## ADDED Requirements

### Requirement: Pin-quality README case study
The README MUST present courier-outbox as a pin-quality reliability case study (SRS §9, FR-DOC-001): problem, why, Compose demo path, stack, architecture diagram, Consumer responsibilities, and Trade-offs. It MUST NOT contain a `TODO(readme)` stub. The status blurb MUST describe a pin-ready demo rather than OpenAPI and metrics alone. Trade-offs MUST state at-least-once and MUST NOT claim exactly-once.

#### Scenario: README is a pin-quality case study
- **WHEN** a reviewer opens the README
- **THEN** there is no `TODO(readme)` stub
- **AND** the status blurb describes a pin-ready Compose demo, not OpenAPI and metrics alone
- **AND** Why courier-outbox covers transactional enqueue, honest at-least-once semantics, verifiable HMAC, operability, and safe-by-default outbound HTTP

#### Scenario: Trade-offs reject exactly-once
- **WHEN** a reviewer reads the README Trade-offs section
- **THEN** at-least-once is explained as the chosen semantic
- **AND** the text does not claim exactly-once delivery

### Requirement: README CI badge and GitHub metadata
The README MUST show a GitHub Actions badge for this repository's default-branch CI workflow. It MUST list the intended GitHub About blurb and topics for a human to paste into Settings. It MUST NOT instruct calling the GitHub Topics or About API.

#### Scenario: README shows CI badge and GitHub metadata
- **WHEN** a reviewer reads the README
- **THEN** a GitHub Actions badge points at this repository's default-branch CI workflow
- **AND** intended About text and topics are listed for a human to paste into GitHub Settings
- **AND** the document does not instruct calling the GitHub Topics or About API
