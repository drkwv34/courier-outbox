package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/drkwv34/courier-outbox/internal/domain"
)

type fakeCreator struct {
	name, prefix string
	hash         []byte
	err          error
}

func (f *fakeCreator) CreateAPIKey(_ context.Context, name, prefix string, hash []byte) (domain.APIKey, error) {
	if f.err != nil {
		return domain.APIKey{}, f.err
	}
	f.name, f.prefix, f.hash = name, prefix, hash
	return domain.APIKey{ID: "11111111-2222-3333-4444-555555555555", Name: name, Prefix: prefix, Hash: hash, CreatedAt: time.Now()}, nil
}

func testHasher(t *testing.T) domain.APIKeyHasher {
	t.Helper()
	h, err := domain.NewAPIKeyHasher([]byte("0123456789abcdef0123456789abcdef"), 32)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestCreateAPIKey_PrintsRawKeyOnceAndStoresOnlyHash(t *testing.T) {
	t.Parallel()

	h := testHasher(t)
	repo := &fakeCreator{}
	var out bytes.Buffer

	if err := createAPIKey(t.Context(), &out, repo, h, rand.Reader, "  ci  "); err != nil {
		t.Fatalf("createAPIKey: %v", err)
	}

	var raw string
	for _, line := range strings.Split(out.String(), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "key:"); ok {
			raw = strings.TrimSpace(v)
		}
	}
	if raw == "" {
		t.Fatalf("output has no key line:\n%s", out.String())
	}
	if strings.Count(out.String(), raw) != 1 {
		t.Fatal("raw key printed more than once")
	}
	if repo.name != "ci" {
		t.Fatalf("stored name = %q, want trimmed %q", repo.name, "ci")
	}
	prefix, err := domain.ParseAPIKeyPrefix(raw)
	if err != nil || prefix != repo.prefix {
		t.Fatalf("printed key prefix %q (err %v), stored prefix %q", prefix, err, repo.prefix)
	}
	if !h.Verify(raw, repo.hash) {
		t.Fatal("stored hash does not verify the printed key")
	}
	if bytes.Contains(repo.hash, []byte(raw)) {
		t.Fatal("stored hash contains the raw key")
	}
	if !strings.Contains(out.String(), "prefix: "+repo.prefix) || !strings.Contains(out.String(), "id:     11111111-") {
		t.Fatalf("output missing id or prefix:\n%s", out.String())
	}
}

func TestCreateAPIKey_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		keyName string
		repoErr error
		wantIs  error
	}{
		{name: "blank name", keyName: " ", wantIs: domain.ErrValidation},
		{name: "prefix collision", keyName: "ci", repoErr: fmt.Errorf("store: insert api key: %w", domain.ErrConflict), wantIs: domain.ErrConflict},
		{name: "store down", keyName: "ci", repoErr: errors.New("connection refused")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			err := createAPIKey(t.Context(), &out, &fakeCreator{err: tt.repoErr}, testHasher(t), rand.Reader, tt.keyName)
			if err == nil {
				t.Fatal("createAPIKey succeeded, want error")
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Fatalf("error = %v, want %v", err, tt.wantIs)
			}
			if out.Len() != 0 {
				t.Fatalf("printed output on failure: %q", out.String())
			}
		})
	}
}

func TestKeys_ArgumentValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{name: "no subcommand", args: nil},
		{name: "unknown subcommand", args: []string{"delete"}},
		{name: "missing name", args: []string{"create"}},
		{name: "empty name", args: []string{"create", "--name", ""}},
		{name: "unknown flag", args: []string{"create", "--nope"}},
		{name: "stray argument", args: []string{"create", "--name", "ci", "extra"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := keys(tt.args)
			if err == nil || !strings.Contains(err.Error(), "usage: courier keys create") {
				t.Fatalf("keys(%q) error = %v, want usage error", tt.args, err)
			}
		})
	}
}
