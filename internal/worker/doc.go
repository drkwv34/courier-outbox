// Package worker runs the delivery loop: claim due deliveries with
// FOR UPDATE SKIP LOCKED, sign, POST to subscribers through the hardened
// outbound HTTP client, record attempts, and schedule retries or dead-letter.
//
// Nothing is implemented yet; see docs/architecture/external-integrations.md
// and the delivery capability in openspec/specs/.
package worker
