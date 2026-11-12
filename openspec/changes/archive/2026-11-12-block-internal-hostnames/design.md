# Design

## Context

See proposal.md for motivation. `domain.URLPolicy.Validate` already denies IP literals via `DeniedIP`. Hostnames return nil. `worker.NewClient` uses `net.Dialer` with no `Control`. Domain must stay free of resolver I/O. Compose sets `SSRF_PROTECTION=false` so the mock on the private network still works.

## Goals / Non-Goals

**Goals:**

- Create/update reject hostnames that resolve to denied ranges.
- Dial-time Control rejects the resolved IP (rebinding).
- Literal IP behaviour unchanged.

**Non-Goals:**

- Pinning DNS results between create and dial.
- Allowlisting specific internal hosts while protection is on.

## Decisions

- **Inject `Lookup func(host string) ([]netip.Addr, error)` on `URLPolicy`.** Domain stays pure. Nil lookup with `ProtectSSRF` and a hostname fails closed (`destination is blocked`) so a forgotten wire cannot reopen the hole. Tests inject a static map; production uses `net.Resolver.LookupNetIP` with a 2s timeout in `internal/worker`.
- **Fail closed on lookup error or empty answers.** Cannot prove the destination is public.
- **`NewClient(protectSSRF bool)`.** When true, `Dialer.Control` parses the resolved `address` host as `netip.Addr` and calls `domain.DeniedIP`. Sentinel `ssrf: destination blocked` (no URL echo). Alternative: custom `DialContext`. Control is the documented hook and runs after resolve, before connect.
- **Pool default client stays unprotected** (`NewClient(false)`) so httptest servers on loopback keep working. Production worker passes `cfg.SSRFProtection`.
- **Do not put `net.Lookup*` in `internal/domain`.** I/O stays in worker/api wiring.

## Risks / Trade-offs

- **[Risk] Create-time DNS and dial-time DNS differ (rebinding).** → Mitigation: Control hook is authoritative at connect.
- **[Risk] Public hostnames with both A and AAAA where one is private.** → Mitigation: deny if any resolved address is denied.
- **[Trade-off] Compose keeps SSRF off.** Local mock stays reachable; production defaults stay safe.

## Migration Plan

- Deploy API + worker together. Existing subscriptions to `localhost` remain in the DB; new writes are rejected; dials are blocked when protection is on. Rollback: previous binaries accept hostnames again.

## Test plan

- **Unit (domain):** localhost and metadata.google.internal denied with stub lookup; example.com public allowed; nil lookup hostname denied; IP literals unchanged.
- **Unit (worker):** `NewClient(true)` against httptest (127.0.0.1) fails; Control rejects 127.0.0.1 and 169.254.169.254; `NewClient(false)` still posts to httptest.
- **API:** POST `https://localhost/hooks` → 400; POST `https://metadata.google.internal/` → 400.
