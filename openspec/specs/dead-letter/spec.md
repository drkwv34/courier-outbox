# dead-letter Specification

## Purpose

Make exhausted deliveries queryable and manually replayable without losing history. Replay resets the delivery to pending, and attempt records stay append-only.

Planned coverage: FR-DLQ-001 (list with filters + pagination), FR-DLQ-002 (replay → 202, history retained), FR-ERR-003 (410/404 still retried; optional auto-disable after 50 consecutive 410s).

## Requirements

_None yet. Requirements are added by the change that implements this capability._
