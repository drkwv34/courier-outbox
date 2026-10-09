// Package worker runs the delivery loop: claim due deliveries with
// FOR UPDATE SKIP LOCKED, sign, POST to subscribers through the hardened
// outbound HTTP client, record attempts, and schedule retries or dead-letter.
//
// A database transaction is never held across an outbound HTTP call.
package worker
