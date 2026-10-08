package domain

import "time"

// Delivery statuses. "In flight" is a lease, not a status.
const (
	DeliveryPending      DeliveryStatus = "pending"
	DeliveryRetrying     DeliveryStatus = "retrying"
	DeliveryDelivered    DeliveryStatus = "delivered"
	DeliveryDeadLettered DeliveryStatus = "dead_lettered"
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

// Delivery is the path of one event to one subscription.
type Delivery struct {
	ID             string
	EventID        string
	SubscriptionID string
	Status         DeliveryStatus
	AttemptCount   int
	NextAttemptAt  time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
