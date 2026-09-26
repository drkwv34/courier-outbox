# subscriptions Specification

## Purpose

Let producers register webhook endpoints (target URL, event-type filters, enabled flag, static headers) and hold a per-subscription signing secret. The secret is returned once and stored encrypted at rest. Target URLs pass scheme validation and an SSRF policy.

Planned coverage: FR-SUB-001 (create + secret once), FR-SUB-002 (list/get/update/disable/rotate), FR-SUB-003 (URL validation + SSRF guard), FR-ERR-002 (never re-expose secrets).

## Requirements

_None yet. Requirements are added by the change that implements this capability._
