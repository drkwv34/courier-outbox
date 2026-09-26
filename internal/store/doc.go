// Package store is the persistence adapter: PostgreSQL (system of record)
// and Redis (leases, rate limits). It implements repository interfaces
// declared by the consumers that need them and owns transaction boundaries
// for multi-row writes (e.g. event + deliveries in one TX).
//
// Rules: explicit SQL (sqlc or hand-written pgx), no ORM, driver errors
// translated to domain sentinels before leaving this package. See
// docs/architecture/transactionality.md.
package store
