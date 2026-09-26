//go:build integration

package api_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/drkwv34/courier-outbox/internal/api"
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

type env struct {
	server *httptest.Server
	pg     *store.Postgres
	dsn    string
	hasher domain.APIKeyHasher
}

func newEnv(t *testing.T, redisURL string) env {
	t.Helper()
	dsn := storetest.FreshDatabase(t, pgServer.URL)
	if _, err := store.Migrate(t.Context(), dsn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	pg, err := store.OpenPostgres(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pg.Close)
	rdb, err := store.OpenRedis(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rdb.Close() })

	hasher, err := domain.NewAPIKeyHasher([]byte("integration-pepper-0123456789abcdef"), 32)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(api.NewRouter(api.Deps{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Keys:   pg,
		Hasher: hasher,
		Readiness: []api.ReadinessCheck{
			{Name: "postgres", Check: pg.Ping},
			{Name: "redis", Check: rdb.Ping},
		},
	}))
	t.Cleanup(srv.Close)
	return env{server: srv, pg: pg, dsn: dsn, hasher: hasher}
}

func (e env) issueKey(t *testing.T) string {
	t.Helper()
	gen, err := e.hasher.Generate(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.pg.CreateAPIKey(t.Context(), "it", gen.Prefix, gen.Hash); err != nil {
		t.Fatal(err)
	}
	return gen.Raw
}

func (e env) get(t *testing.T, path, bearer string) (int, map[string]string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, e.server.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return resp.StatusCode, body
}

func TestAuth_RealStore(t *testing.T) {
	t.Parallel()

	e := newEnv(t, redisServer.URL)
	valid := e.issueKey(t)

	otherPepper, err := domain.NewAPIKeyHasher([]byte("some-other-pepper-0123456789abcdef"), 32)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := otherPepper.Generate(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		bearer     string
		wantStatus int
		wantCode   string
	}{
		{name: "no key", bearer: "", wantStatus: http.StatusUnauthorized, wantCode: "unauthorized"},
		{name: "garbage", bearer: "hello", wantStatus: http.StatusUnauthorized, wantCode: "unauthorized"},
		{name: "well-formed unknown key", bearer: foreign.Raw, wantStatus: http.StatusUnauthorized, wantCode: "unauthorized"},
		{name: "valid key reaches empty v1", bearer: valid, wantStatus: http.StatusNotFound, wantCode: "not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := e.get(t, "/v1/subscriptions", tt.bearer)
			if status != tt.wantStatus || body["code"] != tt.wantCode {
				t.Fatalf("GET /v1/subscriptions = %d %v, want %d code %s", status, body, tt.wantStatus, tt.wantCode)
			}
		})
	}
}

func TestAuth_RevokedKeyRejected(t *testing.T) {
	t.Parallel()

	e := newEnv(t, redisServer.URL)
	raw := e.issueKey(t)
	if status, _ := e.get(t, "/v1/x", raw); status != http.StatusNotFound {
		t.Fatalf("before revoke: status %d, want 404", status)
	}

	prefix, err := domain.ParseAPIKeyPrefix(raw)
	if err != nil {
		t.Fatal(err)
	}
	revokeByPrefix(t, e, prefix)

	if status, body := e.get(t, "/v1/x", raw); status != http.StatusUnauthorized || body["code"] != "unauthorized" {
		t.Fatalf("after revoke: %d %v, want 401 unauthorized", status, body)
	}
}

func TestReadyz_RealDependencies(t *testing.T) {
	t.Parallel()

	up := newEnv(t, redisServer.URL)
	if status, body := up.get(t, "/readyz", ""); status != http.StatusOK || body["status"] != "ready" {
		t.Fatalf("readyz with deps up = %d %v, want 200 ready", status, body)
	}

	redisDown := newEnv(t, "redis://127.0.0.1:1/0")
	status, body := redisDown.get(t, "/readyz", "")
	if status != http.StatusServiceUnavailable || body["code"] != "unavailable" || body["message"] != "not ready: redis" {
		t.Fatalf("readyz with redis down = %d %v, want 503 naming redis", status, body)
	}
}

// revokeByPrefix sets revoked_at directly; key revocation has no CLI or API yet.
func revokeByPrefix(t *testing.T, e env, prefix string) {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), e.dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	tag, err := conn.Exec(t.Context(), "UPDATE api_keys SET revoked_at = now() WHERE prefix = $1", prefix)
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatalf("revoke: rows=%d err=%v", tag.RowsAffected(), err)
	}
}
