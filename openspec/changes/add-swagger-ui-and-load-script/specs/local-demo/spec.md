# local-demo — spec delta for add-swagger-ui-and-load-script

## ADDED Requirements

### Requirement: Load script
The repository SHALL include a documented load script under `scripts/` that enqueues a configurable number of events against a running API and reports enqueue throughput (NFR-PERF-002).

#### Scenario: README documents the load script
- **WHEN** a reviewer reads the README
- **THEN** they see how to run the load script after Compose is up
- **AND** the script lives at `scripts/load.sh`

#### Scenario: Script is executable smoke
- **WHEN** CI runs a smoke of the load script
- **THEN** the script parses under `bash -n`
- **AND** it accepts a count argument or environment variable for how many events to enqueue
