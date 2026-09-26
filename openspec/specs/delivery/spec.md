# delivery Specification

## Purpose

Deliver events to subscribers at least once. Workers claim due deliveries with `FOR UPDATE SKIP LOCKED` under time-bounded leases, sign payloads (v1 HMAC-SHA256), and POST through an isolated HTTP client with strict timeouts and SSRF protection. They record every attempt and schedule retries (1s → 5s → 25s → 2m → 10m) before dead-lettering.

Planned coverage: FR-DEL-001..007, NFR-REL-001 (no lost deliveries on crash), NFR-PERF-002 (≥ 20 deliveries/s locally), NFR-SEC-002 (signature golden tests).

## Requirements

_None yet. Requirements are added by the change that implements this capability._
