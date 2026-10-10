package domain

import "time"

// Delivery statuses. "In flight" is a lease, not a status.
const (
	DeliveryPending      DeliveryStatus = "pending"
	DeliveryRetrying     DeliveryStatus = "retrying"
	DeliveryDelivered    DeliveryStatus = "delivered"
	DeliveryDeadLettered DeliveryStatus = "dead_lettered"
)

const (
	// MaxAttempts is the last HTTP try before a delivery is dead-lettered.
	MaxAttempts = 6
	// DefaultDeliveryLimit is the list page size when the caller omits limit.
	DefaultDeliveryLimit = 50
	// MaxDeliveryLimit is the maximum accepted list page size.
	MaxDeliveryLimit = 100
)

// DeliveryStatus is the durable state of one event→subscription path.
type DeliveryStatus string

// Valid reports whether s is a known delivery status.
func (s DeliveryStatus) Valid() bool {
	switch s {
	case DeliveryPending, DeliveryRetrying, DeliveryDelivered, DeliveryDeadLettered:
		return true
	default:
		return false
	}
}

// DefaultBackoff is the delay after failure counts 1..5 (FR-DEL-005).
var DefaultBackoff = []time.Duration{
	time.Second,
	5 * time.Second,
	25 * time.Second,
	2 * time.Minute,
	10 * time.Minute,
}

// Delivery is the path of one event to one subscription.
type Delivery struct {
	ID             string
	EventID        string
	SubscriptionID string
	Status         DeliveryStatus
	AttemptCount   int
	NextAttemptAt  time.Time
	LeaseOwner     string
	LeaseUntil     time.Time
	DeliveredAt    time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// DeliveryClaim is a row a worker may POST after a committed claim.
type DeliveryClaim struct {
	Delivery         Delivery
	Payload          []byte
	TargetURL        string
	Headers          map[string]string
	SigningSecretEnc []byte
}

// Attempt is one HTTP try against a subscriber. Append-only.
type Attempt struct {
	DeliveryID   string
	StatusCode   *int
	ErrorMessage *string
	DurationMs   int
	RequestID    string
	CreatedAt    time.Time
}

// AttemptRecord is the input to persist one try and the resulting delivery state.
type AttemptRecord struct {
	DeliveryID   string
	WorkerID     string
	Status       DeliveryStatus
	AttemptCount int
	RetryDelay   time.Duration
	StatusCode   *int
	ErrorMessage *string
	DurationMs   int
	RequestID    string
}

// DeliveryView is a delivery plus the parent event type (no payload).
type DeliveryView struct {
	Delivery
	EventType string
}

// DeliveryFilter selects a page of deliveries for one API key.
type DeliveryFilter struct {
	Status         DeliveryStatus
	SubscriptionID string
	EventType      string
	CreatedAfter   *time.Time
	CreatedBefore  *time.Time
	Limit          int
	Offset         int
}

// DeliveryPage is one page of a delivery list.
type DeliveryPage struct {
	Items  []DeliveryView
	Total  int
	Limit  int
	Offset int
}

// ParseDeliveryID checks that s is a UUID string.
func ParseDeliveryID(s string) (string, error) {
	if !subscriptionIDRe.MatchString(s) {
		return "", &ValidationError{Field: "id", Reason: "must be a uuid"}
	}
	return s, nil
}

// BackoffAfterFailure returns the delay until the next try after a failure
// that leaves attemptCount (1-based) recorded. deadLetter is true at 6+.
func BackoffAfterFailure(attemptCount int, schedule []time.Duration) (delay time.Duration, deadLetter bool) {
	if attemptCount >= MaxAttempts {
		return 0, true
	}
	if attemptCount < 1 {
		return 0, false
	}
	if len(schedule) == 0 {
		schedule = DefaultBackoff
	}
	idx := attemptCount - 1
	if idx >= len(schedule) {
		return schedule[len(schedule)-1], false
	}
	return schedule[idx], false
}

// OverrideBackoff repeats delay for every production retry slot. Used only
// when tests inject COURIER_BACKOFF_MS.
func OverrideBackoff(delay time.Duration) []time.Duration {
	out := make([]time.Duration, len(DefaultBackoff))
	for i := range out {
		out[i] = delay
	}
	return out
}

func claimable(s DeliveryStatus) bool {
	return s == DeliveryPending || s == DeliveryRetrying
}

// RecordSuccess increments attempt_count and marks the delivery delivered.
func (d Delivery) RecordSuccess() (Delivery, error) {
	if !claimable(d.Status) {
		return Delivery{}, ErrInvalidTransition
	}
	d.AttemptCount++
	d.Status = DeliveryDelivered
	d.LeaseOwner = ""
	d.LeaseUntil = time.Time{}
	return d, nil
}

// RecordFailure increments attempt_count and either schedules a retry or
// dead-letters. RetryDelay is 0 when dead-lettered.
func (d Delivery) RecordFailure(schedule []time.Duration) (Delivery, time.Duration, error) {
	if !claimable(d.Status) {
		return Delivery{}, 0, ErrInvalidTransition
	}
	d.AttemptCount++
	d.LeaseOwner = ""
	d.LeaseUntil = time.Time{}
	delay, dead := BackoffAfterFailure(d.AttemptCount, schedule)
	if dead {
		d.Status = DeliveryDeadLettered
		return d, 0, nil
	}
	d.Status = DeliveryRetrying
	return d, delay, nil
}

// Replay returns a pending copy of a dead-lettered delivery. AttemptCount is
// unchanged. NextAttemptAt is left zero; the store applies the database clock.
func (d Delivery) Replay() (Delivery, error) {
	if d.Status != DeliveryDeadLettered {
		return Delivery{}, ErrInvalidTransition
	}
	d.Status = DeliveryPending
	d.LeaseOwner = ""
	d.LeaseUntil = time.Time{}
	d.DeliveredAt = time.Time{}
	d.NextAttemptAt = time.Time{}
	return d, nil
}

// LimitOrDefault returns a positive page size, capped at MaxDeliveryLimit.
func (f DeliveryFilter) LimitOrDefault() int {
	if f.Limit < 1 {
		return DefaultDeliveryLimit
	}
	if f.Limit > MaxDeliveryLimit {
		return MaxDeliveryLimit
	}
	return f.Limit
}
