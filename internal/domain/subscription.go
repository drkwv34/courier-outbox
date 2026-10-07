package domain

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// SigningSecretBytes is the length of a generated signing secret.
	SigningSecretBytes = 32
	// DescriptionMaxLen bounds the producer-supplied subscription description.
	DescriptionMaxLen = 500
	// EventTypeMaxLen bounds a single event type token.
	EventTypeMaxLen = 128
	// MaxEventTypes bounds the event_types array.
	MaxEventTypes = 64
	// HeaderNameMaxLen bounds a static header name.
	HeaderNameMaxLen = 256
	// HeaderValueMaxLen bounds a static header value.
	HeaderValueMaxLen = 1024
	// MaxHeaders bounds the static headers object.
	MaxHeaders = 32
)

var (
	eventTypeRe      = regexp.MustCompile(`^[a-z0-9]+([._-][a-z0-9]+)*$`)
	subscriptionIDRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

// Subscription is a producer-owned webhook endpoint. The signing secret is
// never a field on this type; callers hold plaintext only long enough to
// encrypt it and return it once.
type Subscription struct {
	ID          string
	APIKeyID    string
	TargetURL   string
	EventTypes  []string
	Headers     map[string]string
	Enabled     bool
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// SubscriptionUpdate is a partial mutation. Nil pointers mean "leave unchanged".
// A non-empty SecretEnc replaces the stored ciphertext.
type SubscriptionUpdate struct {
	TargetURL   *string
	EventTypes  *[]string
	Headers     *map[string]string
	Description *string
	Enabled     *bool
	SecretEnc   []byte
}

// ParseSubscriptionID checks that s is a UUID string.
func ParseSubscriptionID(s string) (string, error) {
	if !subscriptionIDRe.MatchString(s) {
		return "", &ValidationError{Field: "id", Reason: "must be a uuid"}
	}
	return s, nil
}

// GenerateSigningSecret returns SigningSecretBytes of entropy from rand.
func GenerateSigningSecret(rand io.Reader) ([]byte, error) {
	buf := make([]byte, SigningSecretBytes)
	if _, err := io.ReadFull(rand, buf); err != nil {
		return nil, fmt.Errorf("read entropy: %w", err)
	}
	return buf, nil
}

// NormalizeEventTypes validates event type filters. A nil input defaults to
// ["*"]. "*" must appear alone.
func NormalizeEventTypes(in []string) ([]string, error) {
	if in == nil {
		return []string{"*"}, nil
	}
	if len(in) == 0 {
		return nil, &ValidationError{Field: "event_types", Reason: "must not be empty"}
	}
	if len(in) > MaxEventTypes {
		return nil, &ValidationError{Field: "event_types", Reason: fmt.Sprintf("must have at most %d entries", MaxEventTypes)}
	}
	out := make([]string, 0, len(in))
	seenStar := false
	for _, t := range in {
		t = strings.TrimSpace(t)
		if t == "*" {
			seenStar = true
			out = append(out, "*")
			continue
		}
		if t == "" || utf8.RuneCountInString(t) > EventTypeMaxLen || !eventTypeRe.MatchString(t) {
			return nil, &ValidationError{Field: "event_types", Reason: "contains an invalid type"}
		}
		out = append(out, t)
	}
	if seenStar && len(out) != 1 {
		return nil, &ValidationError{Field: "event_types", Reason: "wildcard must stand alone"}
	}
	return out, nil
}

// NormalizeHeaders copies and validates static headers. Reserved names that
// would later collide with delivery headers are rejected.
func NormalizeHeaders(in map[string]string) (map[string]string, error) {
	if in == nil {
		return map[string]string{}, nil
	}
	if len(in) > MaxHeaders {
		return nil, &ValidationError{Field: "headers", Reason: fmt.Sprintf("must have at most %d entries", MaxHeaders)}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		k = strings.TrimSpace(k)
		if k == "" {
			return nil, &ValidationError{Field: "headers", Reason: "name is required"}
		}
		if utf8.RuneCountInString(k) > HeaderNameMaxLen {
			return nil, &ValidationError{Field: "headers", Reason: "name is too long"}
		}
		if utf8.RuneCountInString(v) > HeaderValueMaxLen {
			return nil, &ValidationError{Field: "headers", Reason: "value is too long"}
		}
		if reservedHeader(k) {
			return nil, &ValidationError{Field: "headers", Reason: "name is reserved"}
		}
		out[k] = v
	}
	return out, nil
}

// NormalizeDescription checks the description length. Empty is allowed.
func NormalizeDescription(s string) (string, error) {
	if utf8.RuneCountInString(s) > DescriptionMaxLen {
		return "", &ValidationError{Field: "description", Reason: fmt.Sprintf("must be at most %d characters", DescriptionMaxLen)}
	}
	return s, nil
}

func reservedHeader(name string) bool {
	lower := strings.ToLower(name)
	switch lower {
	case "host", "content-length", "authorization":
		return true
	}
	return strings.HasPrefix(lower, "x-courier-")
}
