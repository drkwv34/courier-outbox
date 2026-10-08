package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
	"unicode/utf8"
)

const (
	// IdempotencyKeyMaxLen bounds a producer-supplied idempotency key.
	IdempotencyKeyMaxLen = 128
)

// Event is an immutable fact from a producer. Payload is a JSON object as
// raw bytes; it is never logged.
type Event struct {
	ID             string
	APIKeyID       string
	Type           string
	Payload        []byte
	IdempotencyKey string
	OccurredAt     time.Time
	CreatedAt      time.Time
}

// EnqueueResult is the outcome of persisting an event and its deliveries.
// Replay is true when the idempotency key already existed for this API key.
type EnqueueResult struct {
	EventID     string
	DeliveryIDs []string
	Replay      bool
}

// ParseEventID checks that s is a UUID string.
func ParseEventID(s string) (string, error) {
	if !subscriptionIDRe.MatchString(s) {
		return "", &ValidationError{Field: "id", Reason: "must be a uuid"}
	}
	return s, nil
}

// ParseEventType validates a producer event type. Wildcards are not allowed.
func ParseEventType(s string) (string, error) {
	if s == "" || utf8.RuneCountInString(s) > EventTypeMaxLen || !eventTypeRe.MatchString(s) {
		return "", &ValidationError{Field: "type", Reason: "must be a lowercase token of 1-128 characters"}
	}
	return s, nil
}

// ParseIdempotencyKey validates a 1–128 character idempotency key.
func ParseIdempotencyKey(s string) (string, error) {
	n := utf8.RuneCountInString(s)
	if n < 1 || n > IdempotencyKeyMaxLen {
		return "", &ValidationError{Field: "idempotency_key", Reason: fmt.Sprintf("must be 1-%d characters", IdempotencyKeyMaxLen)}
	}
	return s, nil
}

// ParseEventPayload checks that raw is a JSON object.
func ParseEventPayload(raw []byte) ([]byte, error) {
	trim := bytes.TrimSpace(raw)
	if len(trim) == 0 || trim[0] != '{' || !json.Valid(trim) {
		return nil, &ValidationError{Field: "payload", Reason: "must be a json object"}
	}
	return trim, nil
}

// ParseOccurredAt parses an RFC 3339 timestamp.
func ParseOccurredAt(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, &ValidationError{Field: "occurred_at", Reason: "must be rfc 3339"}
	}
	return t.UTC(), nil
}

// MatchesEventType reports whether a subscription's event_types filter
// includes eventType. "*" matches every type.
func MatchesEventType(eventTypes []string, eventType string) bool {
	for _, t := range eventTypes {
		if t == "*" || t == eventType {
			return true
		}
	}
	return false
}

// NewEvent validates producer input. A zero OccurredAt means the store
// should use the database clock.
func NewEvent(apiKeyID, typ string, payload []byte, idempotencyKey string, occurredAt *time.Time) (Event, error) {
	if apiKeyID == "" {
		return Event{}, &ValidationError{Field: "api_key_id", Reason: "is required"}
	}
	parsedType, err := ParseEventType(typ)
	if err != nil {
		return Event{}, err
	}
	parsedPayload, err := ParseEventPayload(payload)
	if err != nil {
		return Event{}, err
	}
	parsedKey, err := ParseIdempotencyKey(idempotencyKey)
	if err != nil {
		return Event{}, err
	}
	e := Event{
		APIKeyID:       apiKeyID,
		Type:           parsedType,
		Payload:        parsedPayload,
		IdempotencyKey: parsedKey,
	}
	if occurredAt != nil {
		if occurredAt.IsZero() {
			return Event{}, &ValidationError{Field: "occurred_at", Reason: "must be rfc 3339"}
		}
		e.OccurredAt = occurredAt.UTC()
	}
	return e, nil
}
