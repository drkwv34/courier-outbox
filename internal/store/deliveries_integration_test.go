//go:build integration

package store_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/drkwv34/courier-outbox/internal/domain"
	"github.com/drkwv34/courier-outbox/internal/store"
)

func TestClaimDue_DueVsFutureVsLeased(t *testing.T) {
	t.Parallel()

	pg, dsn := migratedDB(t)
	owner := mustAPIKey(t, pg, uniquePrefix())
	sub := mustSub(t, pg, owner.ID, []string{"*"}, true)
	ev := mustEnqueue(t, pg, owner.ID, "due-1")
	if len(ev.DeliveryIDs) != 1 {
		t.Fatalf("deliveries = %v", ev.DeliveryIDs)
	}

	first, err := pg.ClaimDue(t.Context(), "w1", 30*time.Second, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || first[0].Delivery.ID != ev.DeliveryIDs[0] || first[0].Delivery.LeaseOwner != "w1" {
		t.Fatalf("first claim = %+v", first)
	}
	if !bytes.Contains(first[0].Payload, []byte(`"n"`)) || first[0].TargetURL == "" || len(first[0].SigningSecretEnc) == 0 {
		t.Fatalf("claim payload/url/secret missing: %+v", first[0])
	}
	if first[0].Delivery.SubscriptionID != sub.ID {
		t.Fatalf("sub %s vs %s", first[0].Delivery.SubscriptionID, sub.ID)
	}

	second, err := pg.ClaimDue(t.Context(), "w2", 30*time.Second, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 0 {
		t.Fatalf("second claim during lease = %+v", second)
	}

	pushNextAttempt(t, dsn, ev.DeliveryIDs[0], time.Hour)
	expireLease(t, dsn, ev.DeliveryIDs[0])
	third, err := pg.ClaimDue(t.Context(), "w3", 30*time.Second, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(third) != 0 {
		t.Fatalf("future next_attempt claimed = %+v", third)
	}
}

func TestClaimDue_SkipsDisabled(t *testing.T) {
	t.Parallel()

	pg, _ := migratedDB(t)
	owner := mustAPIKey(t, pg, uniquePrefix())
	_ = mustSub(t, pg, owner.ID, []string{"*"}, false)
	_ = mustEnqueue(t, pg, owner.ID, "disabled-1")

	got, err := pg.ClaimDue(t.Context(), "w1", 30*time.Second, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("claimed disabled = %+v", got)
	}
}

func TestRecordAttempt_SuccessFailureDeadLetter(t *testing.T) {
	t.Parallel()

	pg, dsn := migratedDB(t)
	owner := mustAPIKey(t, pg, uniquePrefix())
	_ = mustSub(t, pg, owner.ID, []string{"*"}, true)
	ev := mustEnqueue(t, pg, owner.ID, "rec-1")

	claimed, err := pg.ClaimDue(t.Context(), "w1", 30*time.Second, 1)
	if err != nil || len(claimed) != 1 || claimed[0].Delivery.ID != ev.DeliveryIDs[0] {
		t.Fatalf("claim = %v %v", claimed, err)
	}
	code500 := 500
	msg := "subscriber 500"
	next, delay, err := claimed[0].Delivery.RecordFailure(domain.DefaultBackoff)
	if err != nil {
		t.Fatal(err)
	}
	if err := pg.RecordAttempt(t.Context(), domain.AttemptRecord{
		DeliveryID:   next.ID,
		WorkerID:     "w1",
		Status:       next.Status,
		AttemptCount: next.AttemptCount,
		RetryDelay:   delay,
		StatusCode:   &code500,
		ErrorMessage: &msg,
		DurationMs:   12,
		RequestID:    "req-1",
	}); err != nil {
		t.Fatal(err)
	}

	st, count, ownerLease := deliveryState(t, dsn, next.ID)
	if st != string(domain.DeliveryRetrying) || count != 1 || ownerLease != "" {
		t.Fatalf("after fail status=%s count=%d lease=%q", st, count, ownerLease)
	}
	if n := countAttempts(t, dsn, next.ID); n != 1 {
		t.Fatalf("attempts = %d", n)
	}

	// Make it due again and succeed.
	pushNextAttempt(t, dsn, next.ID, -time.Second)
	claimed, err = pg.ClaimDue(t.Context(), "w1", 30*time.Second, 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("reclaim = %v %v", claimed, err)
	}
	ok := 200
	succ, err := claimed[0].Delivery.RecordSuccess()
	if err != nil {
		t.Fatal(err)
	}
	if err := pg.RecordAttempt(t.Context(), domain.AttemptRecord{
		DeliveryID:   succ.ID,
		WorkerID:     "w1",
		Status:       succ.Status,
		AttemptCount: succ.AttemptCount,
		StatusCode:   &ok,
		DurationMs:   4,
		RequestID:    "req-2",
	}); err != nil {
		t.Fatal(err)
	}
	st, count, ownerLease = deliveryState(t, dsn, succ.ID)
	if st != string(domain.DeliveryDelivered) || count != 2 || ownerLease != "" {
		t.Fatalf("after success status=%s count=%d lease=%q", st, count, ownerLease)
	}
	if !deliveredAtSet(t, dsn, succ.ID) {
		t.Fatal("delivered_at not set")
	}
	if n := countAttempts(t, dsn, succ.ID); n != 2 {
		t.Fatalf("attempts = %d, want 2", n)
	}
}

func TestRecordAttempt_DeadLetterAtSix(t *testing.T) {
	t.Parallel()

	pg, dsn := migratedDB(t)
	owner := mustAPIKey(t, pg, uniquePrefix())
	_ = mustSub(t, pg, owner.ID, []string{"*"}, true)
	ev := mustEnqueue(t, pg, owner.ID, "dlq-1")

	code := 500
	curAttempt := 0
	status := domain.DeliveryPending
	for i := 0; i < domain.MaxAttempts; i++ {
		pushNextAttempt(t, dsn, ev.DeliveryIDs[0], -time.Second)
		expireLease(t, dsn, ev.DeliveryIDs[0])
		claimed, err := pg.ClaimDue(t.Context(), "w1", 30*time.Second, 1)
		if err != nil || len(claimed) != 1 {
			t.Fatalf("claim %d = %v %v", i, claimed, err)
		}
		claimed[0].Delivery.Status = status
		claimed[0].Delivery.AttemptCount = curAttempt
		next, delay, err := claimed[0].Delivery.RecordFailure(domain.DefaultBackoff)
		if err != nil {
			t.Fatal(err)
		}
		if err := pg.RecordAttempt(t.Context(), domain.AttemptRecord{
			DeliveryID:   next.ID,
			WorkerID:     "w1",
			Status:       next.Status,
			AttemptCount: next.AttemptCount,
			RetryDelay:   delay,
			StatusCode:   &code,
			DurationMs:   1,
			RequestID:    "r",
		}); err != nil {
			t.Fatal(err)
		}
		status = next.Status
		curAttempt = next.AttemptCount
	}
	st, count, _ := deliveryState(t, dsn, ev.DeliveryIDs[0])
	if st != string(domain.DeliveryDeadLettered) || count != domain.MaxAttempts {
		t.Fatalf("status=%s count=%d", st, count)
	}
}

func TestRecordAttempt_LeaseLostStillLogs(t *testing.T) {
	t.Parallel()

	pg, dsn := migratedDB(t)
	owner := mustAPIKey(t, pg, uniquePrefix())
	_ = mustSub(t, pg, owner.ID, []string{"*"}, true)
	ev := mustEnqueue(t, pg, owner.ID, "lost-1")
	claimed, err := pg.ClaimDue(t.Context(), "w1", 30*time.Second, 1)
	if err != nil || len(claimed) != 1 || claimed[0].Delivery.ID != ev.DeliveryIDs[0] {
		t.Fatal(err)
	}
	expireLease(t, dsn, claimed[0].Delivery.ID)
	_, err = pg.ClaimDue(t.Context(), "w2", 30*time.Second, 1)
	if err != nil {
		t.Fatal(err)
	}

	code := 200
	next, err := claimed[0].Delivery.RecordSuccess()
	if err != nil {
		t.Fatal(err)
	}
	err = pg.RecordAttempt(t.Context(), domain.AttemptRecord{
		DeliveryID:   next.ID,
		WorkerID:     "w1",
		Status:       next.Status,
		AttemptCount: next.AttemptCount,
		StatusCode:   &code,
		DurationMs:   1,
		RequestID:    "stale",
	})
	if !errors.Is(err, domain.ErrLeaseLost) {
		t.Fatalf("err = %v, want ErrLeaseLost", err)
	}
	if n := countAttempts(t, dsn, next.ID); n != 1 {
		t.Fatalf("attempts = %d, want 1 (audit row kept)", n)
	}
}

func TestClaimDue_LeaseReclaimAfterExpiry(t *testing.T) {
	t.Parallel()

	pg, dsn := migratedDB(t)
	owner := mustAPIKey(t, pg, uniquePrefix())
	_ = mustSub(t, pg, owner.ID, []string{"*"}, true)
	ev := mustEnqueue(t, pg, owner.ID, "reclaim-1")

	first, err := pg.ClaimDue(t.Context(), "crashed", 30*time.Second, 1)
	if err != nil || len(first) != 1 || first[0].Delivery.ID != ev.DeliveryIDs[0] {
		t.Fatalf("first = %v %v", first, err)
	}
	expireLease(t, dsn, first[0].Delivery.ID)

	second, err := pg.ClaimDue(t.Context(), "survivor", 30*time.Second, 1)
	if err != nil || len(second) != 1 {
		t.Fatalf("reclaim = %v %v", second, err)
	}
	if second[0].Delivery.LeaseOwner != "survivor" {
		t.Fatalf("owner = %q", second[0].Delivery.LeaseOwner)
	}
}

func TestClaimDue_ConcurrentWorkersSkipLocked(t *testing.T) {
	t.Parallel()

	pg, _ := migratedDB(t)
	owner := mustAPIKey(t, pg, uniquePrefix())
	_ = mustSub(t, pg, owner.ID, []string{"*"}, true)
	ev := mustEnqueue(t, pg, owner.ID, "race-1")
	if len(ev.DeliveryIDs) != 1 {
		t.Fatalf("want 1 delivery, got %v", ev.DeliveryIDs)
	}

	const n = 8
	type result struct {
		id  string
		n   int
		err error
	}
	ch := make(chan result, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		id := "w-" + string(rune('a'+i))
		go func(worker string) {
			defer wg.Done()
			got, err := pg.ClaimDue(t.Context(), worker, 30*time.Second, 8)
			ch <- result{id: worker, n: len(got), err: err}
		}(id)
	}
	wg.Wait()
	close(ch)

	winners := 0
	for r := range ch {
		if r.err != nil {
			t.Fatalf("%s: %v", r.id, r.err)
		}
		if r.n > 1 {
			t.Fatalf("%s claimed %d", r.id, r.n)
		}
		if r.n == 1 {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("winners = %d, want 1", winners)
	}
}

func mustEnqueue(t *testing.T, pg *store.Postgres, apiKeyID, idem string) domain.EnqueueResult {
	t.Helper()
	ev, err := domain.NewEvent(apiKeyID, "order.created", []byte(`{"n":1}`), idem, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := pg.EnqueueEvent(t.Context(), ev)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func expireLease(t *testing.T, dsn, deliveryID string) {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(t.Context()) }()
	if _, err := conn.Exec(t.Context(), `UPDATE deliveries SET lease_owner = COALESCE(lease_owner, 'expired'), lease_until = now() - interval '1 second' WHERE id = $1::uuid`, deliveryID); err != nil {
		t.Fatal(err)
	}
}

func pushNextAttempt(t *testing.T, dsn, deliveryID string, delta time.Duration) {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(t.Context()) }()
	if _, err := conn.Exec(t.Context(), `UPDATE deliveries SET next_attempt_at = now() + ($2::double precision * interval '1 second') WHERE id = $1::uuid`, deliveryID, delta.Seconds()); err != nil {
		t.Fatal(err)
	}
}

func deliveryState(t *testing.T, dsn, id string) (status string, attemptCount int, leaseOwner string) {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(t.Context()) }()
	var owner *string
	if err := conn.QueryRow(t.Context(), `SELECT status, attempt_count, lease_owner FROM deliveries WHERE id = $1::uuid`, id).Scan(&status, &attemptCount, &owner); err != nil {
		t.Fatal(err)
	}
	if owner != nil {
		leaseOwner = *owner
	}
	return status, attemptCount, leaseOwner
}

func deliveredAtSet(t *testing.T, dsn, id string) bool {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(t.Context()) }()
	var ts *time.Time
	if err := conn.QueryRow(t.Context(), `SELECT delivered_at FROM deliveries WHERE id = $1::uuid`, id).Scan(&ts); err != nil {
		t.Fatal(err)
	}
	return ts != nil
}

func countAttempts(t *testing.T, dsn, deliveryID string) int {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(t.Context()) }()
	var n int
	if err := conn.QueryRow(t.Context(), `SELECT count(*) FROM delivery_attempts WHERE delivery_id = $1::uuid`, deliveryID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestListDeliveries_FiltersPaginationIsolation(t *testing.T) {
	t.Parallel()

	pg, dsn := migratedDB(t)
	owner := mustAPIKey(t, pg, uniquePrefix())
	other := mustAPIKey(t, pg, uniquePrefix())
	subA := mustSub(t, pg, owner.ID, []string{"order.created"}, true)
	subB := mustSub(t, pg, owner.ID, []string{"order.paid"}, true)
	_ = mustSub(t, pg, other.ID, []string{"*"}, true)

	first := mustEnqueueType(t, pg, owner.ID, "order.created", "list-1")
	second := mustEnqueueType(t, pg, owner.ID, "order.paid", "list-2")
	_ = mustEnqueueType(t, pg, owner.ID, "order.created", "list-3")
	foreign := mustEnqueueType(t, pg, other.ID, "order.created", "list-x")

	forceDeliveryStatus(t, dsn, first.DeliveryIDs[0], string(domain.DeliveryDeadLettered))
	forceDeliveryStatus(t, dsn, second.DeliveryIDs[0], string(domain.DeliveryDeadLettered))

	page, err := pg.ListDeliveries(t.Context(), owner.ID, domain.DeliveryFilter{Status: domain.DeliveryDeadLettered})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("dead letters = %+v", page)
	}
	for _, item := range page.Items {
		if item.Status != domain.DeliveryDeadLettered || item.ID == foreign.DeliveryIDs[0] {
			t.Fatalf("item = %+v", item)
		}
	}

	bySub, err := pg.ListDeliveries(t.Context(), owner.ID, domain.DeliveryFilter{SubscriptionID: subA.ID})
	if err != nil {
		t.Fatal(err)
	}
	if bySub.Total != 2 {
		t.Fatalf("subscription filter total = %d, want 2", bySub.Total)
	}
	for _, item := range bySub.Items {
		if item.SubscriptionID != subA.ID {
			t.Fatalf("wrong sub %s", item.SubscriptionID)
		}
	}
	if second.DeliveryIDs[0] == "" || subB.ID == "" {
		t.Fatal("expected subB delivery")
	}

	byType, err := pg.ListDeliveries(t.Context(), owner.ID, domain.DeliveryFilter{EventType: "order.paid"})
	if err != nil || byType.Total != 1 || byType.Items[0].EventType != "order.paid" {
		t.Fatalf("type filter = %+v %v", byType, err)
	}

	future := time.Now().UTC().Add(time.Hour)
	empty, err := pg.ListDeliveries(t.Context(), owner.ID, domain.DeliveryFilter{CreatedAfter: &future})
	if err != nil || empty.Total != 0 || len(empty.Items) != 0 {
		t.Fatalf("future after = %+v %v", empty, err)
	}

	page1, err := pg.ListDeliveries(t.Context(), owner.ID, domain.DeliveryFilter{Limit: 2, Offset: 0})
	if err != nil || page1.Total != 3 || len(page1.Items) != 2 {
		t.Fatalf("page1 = %+v %v", page1, err)
	}
	page2, err := pg.ListDeliveries(t.Context(), owner.ID, domain.DeliveryFilter{Limit: 2, Offset: 2})
	if err != nil || page2.Total != 3 || len(page2.Items) != 1 {
		t.Fatalf("page2 = %+v %v", page2, err)
	}
	ids := map[string]struct{}{}
	for _, item := range append(page1.Items, page2.Items...) {
		if _, ok := ids[item.ID]; ok {
			t.Fatalf("duplicate id %s", item.ID)
		}
		ids[item.ID] = struct{}{}
	}

	otherPage, err := pg.ListDeliveries(t.Context(), other.ID, domain.DeliveryFilter{})
	if err != nil || otherPage.Total != 1 || otherPage.Items[0].ID != foreign.DeliveryIDs[0] {
		t.Fatalf("other list = %+v %v", otherPage, err)
	}
}

func TestGetDelivery_AttemptsOldestFirst(t *testing.T) {
	t.Parallel()

	pg, dsn := migratedDB(t)
	owner := mustAPIKey(t, pg, uniquePrefix())
	other := mustAPIKey(t, pg, uniquePrefix())
	_ = mustSub(t, pg, owner.ID, []string{"*"}, true)
	ev := mustEnqueue(t, pg, owner.ID, "get-1")
	id := ev.DeliveryIDs[0]

	claimed, err := pg.ClaimDue(t.Context(), "w1", 30*time.Second, 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim = %v %v", claimed, err)
	}
	code500 := 500
	next, delay, err := claimed[0].Delivery.RecordFailure(domain.DefaultBackoff)
	if err != nil {
		t.Fatal(err)
	}
	if err := pg.RecordAttempt(t.Context(), domain.AttemptRecord{
		DeliveryID: next.ID, WorkerID: "w1", Status: next.Status, AttemptCount: next.AttemptCount,
		RetryDelay: delay, StatusCode: &code500, ErrorMessage: ptrStr("subscriber 500"), DurationMs: 12, RequestID: "r1",
	}); err != nil {
		t.Fatal(err)
	}
	pushNextAttempt(t, dsn, id, -time.Hour)
	claimed, err = pg.ClaimDue(t.Context(), "w1", 30*time.Second, 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("second claim = %v %v", claimed, err)
	}
	next, delay, err = claimed[0].Delivery.RecordFailure(domain.DefaultBackoff)
	if err != nil {
		t.Fatal(err)
	}
	code502 := 502
	if err := pg.RecordAttempt(t.Context(), domain.AttemptRecord{
		DeliveryID: next.ID, WorkerID: "w1", Status: next.Status, AttemptCount: next.AttemptCount,
		RetryDelay: delay, StatusCode: &code502, ErrorMessage: ptrStr("subscriber 502"), DurationMs: 8, RequestID: "r2",
	}); err != nil {
		t.Fatal(err)
	}

	view, attempts, err := pg.GetDelivery(t.Context(), owner.ID, id)
	if err != nil {
		t.Fatal(err)
	}
	if view.ID != id || view.EventType != "order.created" || len(attempts) != 2 {
		t.Fatalf("view = %+v attempts=%d", view, len(attempts))
	}
	if attempts[0].StatusCode == nil || *attempts[0].StatusCode != 500 || attempts[1].StatusCode == nil || *attempts[1].StatusCode != 502 {
		t.Fatalf("attempt order = %+v", attempts)
	}

	_, _, err = pg.GetDelivery(t.Context(), other.ID, id)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign get = %v", err)
	}
	_, _, err = pg.GetDelivery(t.Context(), owner.ID, "11111111-2222-3333-4444-555555555555")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing get = %v", err)
	}
}

func TestReplayDelivery_DeadLetterOnly(t *testing.T) {
	t.Parallel()

	pg, dsn := migratedDB(t)
	owner := mustAPIKey(t, pg, uniquePrefix())
	other := mustAPIKey(t, pg, uniquePrefix())
	_ = mustSub(t, pg, owner.ID, []string{"*"}, true)
	ev := mustEnqueue(t, pg, owner.ID, "rep-1")
	id := ev.DeliveryIDs[0]

	_, err := pg.ReplayDelivery(t.Context(), owner.ID, id)
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("replay pending = %v", err)
	}

	forceDeliveryStatus(t, dsn, id, string(domain.DeliveryDeadLettered))
	before := countAttempts(t, dsn, id)
	view, err := pg.ReplayDelivery(t.Context(), owner.ID, id)
	if err != nil {
		t.Fatal(err)
	}
	if view.Status != domain.DeliveryPending || view.AttemptCount != 0 || view.LeaseOwner != "" {
		t.Fatalf("replayed = %+v", view)
	}
	if countAttempts(t, dsn, id) != before {
		t.Fatal("replay deleted attempts")
	}
	st, _, lease := deliveryState(t, dsn, id)
	if st != string(domain.DeliveryPending) || lease != "" {
		t.Fatalf("state %s lease %q", st, lease)
	}

	_, err = pg.ReplayDelivery(t.Context(), other.ID, id)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign replay = %v", err)
	}
	_, err = pg.ReplayDelivery(t.Context(), owner.ID, "11111111-2222-3333-4444-555555555555")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing replay = %v", err)
	}
}

func mustEnqueueType(t *testing.T, pg *store.Postgres, apiKeyID, eventType, idem string) domain.EnqueueResult {
	t.Helper()
	ev, err := domain.NewEvent(apiKeyID, eventType, []byte(`{"n":1}`), idem, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := pg.EnqueueEvent(t.Context(), ev)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func forceDeliveryStatus(t *testing.T, dsn, id, status string) {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(t.Context()) }()
	tag, err := conn.Exec(t.Context(), `
		UPDATE deliveries
		SET status = $2,
		    delivered_at = CASE WHEN $2 = 'delivered' THEN now() ELSE NULL END,
		    lease_owner = NULL,
		    lease_until = NULL,
		    updated_at = now()
		WHERE id = $1::uuid`, id, status)
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatalf("force status: rows=%d err=%v", tag.RowsAffected(), err)
	}
}

func ptrStr(s string) *string { return &s }

func TestClaimDue_ReturnsHeadersJSON(t *testing.T) {
	t.Parallel()

	pg, _ := migratedDB(t)
	owner := mustAPIKey(t, pg, uniquePrefix())
	_, err := pg.CreateSubscription(t.Context(), domain.Subscription{
		APIKeyID:   owner.ID,
		TargetURL:  "https://example.com/hooks",
		EventTypes: []string{"*"},
		Headers:    map[string]string{"X-Shop": "acme"},
		Enabled:    true,
	}, bytes.Repeat([]byte{0x99}, 48))
	if err != nil {
		t.Fatal(err)
	}
	_ = mustEnqueue(t, pg, owner.ID, "hdr-1")
	got, err := pg.ClaimDue(t.Context(), "w1", 30*time.Second, 1)
	if err != nil || len(got) != 1 {
		t.Fatalf("%v %v", got, err)
	}
	if got[0].Headers["X-Shop"] != "acme" {
		raw, _ := json.Marshal(got[0].Headers)
		t.Fatalf("headers = %s", raw)
	}
}
