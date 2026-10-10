package domain

import (
	"errors"
	"testing"
	"time"
)

func TestBackoffAfterFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		attempt   int
		schedule  []time.Duration
		wantDelay time.Duration
		wantDead  bool
	}{
		{name: "first failure 1s", attempt: 1, wantDelay: time.Second},
		{name: "second failure 5s", attempt: 2, wantDelay: 5 * time.Second},
		{name: "third failure 25s", attempt: 3, wantDelay: 25 * time.Second},
		{name: "fourth failure 2m", attempt: 4, wantDelay: 2 * time.Minute},
		{name: "fifth failure 10m", attempt: 5, wantDelay: 10 * time.Minute},
		{name: "sixth failure dead-letters", attempt: 6, wantDead: true},
		{name: "seventh still dead-letters", attempt: 7, wantDead: true},
		{name: "override 10ms", attempt: 1, schedule: OverrideBackoff(10 * time.Millisecond), wantDelay: 10 * time.Millisecond},
		{name: "override still dlq at 6", attempt: 6, schedule: OverrideBackoff(10 * time.Millisecond), wantDead: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			delay, dead := BackoffAfterFailure(tt.attempt, tt.schedule)
			if dead != tt.wantDead {
				t.Fatalf("deadLetter = %v, want %v", dead, tt.wantDead)
			}
			if delay != tt.wantDelay {
				t.Fatalf("delay = %s, want %s", delay, tt.wantDelay)
			}
		})
	}
}

func TestRecordSuccess(t *testing.T) {
	t.Parallel()

	d := Delivery{ID: "d1", Status: DeliveryPending, AttemptCount: 0, LeaseOwner: "w1"}
	got, err := d.RecordSuccess()
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != DeliveryDelivered || got.AttemptCount != 1 || got.LeaseOwner != "" {
		t.Fatalf("got %+v", got)
	}

	retrying := Delivery{ID: "d2", Status: DeliveryRetrying, AttemptCount: 2}
	got, err = retrying.RecordSuccess()
	if err != nil || got.Status != DeliveryDelivered || got.AttemptCount != 3 {
		t.Fatalf("retrying success = %+v, %v", got, err)
	}
}

func TestRecordFailure_ScheduleAndDeadLetter(t *testing.T) {
	t.Parallel()

	d := Delivery{Status: DeliveryPending, AttemptCount: 0}
	got, delay, err := d.RecordFailure(nil)
	if err != nil || got.Status != DeliveryRetrying || got.AttemptCount != 1 || delay != time.Second {
		t.Fatalf("first fail = %+v delay %s err %v", got, delay, err)
	}

	got, delay, err = got.RecordFailure(DefaultBackoff)
	if err != nil || got.AttemptCount != 2 || delay != 5*time.Second {
		t.Fatalf("second fail = %+v delay %s err %v", got, delay, err)
	}

	cur := Delivery{Status: DeliveryRetrying, AttemptCount: 5}
	got, delay, err = cur.RecordFailure(DefaultBackoff)
	if err != nil || got.Status != DeliveryDeadLettered || got.AttemptCount != 6 || delay != 0 {
		t.Fatalf("sixth fail = %+v delay %s err %v", got, delay, err)
	}
}

func TestRecord_InvalidTransition(t *testing.T) {
	t.Parallel()

	for _, status := range []DeliveryStatus{DeliveryDelivered, DeliveryDeadLettered} {
		d := Delivery{Status: status, AttemptCount: 1}
		if _, err := d.RecordSuccess(); !errors.Is(err, ErrInvalidTransition) {
			t.Fatalf("success from %s: %v", status, err)
		}
		if _, _, err := d.RecordFailure(nil); !errors.Is(err, ErrInvalidTransition) {
			t.Fatalf("failure from %s: %v", status, err)
		}
	}
}

func TestReplay(t *testing.T) {
	t.Parallel()

	d := Delivery{
		ID:            "d1",
		Status:        DeliveryDeadLettered,
		AttemptCount:  6,
		LeaseOwner:    "stale",
		LeaseUntil:    time.Now(),
		NextAttemptAt: time.Now().Add(time.Hour),
	}
	got, err := d.Replay()
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != DeliveryPending {
		t.Fatalf("status = %s", got.Status)
	}
	if got.AttemptCount != 6 {
		t.Fatalf("attempt_count = %d, want 6", got.AttemptCount)
	}
	if got.LeaseOwner != "" || !got.LeaseUntil.IsZero() {
		t.Fatalf("lease not cleared: %+v", got)
	}
	if !got.NextAttemptAt.IsZero() {
		t.Fatal("next_attempt_at should be zero for the store to apply now()")
	}
}

func TestReplay_InvalidTransition(t *testing.T) {
	t.Parallel()

	for _, status := range []DeliveryStatus{DeliveryPending, DeliveryRetrying, DeliveryDelivered} {
		d := Delivery{Status: status, AttemptCount: 2}
		if _, err := d.Replay(); !errors.Is(err, ErrInvalidTransition) {
			t.Fatalf("replay from %s: %v", status, err)
		}
	}
}

func TestParseDeliveryID(t *testing.T) {
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
			got, err := ParseDeliveryID(tt.in)
			if tt.wantErr {
				if !errors.Is(err, ErrValidation) {
					t.Fatalf("error = %v, want ErrValidation", err)
				}
				return
			}
			if err != nil || got != tt.in {
				t.Fatalf("ParseDeliveryID(%q) = %q, %v", tt.in, got, err)
			}
		})
	}
}

func TestDeliveryFilter_LimitOrDefault(t *testing.T) {
	t.Parallel()

	if got := (DeliveryFilter{}).LimitOrDefault(); got != DefaultDeliveryLimit {
		t.Fatalf("empty = %d", got)
	}
	if got := (DeliveryFilter{Limit: 10}).LimitOrDefault(); got != 10 {
		t.Fatalf("10 = %d", got)
	}
	if got := (DeliveryFilter{Limit: MaxDeliveryLimit + 1}).LimitOrDefault(); got != MaxDeliveryLimit {
		t.Fatalf("over max = %d", got)
	}
}
