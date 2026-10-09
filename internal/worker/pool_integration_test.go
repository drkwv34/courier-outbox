//go:build integration

package worker_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/drkwv34/courier-outbox/internal/domain"
	"github.com/drkwv34/courier-outbox/internal/sign"
	"github.com/drkwv34/courier-outbox/internal/store"
	"github.com/drkwv34/courier-outbox/internal/store/storetest"
	"github.com/drkwv34/courier-outbox/internal/worker"
)

var pgServer *storetest.Server

func TestMain(m *testing.M) {
	ctx := context.Background()
	var err error
	pgServer, err = storetest.StartPostgres(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	_ = pgServer.Terminate(ctx)
	os.Exit(code)
}

func migrated(t *testing.T) (*store.Postgres, string) {
	t.Helper()
	dsn := storetest.FreshDatabase(t, pgServer.URL)
	if _, err := store.Migrate(t.Context(), dsn); err != nil {
		t.Fatal(err)
	}
	pg, err := store.OpenPostgres(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pg.Close)
	return pg, dsn
}

func TestWorker_RetryThenDeliver(t *testing.T) {
	t.Parallel()

	pg, dsn := migrated(t)
	var hits atomic.Int32
	plain := []byte("whsec_integration_secret______")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		ts, _ := strconv.ParseInt(r.Header.Get(sign.HeaderTimestamp), 10, 64)
		if err := sign.Verify(plain, ts, body, r.Header.Get(sign.HeaderSignature), time.Now()); err != nil {
			t.Errorf("signature: %v", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		n := hits.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	seed := seedDelivery(t, pg, srv.URL, plain, `{"order_id":"1"}`)
	logs := startPool(t, pg, "w-retry", 10*time.Millisecond)
	waitStatus(t, dsn, seed.deliveryID, domain.DeliveryDelivered)

	if hits.Load() != 2 {
		t.Fatalf("hits = %d, want 2", hits.Load())
	}
	if n := attemptCount(t, dsn, seed.deliveryID); n != 2 {
		t.Fatalf("attempts = %d, want 2", n)
	}
	raw := logs.String()
	if !strings.Contains(raw, seed.deliveryID) || !strings.Contains(raw, seed.eventID) || !strings.Contains(raw, `"attempt"`) {
		t.Fatalf("logs missing ids: %s", raw)
	}
	if strings.Contains(raw, string(plain)) || strings.Contains(raw, "order_id") {
		t.Fatalf("logs leaked secret or payload")
	}
}

func TestWorker_SlowMockFailsAttempt(t *testing.T) {
	t.Parallel()

	pg, dsn := migrated(t)
	plain := []byte("whsec_slow_secret_____________")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(6 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	seed := seedDelivery(t, pg, srv.URL, plain, `{"n":1}`)
	_ = startPool(t, pg, "w-slow", 10*time.Millisecond)
	waitStatus(t, dsn, seed.deliveryID, domain.DeliveryRetrying)
	if n := attemptCount(t, dsn, seed.deliveryID); n != 1 {
		t.Fatalf("attempts = %d", n)
	}
}

func TestWorker_LeaseReclaimAfterCrash(t *testing.T) {
	t.Parallel()

	pg, dsn := migrated(t)
	plain := []byte("whsec_reclaim_secret__________")
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	seed := seedDelivery(t, pg, srv.URL, plain, `{"n":1}`)
	claimed, err := pg.ClaimDue(t.Context(), "crashed", 30*time.Second, 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim = %v %v", claimed, err)
	}
	expireLease(t, dsn, claimed[0].Delivery.ID)

	_ = startPool(t, pg, "survivor", 10*time.Millisecond)
	waitStatus(t, dsn, seed.deliveryID, domain.DeliveryDelivered)
	if hits.Load() != 1 {
		t.Fatalf("hits = %d", hits.Load())
	}
}

func TestWorker_ConcurrentWorkersOnePOST(t *testing.T) {
	t.Parallel()

	pg, dsn := migrated(t)
	plain := []byte("whsec_concurrent_secret_______")
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	seed := seedDelivery(t, pg, srv.URL, plain, `{"n":1}`)
	_ = startPool(t, pg, "wa", 5*time.Millisecond)
	_ = startPool(t, pg, "wb", 5*time.Millisecond)
	waitStatus(t, dsn, seed.deliveryID, domain.DeliveryDelivered)
	if hits.Load() != 1 {
		t.Fatalf("hits = %d, want 1 in the lease window", hits.Load())
	}
}

type seed struct {
	eventID    string
	deliveryID string
}

func seedDelivery(t *testing.T, pg *store.Postgres, target string, plain []byte, payload string) seed {
	t.Helper()
	key, err := pg.CreateAPIKey(t.Context(), "it", uniquePrefix(), bytes.Repeat([]byte{0xab}, domain.APIKeyHashLen))
	if err != nil {
		t.Fatal(err)
	}
	env, err := domain.NewEnvelope([]byte("local-dev-only-enc-key-32bytes!!"))
	if err != nil {
		t.Fatal(err)
	}
	enc, err := env.Seal(rand.Reader, plain)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pg.CreateSubscription(t.Context(), domain.Subscription{
		APIKeyID:   key.ID,
		TargetURL:  target,
		EventTypes: []string{"*"},
		Headers:    map[string]string{},
		Enabled:    true,
	}, enc)
	if err != nil {
		t.Fatal(err)
	}
	ev, err := domain.NewEvent(key.ID, "order.created", []byte(payload), uniquePrefix(), nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := pg.EnqueueEvent(t.Context(), ev)
	if err != nil || len(res.DeliveryIDs) != 1 {
		t.Fatalf("enqueue = %+v %v", res, err)
	}
	return seed{eventID: res.EventID, deliveryID: res.DeliveryIDs[0]}
}

func startPool(t *testing.T, pg *store.Postgres, id string, backoff time.Duration) *bytes.Buffer {
	t.Helper()
	env, err := domain.NewEnvelope([]byte("local-dev-only-enc-key-32bytes!!"))
	if err != nil {
		t.Fatal(err)
	}
	buf := &bytes.Buffer{}
	p, err := worker.NewPool(worker.Config{
		Store:       pg,
		Envelope:    env,
		Logger:      slog.New(slog.NewJSONHandler(buf, nil)),
		Client:      worker.NewClient(),
		WorkerID:    id,
		Concurrency: 2,
		ClaimLimit:  8,
		Lease:       30 * time.Second,
		Poll:        20 * time.Millisecond,
		Backoff:     domain.OverrideBackoff(backoff),
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = p.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			t.Error("pool did not stop")
		}
	})
	return buf
}

func waitStatus(t *testing.T, dsn, deliveryID string, want domain.DeliveryStatus) {
	t.Helper()
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		st, err := lookupStatus(t, dsn, deliveryID)
		if err == nil && st == want {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	st, _ := lookupStatus(t, dsn, deliveryID)
	t.Fatalf("status = %s, want %s", st, want)
}

func lookupStatus(t *testing.T, dsn, id string) (domain.DeliveryStatus, error) {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), dsn)
	if err != nil {
		return "", err
	}
	defer func() { _ = conn.Close(context.Background()) }()
	var st string
	err = conn.QueryRow(t.Context(), `SELECT status FROM deliveries WHERE id = $1::uuid`, id).Scan(&st)
	return domain.DeliveryStatus(st), err
}

func attemptCount(t *testing.T, dsn, id string) int {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	var n int
	if err := conn.QueryRow(t.Context(), `SELECT count(*) FROM delivery_attempts WHERE delivery_id = $1::uuid`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func expireLease(t *testing.T, dsn, id string) {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	if _, err := conn.Exec(t.Context(), `UPDATE deliveries SET lease_owner = COALESCE(lease_owner, 'expired'), lease_until = now() - interval '1 second' WHERE id = $1::uuid`, id); err != nil {
		t.Fatal(err)
	}
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
