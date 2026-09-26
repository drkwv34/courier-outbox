//go:build integration

package store_test

import (
	"bytes"
	"context"
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
