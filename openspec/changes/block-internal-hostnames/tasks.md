# Tasks

## 1. Failing regression

- [ ] 1.1 Add domain table cases that `https://localhost/hooks` and `https://metadata.google.internal/` are denied when `ProtectSSRF` is true, and verify they currently fail (hostname accepted).
- [ ] 1.2 Add a worker test that `NewClient(true)` cannot POST to a loopback httptest server (dial-time block), and verify it currently succeeds.

## 2. Create/update policy

- [ ] 2.1 Inject hostname lookup into `URLPolicy.Validate`, fail closed on error/empty/nil lookup, reuse `DeniedIP`, and verify unit tests for localhost, metadata, public hostname, and unchanged IP literals.
- [ ] 2.2 Wire production lookup into `cmd/courier serve` and handler tests, and verify API POST of `https://localhost/hooks` returns 400 `validation_failed`.

## 3. Dial-time control

- [ ] 3.1 Add `net.Dialer.Control` when SSRF protection is on, change `NewClient` to take that flag, and verify loopback dial is blocked and existing httptest tests still pass with protection off.

## 4. Verification

- [ ] 4.1 `openspec validate block-internal-hostnames --strict` passes.
- [ ] 4.2 `go test -race ./internal/domain ./internal/worker ./internal/api ./cmd/courier` is green.

## Workflow follow-up

- After the PR is merged locally onto `main`, archive with `openspec archive block-internal-hostnames --yes` (add `--skip-specs` if main specs were already synced in the PR) as a direct commit on `main`.
