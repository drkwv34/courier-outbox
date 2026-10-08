package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseEventID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{name: "uuid", in: "11111111-2222-3333-4444-555555555555"},
		{name: "empty", in: "", wantErr: true},
		{name: "not uuid", in: "not-a-uuid", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseEventID(tt.in)
			if tt.wantErr {
				if !errors.Is(err, ErrValidation) {
					t.Fatalf("error = %v, want ErrValidation", err)
				}
				return
			}
			if err != nil || got != tt.in {
				t.Fatalf("ParseEventID(%q) = %q, %v", tt.in, got, err)
			}
		})
	}
}

func TestParseEventType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{name: "simple", in: "order.created"},
		{name: "hyphen", in: "user-updated"},
		{name: "underscore", in: "user_updated"},
		{name: "empty", in: "", wantErr: true},
		{name: "wildcard", in: "*", wantErr: true},
		{name: "uppercase", in: "Order.Created", wantErr: true},
		{name: "too long", in: strings.Repeat("a", EventTypeMaxLen+1), wantErr: true},
		{name: "max length", in: strings.Repeat("a", EventTypeMaxLen)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseEventType(tt.in)
			if tt.wantErr {
				if !errors.Is(err, ErrValidation) {
					t.Fatalf("error = %v, want ErrValidation", err)
				}
				var ve *ValidationError
				if !errors.As(err, &ve) || ve.Field != "type" {
					t.Fatalf("error = %v, want field type", err)
				}
				return
			}
			if err != nil || got != tt.in {
				t.Fatalf("ParseEventType(%q) = %q, %v", tt.in, got, err)
			}
		})
	}
}

func TestParseIdempotencyKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{name: "one char", in: "a"},
		{name: "uuid-like", in: "enq-001"},
		{name: "max", in: strings.Repeat("k", IdempotencyKeyMaxLen)},
		{name: "empty", in: "", wantErr: true},
		{name: "too long", in: strings.Repeat("k", IdempotencyKeyMaxLen+1), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseIdempotencyKey(tt.in)
			if tt.wantErr {
				if !errors.Is(err, ErrValidation) {
					t.Fatalf("error = %v, want ErrValidation", err)
				}
				var ve *ValidationError
				if !errors.As(err, &ve) || ve.Field != "idempotency_key" {
					t.Fatalf("error = %v, want field idempotency_key", err)
				}
				return
			}
			if err != nil || got != tt.in {
				t.Fatalf("ParseIdempotencyKey(%q) = %q, %v", tt.in, got, err)
			}
		})
	}
}

func TestParseEventPayload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{name: "object", in: `{"order_id":"1"}`},
		{name: "empty object", in: `{}`},
		{name: "padded object", in: `  {"a":1}  `},
		{name: "array", in: `[]`, wantErr: true},
		{name: "null", in: `null`, wantErr: true},
		{name: "string", in: `"x"`, wantErr: true},
		{name: "empty", in: ``, wantErr: true},
		{name: "invalid json", in: `{`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseEventPayload([]byte(tt.in))
			if tt.wantErr {
				if !errors.Is(err, ErrValidation) {
					t.Fatalf("error = %v, want ErrValidation", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) == 0 || got[0] != '{' {
				t.Fatalf("payload = %q", got)
			}
		})
	}
}

func TestParseOccurredAt(t *testing.T) {
	t.Parallel()

	got, err := ParseOccurredAt("2026-10-07T15:04:05Z")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 10, 7, 15, 4, 5, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("got %s, want %s", got, want)
	}

	got, err = ParseOccurredAt("2026-10-07T15:04:05.123456789Z")
	if err != nil || got.Nanosecond() != 123456789 {
		t.Fatalf("nano = %v, %v", got, err)
	}

	if _, err := ParseOccurredAt("tomorrow"); !errors.Is(err, ErrValidation) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
	if _, err := ParseOccurredAt(""); !errors.Is(err, ErrValidation) {
		t.Fatalf("empty error = %v, want ErrValidation", err)
	}
}

func TestMatchesEventType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		types []string
		event string
		want  bool
	}{
		{name: "wildcard", types: []string{"*"}, event: "order.created", want: true},
		{name: "exact", types: []string{"order.created", "order.paid"}, event: "order.created", want: true},
		{name: "miss", types: []string{"order.paid"}, event: "order.created", want: false},
		{name: "empty filter", types: nil, event: "order.created", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := MatchesEventType(tt.types, tt.event); got != tt.want {
				t.Fatalf("MatchesEventType(%v, %q) = %v, want %v", tt.types, tt.event, got, tt.want)
			}
		})
	}
}

func TestNewEvent(t *testing.T) {
	t.Parallel()

	occurred := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	got, err := NewEvent("key-1", "order.created", []byte(`{"n":1}`), "idem-1", &occurred)
	if err != nil {
		t.Fatal(err)
	}
	if got.APIKeyID != "key-1" || got.Type != "order.created" || got.IdempotencyKey != "idem-1" {
		t.Fatalf("got %+v", got)
	}
	if !got.OccurredAt.Equal(occurred) {
		t.Fatalf("occurred_at = %s", got.OccurredAt)
	}

	zero, err := NewEvent("key-1", "order.created", []byte(`{}`), "idem-2", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !zero.OccurredAt.IsZero() {
		t.Fatalf("omitted occurred_at should stay zero, got %s", zero.OccurredAt)
	}

	if _, err := NewEvent("", "order.created", []byte(`{}`), "k", nil); !errors.Is(err, ErrValidation) {
		t.Fatalf("empty api key error = %v", err)
	}
}

func TestDeliveryStatusValid(t *testing.T) {
	t.Parallel()
	if !DeliveryPending.Valid() || DeliveryStatus("nope").Valid() {
		t.Fatal("Valid() mismatch")
	}
}
