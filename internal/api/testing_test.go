package api

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/drkwv34/courier-outbox/internal/domain"
)

const testPepper = "0123456789abcdef0123456789abcdef"

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func testHasher(t *testing.T) domain.APIKeyHasher {
	t.Helper()
	h, err := domain.NewAPIKeyHasher([]byte(testPepper), 32)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// fakeKeys is an in-memory KeyLookup.
type fakeKeys struct {
	byPrefix map[string]domain.APIKey
	err      error
}

func (f *fakeKeys) APIKeyByPrefix(_ context.Context, prefix string) (domain.APIKey, error) {
	if f.err != nil {
		return domain.APIKey{}, f.err
	}
	k, ok := f.byPrefix[prefix]
	if !ok {
		return domain.APIKey{}, domain.ErrNotFound
	}
	return k, nil
}

// issue mints a key, stores it in f, and returns the raw key.
func (f *fakeKeys) issue(t *testing.T, h domain.APIKeyHasher, id string, revoked bool) string {
	t.Helper()
	g, err := h.Generate(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	k := domain.APIKey{ID: id, Name: id, Prefix: g.Prefix, Hash: g.Hash, CreatedAt: time.Now()}
	if revoked {
		now := time.Now()
		k.RevokedAt = &now
	}
	if f.byPrefix == nil {
		f.byPrefix = map[string]domain.APIKey{}
	}
	f.byPrefix[g.Prefix] = k
	return g.Raw
}

var errStoreDown = errors.New("connection refused")

func testEnvelope(t *testing.T) domain.Envelope {
	t.Helper()
	e, err := domain.NewEnvelope([]byte("local-dev-only-enc-key-32bytes!!"))
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func testPolicy() domain.URLPolicy {
	return domain.URLPolicy{AllowHTTP: false, ProtectSSRF: true}
}

type fakeSubs struct {
	mu   sync.Mutex
	seq  int
	byID map[string]domain.Subscription
	enc  map[string][]byte
	err  error
}

func (f *fakeSubs) CreateSubscription(_ context.Context, s domain.Subscription, secretEnc []byte) (domain.Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return domain.Subscription{}, f.err
	}
	f.seq++
	s.ID = fmt.Sprintf("11111111-2222-3333-4444-%012d", f.seq)
	now := time.Now().UTC()
	s.CreatedAt, s.UpdatedAt = now, now
	if f.byID == nil {
		f.byID = map[string]domain.Subscription{}
		f.enc = map[string][]byte{}
	}
	f.byID[s.ID] = s
	f.enc[s.ID] = append([]byte(nil), secretEnc...)
	return s, nil
}

func (f *fakeSubs) ListSubscriptions(_ context.Context, apiKeyID string) ([]domain.Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	out := make([]domain.Subscription, 0)
	for _, s := range f.byID {
		if s.APIKeyID == apiKeyID {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeSubs) GetSubscription(_ context.Context, apiKeyID, id string) (domain.Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return domain.Subscription{}, f.err
	}
	s, ok := f.byID[id]
	if !ok || s.APIKeyID != apiKeyID {
		return domain.Subscription{}, domain.ErrNotFound
	}
	return s, nil
}

func (f *fakeSubs) UpdateSubscription(_ context.Context, apiKeyID, id string, u domain.SubscriptionUpdate) (domain.Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return domain.Subscription{}, f.err
	}
	s, ok := f.byID[id]
	if !ok || s.APIKeyID != apiKeyID {
		return domain.Subscription{}, domain.ErrNotFound
	}
	if u.TargetURL != nil {
		s.TargetURL = *u.TargetURL
	}
	if u.EventTypes != nil {
		s.EventTypes = *u.EventTypes
	}
	if u.Headers != nil {
		s.Headers = *u.Headers
	}
	if u.Description != nil {
		s.Description = *u.Description
	}
	if u.Enabled != nil {
		s.Enabled = *u.Enabled
	}
	if len(u.SecretEnc) > 0 {
		f.enc[id] = append([]byte(nil), u.SecretEnc...)
	}
	s.UpdatedAt = time.Now().UTC()
	f.byID[id] = s
	return s, nil
}

func (f *fakeSubs) DisableSubscription(ctx context.Context, apiKeyID, id string) (domain.Subscription, error) {
	enabled := false
	return f.UpdateSubscription(ctx, apiKeyID, id, domain.SubscriptionUpdate{Enabled: &enabled})
}

func (f *fakeSubs) ciphertext(id string) []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]byte(nil), f.enc[id]...)
}

type fakeEvents struct {
	mu   sync.Mutex
	seq  int
	byID map[string]domain.Event
	dels map[string][]domain.Delivery
	keys map[string]string
	err  error
}

func (f *fakeEvents) EnqueueEvent(_ context.Context, e domain.Event) (domain.EnqueueResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return domain.EnqueueResult{}, f.err
	}
	if f.byID == nil {
		f.byID = map[string]domain.Event{}
		f.dels = map[string][]domain.Delivery{}
		f.keys = map[string]string{}
	}
	k := e.APIKeyID + "\x00" + e.IdempotencyKey
	if id, ok := f.keys[k]; ok {
		return domain.EnqueueResult{EventID: id, DeliveryIDs: deliveryIDs(f.dels[id]), Replay: true}, nil
	}
	f.seq++
	id := fmt.Sprintf("aaaaaaaa-bbbb-cccc-dddd-%012d", f.seq)
	now := time.Now().UTC()
	e.ID = id
	e.CreatedAt = now
	if e.OccurredAt.IsZero() {
		e.OccurredAt = now
	}
	del := domain.Delivery{
		ID:             fmt.Sprintf("bbbbbbbb-cccc-dddd-eeee-%012d", f.seq),
		EventID:        id,
		SubscriptionID: "11111111-2222-3333-4444-000000000001",
		Status:         domain.DeliveryPending,
		AttemptCount:   0,
		NextAttemptAt:  now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	f.byID[id] = e
	f.dels[id] = []domain.Delivery{del}
	f.keys[k] = id
	return domain.EnqueueResult{EventID: id, DeliveryIDs: []string{del.ID}, Replay: false}, nil
}

func (f *fakeEvents) GetEvent(_ context.Context, apiKeyID, id string) (domain.Event, []domain.Delivery, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return domain.Event{}, nil, f.err
	}
	e, ok := f.byID[id]
	if !ok || e.APIKeyID != apiKeyID {
		return domain.Event{}, nil, domain.ErrNotFound
	}
	return e, append([]domain.Delivery(nil), f.dels[id]...), nil
}

func deliveryIDs(dels []domain.Delivery) []string {
	out := make([]string, 0, len(dels))
	for _, d := range dels {
		out = append(out, d.ID)
	}
	return out
}

type fakeDeliveries struct {
	mu    sync.Mutex
	seq   int
	byID  map[string]domain.DeliveryView
	owner map[string]string
	atts  map[string][]domain.Attempt
	err   error
}

func (f *fakeDeliveries) seed(t *testing.T, apiKeyID string, status domain.DeliveryStatus, attempts int, eventType string) domain.DeliveryView {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.byID == nil {
		f.byID = map[string]domain.DeliveryView{}
		f.owner = map[string]string{}
		f.atts = map[string][]domain.Attempt{}
	}
	f.seq++
	now := time.Now().UTC()
	v := domain.DeliveryView{
		Delivery: domain.Delivery{
			ID:             fmt.Sprintf("cccccccc-dddd-eeee-ffff-%012d", f.seq),
			EventID:        fmt.Sprintf("aaaaaaaa-bbbb-cccc-dddd-%012d", f.seq),
			SubscriptionID: fmt.Sprintf("11111111-2222-3333-4444-%012d", f.seq),
			Status:         status,
			AttemptCount:   attempts,
			NextAttemptAt:  now,
			CreatedAt:      now.Add(-time.Duration(f.seq) * time.Minute),
			UpdatedAt:      now,
		},
		EventType: eventType,
	}
	f.byID[v.ID] = v
	f.owner[v.ID] = apiKeyID
	rows := make([]domain.Attempt, 0)
	if attempts > 0 {
		code := 500
		msg := "subscriber 500"
		rows = append(rows, domain.Attempt{DeliveryID: v.ID, StatusCode: &code, ErrorMessage: &msg, DurationMs: 11, RequestID: "r1", CreatedAt: now.Add(-2 * time.Second)})
		if attempts > 1 {
			code2 := 502
			msg2 := "subscriber 502"
			rows = append(rows, domain.Attempt{DeliveryID: v.ID, StatusCode: &code2, ErrorMessage: &msg2, DurationMs: 9, RequestID: "r2", CreatedAt: now.Add(-time.Second)})
		}
	}
	f.atts[v.ID] = rows
	return v
}

func (f *fakeDeliveries) ListDeliveries(_ context.Context, apiKeyID string, filter domain.DeliveryFilter) (domain.DeliveryPage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return domain.DeliveryPage{}, f.err
	}
	items := make([]domain.DeliveryView, 0)
	for _, v := range f.byID {
		if f.owner[v.ID] != apiKeyID {
			continue
		}
		if filter.Status != "" && v.Status != filter.Status {
			continue
		}
		if filter.SubscriptionID != "" && v.SubscriptionID != filter.SubscriptionID {
			continue
		}
		if filter.EventType != "" && v.EventType != filter.EventType {
			continue
		}
		if filter.CreatedAfter != nil && v.CreatedAt.Before(*filter.CreatedAfter) {
			continue
		}
		if filter.CreatedBefore != nil && !v.CreatedAt.Before(*filter.CreatedBefore) {
			continue
		}
		items = append(items, v)
	}
	limit := filter.LimitOrDefault()
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	total := len(items)
	if offset > len(items) {
		items = []domain.DeliveryView{}
	} else {
		items = items[offset:]
		if len(items) > limit {
			items = items[:limit]
		}
	}
	return domain.DeliveryPage{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}

func (f *fakeDeliveries) GetDelivery(_ context.Context, apiKeyID, id string) (domain.DeliveryView, []domain.Attempt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return domain.DeliveryView{}, nil, f.err
	}
	v, ok := f.byID[id]
	if !ok || f.owner[id] != apiKeyID {
		return domain.DeliveryView{}, nil, domain.ErrNotFound
	}
	return v, append([]domain.Attempt(nil), f.atts[id]...), nil
}

func (f *fakeDeliveries) ReplayDelivery(_ context.Context, apiKeyID, id string) (domain.DeliveryView, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return domain.DeliveryView{}, f.err
	}
	v, ok := f.byID[id]
	if !ok || f.owner[id] != apiKeyID {
		return domain.DeliveryView{}, domain.ErrNotFound
	}
	next, err := v.Replay()
	if err != nil {
		return domain.DeliveryView{}, err
	}
	next.NextAttemptAt = time.Now().UTC()
	v.Delivery = next
	f.byID[id] = v
	return v, nil
}
