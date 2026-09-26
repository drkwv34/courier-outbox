# 0004. API keys hashed with HMAC-SHA256 and a server pepper

- **Status:** Accepted
- **Date:** 2026-09-26
- **Source:** SRS FR-AUTH-001, NFR-SEC-001; change `add-persistence-and-api-keys`

## Context

NFR-SEC-001 allows argon2/bcrypt or SHA-256 with a secret pepper. Every authenticated request verifies a key, so the hash sits on the hot path.

## Decision

- Key format: `co_<prefix>_<secret>`. The prefix is 5 random bytes (8 lowercase base32 chars) and the secret is 32 random bytes (52 lowercase base32 chars), both from `crypto/rand`.
- Store `prefix` (unique, display-safe) and `key_hash = HMAC-SHA256(COURIER_API_KEY_PEPPER, raw_key)`.
- Look up by prefix and compare with `hmac.Equal` (constant time).
- The pepper comes from the environment, must be at least 32 bytes, and is never stored in the database.

## Consequences

- Keys have 256 bits of entropy, so brute force is infeasible regardless of hash speed. A slow KDF would add per-request latency for no practical gain. That trade-off is only safe because keys are machine-generated; this scheme must never be reused for human passwords.
- A database dump without the pepper cannot be used to test candidate keys.
- Rotating the pepper invalidates every key. Doing that without an outage would need a `hash_version` column, which is deferred until needed.
- The unknown-prefix and wrong-secret paths take different amounts of time, but that only reveals whether a prefix exists, and prefixes are not secret.
