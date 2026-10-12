# Proposal

- **Id:** `add-readme-demo`
- **Status:** approved
- **SRS requirements:** FR-DOC-001, NFR-PORT-001 (SRS §9)

## Why

A reviewer cannot yet run one documented Compose smoke and read a pin-quality case study of the reliability story. The README still carries a `TODO(readme)` stub and an “OpenAPI + metrics” status line, and there is no seed script that proves enqueue → signed POST to `mock-subscriber` within ~5s (FR-DOC-001, NFR-PORT-001).

## What Changes

- Replace the README stub with a pin-quality case study: problem, why this stack (Postgres outbox + chi), Compose demo path, architecture diagram, **Consumer responsibilities**, and **Trade-offs** that state at-least-once and never claim exactly-once.
- Point a GitHub Actions CI badge at the default-branch workflow.
- Add `scripts/demo.sh`: after `docker compose up --build`, create an API key, subscribe to `mock-subscriber`, enqueue one event, and verify the mock received a signed POST within ~5s.
- Audit `.env.example` against live env vars (placeholders only).
- Document the intended GitHub About blurb and topics in the README only (the Settings API returns 403). Do not call that API.
- Update the capability index so Day 8 is marked done.

## Capabilities

### New Capabilities

- `local-demo`: one-command Compose smoke that a reviewer runs after the stack is up (NFR-PORT-001).

### Modified Capabilities

- `delivery`: extend **Consumer contract documentation** so the README is the pin-quality case study (FR-DOC-001 / SRS §9): consumer contract, at-least-once trade-offs, CI badge, and documented About/topics. No delivery-loop or signature-scheme behaviour change.

## Impact

- **Capabilities:** new `local-demo`; `delivery` documentation requirement only.
- **Endpoints / CLI:** none. Demo uses existing `courier keys create`, `POST /v1/subscriptions`, `POST /v1/events`.
- **Schema / migrations:** none.
- **Config:** audit `.env.example`; wire documented `WORKER_*` vars into Compose if they are currently ignored. No new env vars.
- **Packages:** `README.md`, `scripts/demo.sh`, `.env.example`, `docker-compose.yml` (env pass-through only if needed), `openspec/specs/README.md`, `docs/architecture/testing-strategy.md` (script name).
- **Dependencies:** none. Script uses bash, curl, and Python 3 stdlib (or jq if present).

## Out of scope

- UI console / Swagger UI; Playwright screenshots (N/A).
- Day 9 pin-ready triage or `v0.1.0` tag.
- gRPC; kickoff-club / beacon-watch / semaphore-flags / profile repo.
- Exactly-once claims.
- GitHub About/topics Settings API (403).
- New HTTP surface on api or mock-subscriber unless a one-line fix is required for the smoke path.
