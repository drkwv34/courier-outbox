//go:build integration

package store_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/drkwv34/courier-outbox/internal/domain"
	"github.com/drkwv34/courier-outbox/internal/store"
	"github.com/drkwv34/courier-outbox/internal/store/storetest"
)

var pgServer, redisServer *storetest.Server

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctx := context.Background()
	var err error
	if pgServer, err = storetest.StartPostgres(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() { _ = pgServer.Terminate(ctx) }()
	if redisServer, err = storetest.StartRedis(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() { _ = redisServer.Terminate(ctx) }()
	return m.Run()
}

func migratedDB(t *testing.T) (*store.Postgres, string) {
	t.Helper()
	dsn := storetest.FreshDatabase(t, pgServer.URL)
	if _, err := store.Migrate(t.Context(), dsn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	pg, err := store.OpenPostgres(t.Context(), dsn)
	if err != nil {
		t.Fatalf("OpenPostgres: %v", err)
	}
	t.Cleanup(pg.Close)
	return pg, dsn
}

func TestMigrate_FreshDatabaseCreatesSchema(t *testing.T) {
	t.Parallel()

	dsn := storetest.FreshDatabase(t, pgServer.URL)
	applied, err := store.Migrate(t.Context(), dsn)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if want := []int64{1, 2, 3, 4, 5}; !slices.Equal(applied, want) {
		t.Fatalf("applied = %v, want %v", applied, want)
	}

	conn, err := pgx.Connect(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()

	for _, table := range []string{"api_keys", "subscriptions", "events", "deliveries", "delivery_attempts"} {
		var exists bool
		if err := conn.QueryRow(t.Context(), "SELECT to_regclass($1) IS NOT NULL", "public."+table).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Errorf("table %s missing after migrate", table)
		}
	}

	var uniqueIdem bool
	err = conn.QueryRow(t.Context(), `
		SELECT EXISTS (
			SELECT 1 FROM pg_constraint
			WHERE conname = 'events_api_key_id_idempotency_key_key' AND contype = 'u')`).Scan(&uniqueIdem)
	if err != nil || !uniqueIdem {
		t.Fatalf("UNIQUE (api_key_id, idempotency_key) on events missing (err=%v)", err)
	}
}

func TestMigrate_SecondRunIsNoop(t *testing.T) {
	t.Parallel()

	_, dsn := migratedDB(t)
	applied, err := store.Migrate(t.Context(), dsn)
	if err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if len(applied) != 0 {
		t.Fatalf("second Migrate applied %v, want nothing", applied)
	}
}

func TestMigrate_BadDSN(t *testing.T) {
	t.Parallel()

	if _, err := store.Migrate(t.Context(), "postgres://nobody:x@127.0.0.1:1/none?sslmode=disable&connect_timeout=1"); err == nil {
		t.Fatal("Migrate against unreachable db succeeded")
	}
}

func TestAPIKeys_CreateAndLookup(t *testing.T) {
	t.Parallel()

	pg, _ := migratedDB(t)
	hash := bytes.Repeat([]byte{0xab}, domain.APIKeyHashLen)

	created, err := pg.CreateAPIKey(t.Context(), "ci", "abcdefgh", hash)
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if created.ID == "" || created.CreatedAt.IsZero() || created.Revoked() {
		t.Fatalf("CreateAPIKey returned %+v", created)
	}

	got, err := pg.APIKeyByPrefix(t.Context(), "abcdefgh")
	if err != nil {
		t.Fatalf("APIKeyByPrefix: %v", err)
	}
	if got.ID != created.ID || got.Name != "ci" || !bytes.Equal(got.Hash, hash) {
		t.Fatalf("APIKeyByPrefix = %+v, want %+v", got, created)
	}
}

func TestAPIKeys_UnknownPrefixIsNotFound(t *testing.T) {
	t.Parallel()

	pg, _ := migratedDB(t)
	_, err := pg.APIKeyByPrefix(t.Context(), "zzzzzzzz")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("APIKeyByPrefix(unknown) error = %v, want ErrNotFound", err)
	}
}

func TestAPIKeys_DuplicatePrefixIsConflict(t *testing.T) {
	t.Parallel()

	pg, _ := migratedDB(t)
	hash := bytes.Repeat([]byte{0x01}, domain.APIKeyHashLen)
	if _, err := pg.CreateAPIKey(t.Context(), "a", "dupdupdu", hash); err != nil {
		t.Fatal(err)
	}
	_, err := pg.CreateAPIKey(t.Context(), "b", "dupdupdu", hash)
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("CreateAPIKey(dup prefix) error = %v, want ErrConflict", err)
	}
}

func TestAPIKeys_SchemaRejectsBadRows(t *testing.T) {
	t.Parallel()

	pg, _ := migratedDB(t)
	tests := []struct {
		name   string
		prefix string
		hash   []byte
	}{
		{name: "short hash", prefix: "abcdefgh", hash: []byte{1, 2, 3}},
		{name: "bad prefix alphabet", prefix: "ABCDEFGH", hash: bytes.Repeat([]byte{1}, 32)},
		{name: "short prefix", prefix: "abc", hash: bytes.Repeat([]byte{1}, 32)},
	}
	for _, tt := range tests {
		if _, err := pg.CreateAPIKey(t.Context(), "x", tt.prefix, tt.hash); err == nil {
			t.Errorf("%s: insert succeeded, want CHECK violation", tt.name)
		}
	}
}

func TestPing(t *testing.T) {
	t.Parallel()

	pg, _ := migratedDB(t)
	if err := pg.Ping(t.Context()); err != nil {
		t.Fatalf("Postgres.Ping: %v", err)
	}

	rdb, err := store.OpenRedis(redisServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rdb.Close() })
	if err := rdb.Ping(t.Context()); err != nil {
		t.Fatalf("Redis.Ping: %v", err)
	}
}

func TestPing_UnreachableFails(t *testing.T) {
	t.Parallel()

	pg, err := store.OpenPostgres(t.Context(), "postgres://x:y@127.0.0.1:1/none?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pg.Close)
	if err := pg.Ping(t.Context()); err == nil {
		t.Fatal("Postgres.Ping against closed port succeeded")
	}

	rdb, err := store.OpenRedis("redis://127.0.0.1:1/0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rdb.Close() })
	if err := rdb.Ping(t.Context()); err == nil {
		t.Fatal("Redis.Ping against closed port succeeded")
	}
}

func TestSubscriptions_CRUDAndIsolation(t *testing.T) {
	t.Parallel()

	pg, dsn := migratedDB(t)
	owner := mustAPIKey(t, pg, uniquePrefix())
	other := mustAPIKey(t, pg, uniquePrefix())

	plain := bytes.Repeat([]byte{0x42}, domain.SigningSecretBytes)
	created, err := pg.CreateSubscription(t.Context(), domain.Subscription{
		APIKeyID:    owner.ID,
		TargetURL:   "https://example.com/hooks",
		EventTypes:  []string{"*"},
		Headers:     map[string]string{"X-Shop": "acme"},
		Enabled:     true,
		Description: "primary",
	}, bytes.Repeat([]byte{0x99}, 48))
	if err != nil {
		t.Fatalf("CreateSubscription: %v", err)
	}
	if created.ID == "" || created.APIKeyID != owner.ID || !created.Enabled {
		t.Fatalf("created = %+v", created)
	}

	enc := secretEnc(t, dsn, created.ID)
	if bytes.Equal(enc, plain) || bytes.Contains(enc, []byte("example.com")) {
		t.Fatalf("stored blob looks like plaintext: %x", enc)
	}

	listed, err := pg.ListSubscriptions(t.Context(), owner.ID)
	if err != nil || len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("ListSubscriptions = %v, %v", listed, err)
	}
	empty, err := pg.ListSubscriptions(t.Context(), other.ID)
	if err != nil || len(empty) != 0 {
		t.Fatalf("other list = %v, %v, want empty", empty, err)
	}

	got, err := pg.GetSubscription(t.Context(), owner.ID, created.ID)
	if err != nil || got.TargetURL != "https://example.com/hooks" {
		t.Fatalf("GetSubscription = %+v, %v", got, err)
	}
	_, err = pg.GetSubscription(t.Context(), other.ID, created.ID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign get error = %v, want ErrNotFound", err)
	}

	desc := "updated"
	patched, err := pg.UpdateSubscription(t.Context(), owner.ID, created.ID, domain.SubscriptionUpdate{Description: &desc})
	if err != nil || patched.Description != "updated" || patched.TargetURL != created.TargetURL {
		t.Fatalf("UpdateSubscription = %+v, %v", patched, err)
	}
	_, err = pg.UpdateSubscription(t.Context(), other.ID, created.ID, domain.SubscriptionUpdate{Description: &desc})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign update error = %v, want ErrNotFound", err)
	}

	rotated, err := pg.UpdateSubscription(t.Context(), owner.ID, created.ID, domain.SubscriptionUpdate{SecretEnc: bytes.Repeat([]byte{0x11}, 48)})
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if bytes.Equal(secretEnc(t, dsn, rotated.ID), enc) {
		t.Fatal("rotate left ciphertext unchanged")
	}

	disabled, err := pg.DisableSubscription(t.Context(), owner.ID, created.ID)
	if err != nil || disabled.Enabled {
		t.Fatalf("DisableSubscription = %+v, %v", disabled, err)
	}
	again, err := pg.DisableSubscription(t.Context(), owner.ID, created.ID)
	if err != nil || again.Enabled {
		t.Fatalf("second disable = %+v, %v", again, err)
	}
	_, err = pg.DisableSubscription(t.Context(), other.ID, created.ID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign disable error = %v, want ErrNotFound", err)
	}
}

func TestSubscriptions_UnknownIsNotFound(t *testing.T) {
	t.Parallel()

	pg, _ := migratedDB(t)
	key := mustAPIKey(t, pg, uniquePrefix())
	_, err := pg.GetSubscription(t.Context(), key.ID, "11111111-2222-3333-4444-555555555555")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetSubscription(unknown) = %v, want ErrNotFound", err)
	}
}

func mustAPIKey(t *testing.T, pg *store.Postgres, prefix string) domain.APIKey {
	t.Helper()
	k, err := pg.CreateAPIKey(t.Context(), "it", prefix, bytes.Repeat([]byte{0xab}, domain.APIKeyHashLen))
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func uniquePrefix() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz234567"
	var b [8]byte
	_, _ = rand.Read(b[:])
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b[:])
}

func secretEnc(t *testing.T, dsn, id string) []byte {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	var enc []byte
	if err := conn.QueryRow(t.Context(), "SELECT signing_secret_enc FROM subscriptions WHERE id = $1::uuid", id).Scan(&enc); err != nil {
		t.Fatal(err)
	}
	return enc
}
