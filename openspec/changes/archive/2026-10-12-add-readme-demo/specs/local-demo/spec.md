# Spec Delta: local-demo

## Purpose

Give a reviewer a one-command Compose smoke path that creates a key, subscribes the mock, enqueues one event, and proves the mock received a signed POST within about five seconds (NFR-PORT-001).

## ADDED Requirements

### Requirement: Compose demo seed
The repository SHALL include a documented seed script that a reviewer runs after `docker compose up --build`. The script MUST create an API key, create a subscription whose target is the Compose `mock-subscriber`, enqueue one event, and exit 0 only when that mock has received a signed POST for the event within about 5 seconds (NFR-PORT-001).

#### Scenario: Happy-path smoke
- **WHEN** Compose is healthy (`/healthz` and `/readyz` succeed) and a reviewer runs the documented demo script
- **THEN** the script creates an API key with `courier keys create`
- **AND** it creates a subscription targeting `http://mock-subscriber:9090/hooks`
- **AND** it enqueues one event
- **AND** it exits 0 after the mock log shows a POST with `X-Courier-Signature` present and `X-Courier-Idempotency-Key` equal to the event id

#### Scenario: Timeout is a failure
- **WHEN** the mock does not receive a signed POST for that event id within about 5 seconds
- **THEN** the script exits non-zero
- **AND** it does not print the API key, signing secret, or payload body after creation

### Requirement: README documents the one-command path
The README MUST document `docker compose up --build` followed by the demo script as the reviewer smoke path.

#### Scenario: README points at the script
- **WHEN** a reviewer reads the Run locally section
- **THEN** they see `docker compose up --build` and the demo script invocation
- **AND** they do not need an undocumented curl sequence to satisfy NFR-PORT-001
