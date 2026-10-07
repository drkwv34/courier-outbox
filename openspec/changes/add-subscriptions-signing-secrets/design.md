# Design: add-subscriptions-signing-secrets

## Context

Day 2 left a staged `subscriptions` table (`000002`), Bearer auth on `/v1`, and config flags named but unused: `COURIER_ENCRYPTION_KEY`, `SSRF_PROTECTION`, `ALLOW_HTTP_CALLBACKS`. No subscription handlers exist; authenticated `GET /v1/subscriptions` currently 404s. See proposal.md for motivation. Constraints: domain stays pure (no `net/http`); store queries always filter by `api_key_id`; secrets never appear in logs.

## Goals / Non-Goals

**Goals:**

- Thin handlers: decode → domain validate → store → map error.
- Envelope encryption and URL/SSRF policy as unit-testable domain functions (table-driven deny-list).
- Existing `subscriptions` schema reused; no new migration unless a CHECK is missing.
- Dial-time SSRF (custom DialContext) is designed for the worker, not implemented here.

**Non-Goals:**

- Payload HMAC, outbound POST, enqueue, DLQ (see proposal out of scope).
- Decrypting secrets in the API process except tests that assert ciphertext ≠ plaintext.

## Decisions

- **No new migration.** `signing_secret_enc bytea NOT NULL`, JSONB `event_types`/`headers`, `enabled`, and `api_key_id` already match FR-SUB-001. A CHECK on URL shape is not added: URL policy is config-dependent and belongs in Go.
- **Disable is `POST /v1/subscriptions/{id}/disable`**, not DELETE. Rows stay for later delivery history. PATCH may set `enabled` true/false so a disabled subscription can be turned back on. Disable is idempotent.
- **Secret encoding:** 32 raw bytes from `crypto/rand`, shown as `base64.StdEncoding` in JSON. Ciphertext is AES-256-GCM of the raw bytes (`nonce || ciphertext`, 12-byte nonce). Alternative considered: store the base64 string; rejected so the worker signs with raw bytes and encoding stays an HTTP concern.
- **Encryptor in `internal/domain`.** `crypto/aes` is stdlib and the key is injected; store persists `[]byte` only. Alternative: encrypt inside store. Rejected so unit tests cover Seal/Open without Postgres.
- **`COURIER_ENCRYPTION_KEY` is standard base64 of 32 bytes** (RawStdEncoding also accepted). Empty/wrong length fails `config.Load` with `ErrInvalid`. Compose and `.env.example` get a local-only 32-byte placeholder; production must set a real key.
- **Boolean env parsing:** empty → documented default (`SSRF_PROTECTION=true`, `ALLOW_HTTP_CALLBACKS=false`); `true`/`false` case-insensitive; anything else is `ErrInvalid`. Compose already overrides both for the mock subscriber.
- **URL policy is data.** `domain.URLPolicy{AllowHTTP, ProtectSSRF}` parses with `net/url`, rejects userinfo and empty host, then if the host is an IP literal (`netip.ParseAddr` after unbracketing) checks a table of denied prefixes via `netip.Addr.Unmap()`. Hostnames are not resolved at create/update (dial-time check is Day 5). Alternative: reject `localhost` by name; deferred to keep create-time policy identical to docs/architecture/external-integrations.md.
- **Denied networks (when protection on):** `0.0.0.0/8`, `127.0.0.0/8`, `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `169.254.0.0/16`, `100.64.0.0/10`, multicast/broadcast, `::`, `::1`, `fc00::/7`, `fe80::/10`, `ff00::/8`, plus mapped IPv4 forms of the IPv4 ranges.
- **JSON decode:** shared helper, `http.MaxBytesReader` 256 KiB, `DisallowUnknownFields`, reject trailing data. Map `http.MaxBytesError` → 413; syntax/unknown field → 400 `validation_failed`.
- **Error mapping:** one `writeDomainError` using `errors.Is`/`As`: `ErrValidation` 400, `ErrNotFound` 404, `ErrConflict` 409, else 500 generic. `*ValidationError` message is `"field: reason"`.
- **IDs:** path `{id}` must be a UUID (`uuid` text from Postgres). Invalid UUID → 400, not 404, so clients can tell a bad handle from a missing row.
- **List shape:** `{ "items": [ ... ] }` with `created_at` ascending. No pagination in this change (volume is expected small per key).
- **Reserved static headers:** reject case-insensitive names `Host`, `Content-Length`, `Authorization`, and any `X-Courier-*` at create/update so they cannot later override delivery headers.
- **Wiring:** `api.Deps` gains `Subscriptions` (consumer-declared interface), `URLPolicy`, and `Envelope`. `cmd/courier` builds them from config. Store methods: `CreateSubscription`, `ListSubscriptions`, `GetSubscription`, `UpdateSubscription`, `DisableSubscription` — each `WHERE api_key_id = $1`. Zero rows on update/disable/get → `ErrNotFound`.

## Risks / Trade-offs

- **[Risk] Hostname `localhost` or DNS-rebinding names pass create-time checks.** → Mitigation: documented; worker DialContext (later change) checks resolved IPs. Compose demo relies on hostnames on the Docker network.
- **[Risk] Losing `COURIER_ENCRYPTION_KEY` makes every stored secret unusable.** → Mitigation: fail fast at startup; README warns that rotation of the envelope key is out of scope (would need re-encrypt).
- **[Risk] Existing handler tests expect `GET /v1/subscriptions` → 404.** → Mitigation: point those auth-passthrough cases at `/v1/events` (still unrouted).
- **[Trade-off] No pagination.** Acceptable until enqueue/delivery create volume; add a follow-up change if needed.
- **[Trade-off] Disable keeps the row.** Needed for delivery history; there is no hard-delete.

## Migration Plan

- Deploy requires setting `COURIER_ENCRYPTION_KEY` before new binaries start. Compose placeholder is local-only.
- Schema already migrated; `courier migrate` is a no-op on up-to-date databases.
- Rollback: revert the binary; leftover subscription rows are harmless. Do not drop `signing_secret_enc`.

## Test plan

- **Unit (domain):** URL allow/deny matrix (schemes, userinfo, IPv4/IPv6/mapped, protection on/off, HTTP allowed/denied); event_types (`*` alone, empty, valid names); reserved headers; envelope Seal/Open, wrong key, truncated blob; secret length 32; `LogValue` redacts secrets.
- **Unit (config):** missing/short/invalid encryption key; bool defaults and invalid values; errors do not echo the key material.
- **Unit (api, httptest + fakes):** create 201 + secret; get/list omit secret; rotate returns new secret; disable; validation 400; isolation 404; unknown JSON field 400; 413 on oversized body.
- **Integration (tagged, real Postgres):** create → list → get → patch description → rotate → disable; ciphertext in DB ≠ plaintext; second key cannot get/update/disable the first key's id (404); secret not re-readable.
