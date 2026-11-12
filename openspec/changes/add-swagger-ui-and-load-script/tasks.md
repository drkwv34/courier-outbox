# Tasks

## 1. Failing regression

- [ ] 1.1 Add `TestDocs_PublicHTML` expecting `GET /docs` 200 HTML, and verify it currently 404s.
- [ ] 1.2 Add `scripts/load.sh` (even a stub) and a CI `bash -n` step so the missing script is visible until the real script lands.

## 2. Swagger UI

- [ ] 2.1 Vendor swagger-ui assets (no runtime CDN), embed them, mount `/docs`, update OpenAPI, and verify `GET /docs` is 200 HTML referencing `/openapi.json`.

## 3. Load script

- [ ] 3.1 Implement `scripts/load.sh` with COUNT/KEY/BASE_URL, document it in README, and verify `bash -n scripts/load.sh` and the README mention.

## 4. Verification

- [ ] 4.1 `openspec validate add-swagger-ui-and-load-script --strict` passes.
- [ ] 4.2 `go test -race ./internal/api ./openapi` is green and CI YAML includes the load-script smoke.

## Workflow follow-up

- After the PR is merged locally onto `main`, archive with `openspec archive add-swagger-ui-and-load-script --yes` (add `--skip-specs` if main specs were already synced in the PR) as a direct commit on `main`.
