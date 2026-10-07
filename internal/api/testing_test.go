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
