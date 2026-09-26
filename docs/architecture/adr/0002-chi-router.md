# 0002. chi for HTTP routing

- **Status:** Accepted
- **Date:** 2026-09-26
- **Source:** SRS §2.5, §2.7

## Context

We need path parameters, route groups (public vs authenticated `/v1`), and composable middleware (request id, recover, auth, access log). The candidates were the stdlib `net/http` mux alone, echo, and chi.

## Decision

Use `github.com/go-chi/chi/v5`.

## Consequences

- Handlers remain plain `http.Handler` / `http.HandlerFunc`, so there is no framework context type and they are trivially testable with `httptest`.
- Route groups and middleware stacks are explicit. `chi/middleware` supplies RequestID and Recoverer. `middleware.RealIP` is deliberately not used because it is deprecated and trivially spoofable.
- It is a small dependency surface compared with echo, and there is no custom binder or validator magic. Decoding and validation stay explicit in our code.
- The Go 1.22+ stdlib mux would also work. chi was chosen for route groups and its middleware ecosystem, not out of necessity.
