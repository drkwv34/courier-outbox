package api

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"log/slog"
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
