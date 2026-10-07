//go:build integration

package api_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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
	envelope, err := domain.NewEnvelope([]byte("local-dev-only-enc-key-32bytes!!"))
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
		Subscriptions: pg,
		URLPolicy:     domain.URLPolicy{AllowHTTP: false, ProtectSSRF: true},
		Envelope:      envelope,
		Rand:          rand.Reader,
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
			status, body := e.get(t, "/v1/events", tt.bearer)
			if status != tt.wantStatus || body["code"] != tt.wantCode {
				t.Fatalf("GET /v1/events = %d %v, want %d code %s", status, body, tt.wantStatus, tt.wantCode)
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

type subJSON struct {
	ID            string            `json:"id"`
	TargetURL     string            `json:"target_url"`
	EventTypes    []string          `json:"event_types"`
	Headers       map[string]string `json:"headers"`
	Enabled       bool              `json:"enabled"`
	Description   string            `json:"description"`
	SigningSecret string            `json:"signing_secret"`
}

func TestSubscriptions_Lifecycle(t *testing.T) {
	t.Parallel()

	e := newEnv(t, redisServer.URL)
	a := e.issueKey(t)
	b := e.issueKey(t)

	status, raw := e.do(t, http.MethodPost, "/v1/subscriptions", a, `{"target_url":"https://example.com/hooks","description":"primary"}`)
	if status != http.StatusCreated {
		t.Fatalf("create = %d %s", status, raw)
	}
	var created subJSON
	if err := json.Unmarshal(raw, &created); err != nil {
		t.Fatal(err)
	}
	if created.SigningSecret == "" || created.ID == "" {
		t.Fatalf("create body = %+v", created)
	}
	secretBytes, err := base64.StdEncoding.DecodeString(created.SigningSecret)
	if err != nil || len(secretBytes) != domain.SigningSecretBytes {
		t.Fatalf("secret decode: %v len %d", err, len(secretBytes))
	}
	enc := storedSecret(t, e.dsn, created.ID)
	if bytes.Equal(enc, secretBytes) || bytes.Contains(enc, []byte(created.SigningSecret)) {
		t.Fatal("database stored plaintext signing secret")
	}

	status, raw = e.do(t, http.MethodGet, "/v1/subscriptions/"+created.ID, a, "")
	if status != http.StatusOK {
		t.Fatalf("get = %d %s", status, raw)
	}
	if strings.Contains(string(raw), "signing_secret") {
		t.Fatalf("GET re-exposed secret: %s", raw)
	}

	status, raw = e.do(t, http.MethodGet, "/v1/subscriptions", a, "")
	if status != http.StatusOK {
		t.Fatalf("list = %d %s", status, raw)
	}
	var listed struct {
		Items []subJSON `json:"items"`
	}
	if err := json.Unmarshal(raw, &listed); err != nil || len(listed.Items) != 1 || listed.Items[0].SigningSecret != "" {
		t.Fatalf("list body = %s err %v", raw, err)
	}

	status, raw = e.do(t, http.MethodPatch, "/v1/subscriptions/"+created.ID, a, `{"description":"updated"}`)
	if status != http.StatusOK {
		t.Fatalf("patch = %d %s", status, raw)
	}
	var patched subJSON
	if err := json.Unmarshal(raw, &patched); err != nil || patched.Description != "updated" || patched.SigningSecret != "" {
		t.Fatalf("patch body = %s", raw)
	}

	status, raw = e.do(t, http.MethodPatch, "/v1/subscriptions/"+created.ID, a, `{"rotate_secret":true}`)
	var rotated subJSON
	if err := json.Unmarshal(raw, &rotated); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || rotated.SigningSecret == "" || rotated.SigningSecret == created.SigningSecret {
		t.Fatalf("rotate = %d %s", status, raw)
	}

	status, raw = e.do(t, http.MethodPost, "/v1/subscriptions/"+created.ID+"/disable", a, "")
	var disabled subJSON
	if err := json.Unmarshal(raw, &disabled); err != nil || status != http.StatusOK || disabled.Enabled {
		t.Fatalf("disable = %d %s", status, raw)
	}

	status, _ = e.do(t, http.MethodGet, "/v1/subscriptions/"+created.ID, b, "")
	if status != http.StatusNotFound {
		t.Fatalf("foreign get = %d, want 404", status)
	}
	status, _ = e.do(t, http.MethodPatch, "/v1/subscriptions/"+created.ID, b, `{"description":"nope"}`)
	if status != http.StatusNotFound {
		t.Fatalf("foreign patch = %d, want 404", status)
	}
	status, _ = e.do(t, http.MethodPost, "/v1/subscriptions/"+created.ID+"/disable", b, "")
	if status != http.StatusNotFound {
		t.Fatalf("foreign disable = %d, want 404", status)
	}
	status, raw = e.do(t, http.MethodGet, "/v1/subscriptions", b, "")
	var otherList struct {
		Items []subJSON `json:"items"`
	}
	if err := json.Unmarshal(raw, &otherList); err != nil || status != http.StatusOK || len(otherList.Items) != 0 {
		t.Fatalf("foreign list = %d %s", status, raw)
	}
}

func (e env) do(t *testing.T, method, path, bearer, body string) (int, []byte) {
	t.Helper()
	var rdr io.Reader = http.NoBody
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, e.server.URL+path, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, raw
}

func storedSecret(t *testing.T, dsn, id string) []byte {
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
