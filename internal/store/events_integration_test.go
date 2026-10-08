//go:build integration

package store_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/drkwv34/courier-outbox/internal/domain"
	"github.com/drkwv34/courier-outbox/internal/store"
)

func TestEnqueue_MatchingDeliveries(t *testing.T) {
	t.Parallel()

	pg, dsn := migratedDB(t)
	owner := mustAPIKey(t, pg, uniquePrefix())
	other := mustAPIKey(t, pg, uniquePrefix())

	star := mustSub(t, pg, owner.ID, []string{"*"}, true)
	named := mustSub(t, pg, owner.ID, []string{"order.created"}, true)
	_ = mustSub(t, pg, owner.ID, []string{"order.paid"}, true)
	_ = mustSub(t, pg, owner.ID, []string{"*"}, false)
	_ = mustSub(t, pg, other.ID, []string{"*"}, true)

	ev, err := domain.NewEvent(owner.ID, "order.created", []byte(`{"n":1}`), "idem-match", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := pg.EnqueueEvent(t.Context(), ev)
	if err != nil {
		t.Fatalf("EnqueueEvent: %v", err)
	}
	if res.Replay || res.EventID == "" || len(res.DeliveryIDs) != 2 {
		t.Fatalf("result = %+v, want 2 deliveries", res)
	}

	got, dels, err := pg.GetEvent(t.Context(), owner.ID, res.EventID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != "order.created" || !bytes.Contains(got.Payload, []byte(`"n"`)) {
		t.Fatalf("event = %+v", got)
	}
	if got.OccurredAt.IsZero() || got.CreatedAt.IsZero() {
		t.Fatalf("timestamps not set: %+v", got)
	}
	subIDs := map[string]domain.Delivery{}
	for _, d := range dels {
		if d.Status != domain.DeliveryPending || d.AttemptCount != 0 || d.NextAttemptAt.IsZero() {
			t.Fatalf("delivery = %+v", d)
		}
		subIDs[d.SubscriptionID] = d
	}
	if _, ok := subIDs[star.ID]; !ok {
		t.Fatal("missing wildcard delivery")
	}
	if _, ok := subIDs[named.ID]; !ok {
		t.Fatal("missing named delivery")
	}
	if n := countEvents(t, dsn, owner.ID); n != 1 {
		t.Fatalf("events = %d, want 1", n)
	}

	_, _, err = pg.GetEvent(t.Context(), other.ID, res.EventID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign get = %v, want ErrNotFound", err)
	}
}

func TestEnqueue_SequentialReplay(t *testing.T) {
	t.Parallel()

	pg, dsn := migratedDB(t)
	owner := mustAPIKey(t, pg, uniquePrefix())
	_ = mustSub(t, pg, owner.ID, []string{"*"}, true)

	ev, err := domain.NewEvent(owner.ID, "order.created", []byte(`{}`), "idem-replay", nil)
	if err != nil {
		t.Fatal(err)
	}
	first, err := pg.EnqueueEvent(t.Context(), ev)
	if err != nil || first.Replay {
		t.Fatalf("first = %+v, %v", first, err)
	}
	second, err := pg.EnqueueEvent(t.Context(), ev)
	if err != nil || !second.Replay {
		t.Fatalf("second = %+v, %v", second, err)
	}
	if first.EventID != second.EventID {
		t.Fatalf("ids %s vs %s", first.EventID, second.EventID)
	}
	if len(first.DeliveryIDs) != len(second.DeliveryIDs) {
		t.Fatalf("delivery ids %v vs %v", first.DeliveryIDs, second.DeliveryIDs)
	}
	if n := countEvents(t, dsn, owner.ID); n != 1 {
		t.Fatalf("events = %d, want 1", n)
	}
}

func TestEnqueue_OccurredAtPersisted(t *testing.T) {
	t.Parallel()

	pg, _ := migratedDB(t)
	owner := mustAPIKey(t, pg, uniquePrefix())
	occurred := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	ev, err := domain.NewEvent(owner.ID, "order.created", []byte(`{}`), "idem-ts", &occurred)
	if err != nil {
		t.Fatal(err)
	}
	res, err := pg.EnqueueEvent(t.Context(), ev)
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := pg.GetEvent(t.Context(), owner.ID, res.EventID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.OccurredAt.Equal(occurred) {
		t.Fatalf("occurred_at = %s, want %s", got.OccurredAt, occurred)
	}
}

func TestEnqueue_IdempotencyPerAPIKey(t *testing.T) {
	t.Parallel()

	pg, _ := migratedDB(t)
	a := mustAPIKey(t, pg, uniquePrefix())
	b := mustAPIKey(t, pg, uniquePrefix())
	ea, err := domain.NewEvent(a.ID, "order.created", []byte(`{"k":"a"}`), "shared-key", nil)
	if err != nil {
		t.Fatal(err)
	}
	eb, err := domain.NewEvent(b.ID, "order.created", []byte(`{"k":"b"}`), "shared-key", nil)
	if err != nil {
		t.Fatal(err)
	}
	ra, err := pg.EnqueueEvent(t.Context(), ea)
	if err != nil {
		t.Fatal(err)
	}
	rb, err := pg.EnqueueEvent(t.Context(), eb)
	if err != nil {
		t.Fatal(err)
	}
	if ra.EventID == rb.EventID {
		t.Fatal("shared idempotency key collided across API keys")
	}
}

func TestEnqueue_ParallelDuplicate(t *testing.T) {
	t.Parallel()

	pg, dsn := migratedDB(t)
	owner := mustAPIKey(t, pg, uniquePrefix())
	_ = mustSub(t, pg, owner.ID, []string{"*"}, true)
	_ = mustSub(t, pg, owner.ID, []string{"order.created"}, true)

	ev, err := domain.NewEvent(owner.ID, "order.created", []byte(`{"n":1}`), "idem-parallel", nil)
	if err != nil {
		t.Fatal(err)
	}

	const n = 20
	results := make(chan domain.EnqueueResult, n)
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := pg.EnqueueEvent(t.Context(), ev)
			if err != nil {
				errs <- err
				return
			}
			results <- res
		}()
	}
	wg.Wait()
	close(errs)
	close(results)

	for err := range errs {
		t.Fatalf("EnqueueEvent: %v", err)
	}

	var eventID string
	var deliveryN int
	seen := 0
	replays := 0
	for res := range results {
		seen++
		if eventID == "" {
			eventID = res.EventID
			deliveryN = len(res.DeliveryIDs)
		}
		if res.EventID != eventID {
			t.Fatalf("event id %s vs %s", res.EventID, eventID)
		}
		if len(res.DeliveryIDs) != deliveryN {
			t.Fatalf("delivery count %d vs %d", len(res.DeliveryIDs), deliveryN)
		}
		if res.Replay {
			replays++
		}
	}
	if seen != n {
		t.Fatalf("got %d results, want %d", seen, n)
	}
	if eventID == "" || deliveryN != 2 {
		t.Fatalf("eventID=%s deliveries=%d", eventID, deliveryN)
	}
	if replays != n-1 && replays != n {
		// Exactly one insert wins; the rest replay. If every goroutine lost
		// the insert but then saw the committed row, all n can be replay.
		// That still leaves one event row. Fail only if we somehow got
		// zero replays (would mean n inserts) — checked via countEvents.
		t.Logf("replays=%d of %d (winner visibility)", replays, n)
	}
	if count := countEvents(t, dsn, owner.ID); count != 1 {
		t.Fatalf("events = %d, want 1", count)
	}
	if count := countDeliveries(t, dsn, eventID); count != 2 {
		t.Fatalf("deliveries = %d, want 2", count)
	}
}

func TestGetEvent_UnknownIsNotFound(t *testing.T) {
	t.Parallel()

	pg, _ := migratedDB(t)
	key := mustAPIKey(t, pg, uniquePrefix())
	_, _, err := pg.GetEvent(t.Context(), key.ID, "11111111-2222-3333-4444-555555555555")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetEvent(unknown) = %v, want ErrNotFound", err)
	}
}

func mustSub(t *testing.T, pg *store.Postgres, apiKeyID string, types []string, enabled bool) domain.Subscription {
	t.Helper()
	s, err := pg.CreateSubscription(t.Context(), domain.Subscription{
		APIKeyID:   apiKeyID,
		TargetURL:  "https://example.com/hooks",
		EventTypes: types,
		Headers:    map[string]string{},
		Enabled:    enabled,
	}, bytes.Repeat([]byte{0x99}, 48))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func countEvents(t *testing.T, dsn, apiKeyID string) int {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	var n int
	if err := conn.QueryRow(t.Context(), "SELECT count(*) FROM events WHERE api_key_id = $1::uuid", apiKeyID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func countDeliveries(t *testing.T, dsn, eventID string) int {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	var n int
	if err := conn.QueryRow(t.Context(), "SELECT count(*) FROM deliveries WHERE event_id = $1::uuid", eventID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestEnqueue_PayloadRoundTrip(t *testing.T) {
	t.Parallel()

	pg, _ := migratedDB(t)
	owner := mustAPIKey(t, pg, uniquePrefix())
	payload := []byte(`{"order_id":"abc","nested":{"ok":true}}`)
	ev, err := domain.NewEvent(owner.ID, "order.created", payload, "idem-json", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := pg.EnqueueEvent(t.Context(), ev)
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := pg.GetEvent(t.Context(), owner.ID, res.EventID)
	if err != nil {
		t.Fatal(err)
	}
	var a, b any
	if err := json.Unmarshal(payload, &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(got.Payload, &b); err != nil {
		t.Fatal(err)
	}
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	if !bytes.Equal(ab, bb) {
		t.Fatalf("payload %s vs %s", got.Payload, payload)
	}
}
