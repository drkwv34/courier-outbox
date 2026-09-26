# OpenSpec

Spec-driven development for courier-outbox. **Specs describe what the system does today. Changes describe what we intend to make it do.** Non-trivial code never lands without an approved change.

This follows the [OpenSpec](https://github.com/Fission-AI/OpenSpec) layout. You can use the `openspec` CLI, but it's optional. Every file here is plain Markdown and can be edited by hand.

```
openspec/
├── project.md            # Project context & conventions pointer (read first)
├── specs/                # Source of truth: current behaviour, one folder per capability
│   ├── README.md         # Capability index + SRS requirement mapping
│   └── <capability>/spec.md
├── changes/              # Proposed work, one folder per change
│   ├── <change-id>/
│   │   ├── proposal.md   # Why + what + impact
│   │   ├── tasks.md      # Implementation checklist
│   │   ├── design.md     # Optional: trade-offs, data model, sequence
│   │   └── specs/<capability>/spec.md   # Spec deltas (ADDED/MODIFIED/REMOVED)
│   └── archive/          # Completed changes, moved here after merge
└── templates/            # Copy these to start a change
```

## When a change proposal is required

| Needs a proposal | Does not |
|------------------|----------|
| New endpoint, CLI command, table, or migration | Typo / comment / docs wording |
| Changes to delivery semantics, retry schedule, signing, SSRF policy | Dependency patch bumps |
| New config variable or external dependency | Refactor with no behaviour change |
| Anything touching an SRS FR/NFR | Test-only additions for existing behaviour |

When unsure, write the proposal. They are short.

## Workflow

1. **Pick an id.** Use a kebab-case verb phrase, e.g. `add-api-keys`, `add-subscriptions`, `add-event-enqueue`.
2. **Scaffold** `openspec/changes/<id>/` from `openspec/templates/`.
3. **Write `proposal.md`.** Cover why, what changes, the SRS requirement ids covered (`FR-EVT-001`…), impact (tables, endpoints, config), and what is explicitly out of scope.
4. **Write spec deltas** in `changes/<id>/specs/<capability>/spec.md` using `## ADDED Requirements` / `## MODIFIED Requirements` / `## REMOVED Requirements`. Each requirement needs **at least one** `#### Scenario:` with WHEN/THEN bullets.
5. **Write `tasks.md`.** Make it a small, ordered checklist that maps to commits.
6. **Get approval.** Open a PR containing only the proposal (or put it first in the feature PR) and set `Status: approved` in `proposal.md` before writing implementation code.
7. **Implement.** Tick tasks as they land and keep the spec deltas truthful if the design shifts.
8. **Archive after merge.** Apply the deltas to `openspec/specs/<capability>/spec.md`, then move the change folder to `changes/archive/YYYY-MM-DD-<id>/`.

With the CLI installed: `openspec list`, `openspec validate <id> --strict`, `openspec archive <id>`.

## Requirement format

```markdown
### Requirement: Idempotent enqueue
The system SHALL return the original event when the same API key reuses an idempotency key.

#### Scenario: Duplicate key replays
- **WHEN** a producer POSTs /v1/events twice with the same idempotency_key
- **THEN** the second response is 200 with header `Idempotent-Replay: true`
- **AND** only one event row exists
```

Use SHALL/MUST for normative statements. Reference SRS ids in the requirement body where applicable.
