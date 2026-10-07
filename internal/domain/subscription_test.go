package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestParseSubscriptionID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{name: "uuid", in: "11111111-2222-3333-4444-555555555555"},
		{name: "uppercase", in: "AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE"},
		{name: "empty", in: "", wantErr: true},
		{name: "not uuid", in: "not-a-uuid", wantErr: true},
		{name: "missing hyphens", in: "11111111222233334444555555555555", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseSubscriptionID(tt.in)
			if tt.wantErr {
				if !errors.Is(err, ErrValidation) {
					t.Fatalf("error = %v, want ErrValidation", err)
				}
				return
			}
			if err != nil || got != tt.in {
				t.Fatalf("ParseSubscriptionID(%q) = %q, %v", tt.in, got, err)
			}
		})
	}
}

func TestNormalizeEventTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      []string
		want    []string
		wantErr bool
	}{
		{name: "nil defaults to star", in: nil, want: []string{"*"}},
		{name: "star alone", in: []string{"*"}, want: []string{"*"}},
		{name: "named types", in: []string{"order.created", "user-updated"}, want: []string{"order.created", "user-updated"}},
		{name: "empty rejected", in: []string{}, wantErr: true},
		{name: "star mixed", in: []string{"*", "order.created"}, wantErr: true},
		{name: "invalid token", in: []string{"Order.Created"}, wantErr: true},
		{name: "blank token", in: []string{" "}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := NormalizeEventTypes(tt.in)
			if tt.wantErr {
				if !errors.Is(err, ErrValidation) {
					t.Fatalf("error = %v, want ErrValidation", err)
				}
				var ve *ValidationError
				if !errors.As(err, &ve) || ve.Field != "event_types" {
					t.Fatalf("error = %v, want field event_types", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizeHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      map[string]string
		wantErr string
	}{
		{name: "nil", in: nil},
		{name: "custom", in: map[string]string{"X-Shop": "acme"}},
		{name: "host reserved", in: map[string]string{"Host": "evil"}, wantErr: "reserved"},
		{name: "authorization reserved", in: map[string]string{"Authorization": "Bearer x"}, wantErr: "reserved"},
		{name: "content-length reserved", in: map[string]string{"Content-Length": "1"}, wantErr: "reserved"},
		{name: "x-courier reserved", in: map[string]string{"X-Courier-Signature": "nope"}, wantErr: "reserved"},
		{name: "empty name", in: map[string]string{" ": "v"}, wantErr: "name is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := NormalizeHeaders(tt.in)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got == nil {
				t.Fatal("headers is nil")
			}
		})
	}
}

func TestNormalizeDescription(t *testing.T) {
	t.Parallel()

	if _, err := NormalizeDescription(""); err != nil {
		t.Fatal(err)
	}
	if _, err := NormalizeDescription(strings.Repeat("é", DescriptionMaxLen)); err != nil {
		t.Fatal(err)
	}
	err := func() error {
		_, e := NormalizeDescription(strings.Repeat("a", DescriptionMaxLen+1))
		return e
	}()
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("too long = %v, want ErrValidation", err)
	}
}
