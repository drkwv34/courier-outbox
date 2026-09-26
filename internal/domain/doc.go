// Package domain holds courier-outbox's pure business model: API keys,
// subscriptions, events, deliveries, attempts, delivery status transitions,
// the retry/backoff schedule, and sentinel errors.
//
// Rules: no I/O, no SQL, no net/http, no Redis, no logging. Time and
// randomness are injected. Everything here is unit-testable without
// containers. See docs/architecture/domain-model.md.
package domain
