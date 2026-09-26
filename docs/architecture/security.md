# Security

## Secrets in the repo

- No secrets in git, ever. `.env.example` holds placeholders and local-only dev values. `.env` is gitignored.
- CI uses throwaway credentials for service containers only.
- Any value that looks like a real key in a PR is a blocker.

## API keys (FR-AUTH-*, NFR-SEC-001)

- Format (proposed): `co_<prefix>_<random>`. The prefix is 8 chars, stored and displayable. The random part is ≥ 32 bytes, base62 or base32.
- Store only `prefix` and `key_hash`. The hash choice goes in an ADR with the api-keys change: SHA-256 with a server-side pepper (`COURIER_API_KEY_PEPPER`) is acceptable per SRS, and argon2id is the heavier option.
- Look up by prefix and compare hashes with `crypto/subtle.ConstantTimeCompare`.
- The raw key is printed once by `courier keys create` and is never retrievable.
- Revoked keys (`revoked_at` set) authenticate as 401, the same response as a wrong key.

## Signing secrets (FR-SUB-001, FR-ERR-002)

- Generate 32+ bytes from `crypto/rand` and return the secret once on create or rotate.
- Store it encrypted at rest with AES-256-GCM, keyed by `COURIER_ENCRYPTION_KEY` (32 bytes, base64). Use a random nonce per value and store `nonce || ciphertext`.
- Decrypt only inside the worker, immediately before signing. Never put a secret in a DTO, log line, or error message.

## Webhook signatures (FR-DEL-002)

- `X-Courier-Signature: v1=<hex(hmac_sha256(secret, "{timestamp}.{raw_body}"))>`, with `X-Courier-Timestamp` in unix seconds.
- The versioned prefix allows rotation to `v2` later without breaking consumers.
- Verification uses `hmac.Equal`. Consumers reject a timestamp skew over 5 minutes (documented in the README consumer contract).
- `internal/sign` needs golden-vector table tests **and** a fuzz test (SRS §8.9).

## Inbound HTTP hardening

- `http.Server` has `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`, and `IdleTimeout` set (see `cmd/courier`).
- JSON bodies are capped with `http.MaxBytesReader` at 256 KiB plus a small envelope margin, and decoded with `DisallowUnknownFields`.
- Every `/v1/*` route sits behind auth middleware. Only `/healthz`, `/readyz`, `/openapi.json`, `/docs`, and `/metrics` are public (metrics may become token-protected).

## Outbound

See [external-integrations.md](external-integrations.md) for SSRF, redirects, and timeouts. Production callbacks should be HTTPS (NFR-SEC-003).

## Dependencies

- Keep dependencies few and justified in the PR description.
- CI runs `govulncheck` (enable in the CI-hardening change).
