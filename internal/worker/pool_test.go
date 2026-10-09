package worker

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/drkwv34/courier-outbox/internal/domain"
	"github.com/drkwv34/courier-outbox/internal/sign"
)

const testSecret = "whsec_test_secret_for_golden__"

type fakeStore struct {
	mu      sync.Mutex
	jobs    []domain.DeliveryClaim
	records []domain.AttemptRecord
	err     error
}

func (f *fakeStore) ClaimDue(context.Context, string, time.Duration, int) ([]domain.DeliveryClaim, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	out := f.jobs
	f.jobs = nil
	return out, nil
}

func (f *fakeStore) RecordAttempt(_ context.Context, rec domain.AttemptRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records = append(f.records, rec)
	return nil
}

func testPool(t *testing.T, store Store, now time.Time) (*Pool, *bytes.Buffer) {
	t.Helper()
	env, err := domain.NewEnvelope([]byte("local-dev-only-enc-key-32bytes!!"))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	p, err := NewPool(Config{
		Store:       store,
		Envelope:    env,
		Logger:      logger,
		Client:      NewClient(),
		WorkerID:    "test-worker",
		Concurrency: 1,
		ClaimLimit:  8,
		Lease:       30 * time.Second,
		Poll:        10 * time.Millisecond,
		Now:         func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	return p, &buf
}

func sealedJob(t *testing.T, p *Pool, url string, payload []byte) domain.DeliveryClaim {
	t.Helper()
	enc, err := p.envelope.Seal(rand.Reader, []byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}
	return domain.DeliveryClaim{
		Delivery: domain.Delivery{
			ID:             "11111111-1111-1111-1111-111111111111",
			EventID:        "22222222-2222-2222-2222-222222222222",
			SubscriptionID: "33333333-3333-3333-3333-333333333333",
			Status:         domain.DeliveryPending,
		},
		Payload:          payload,
		TargetURL:        url,
		Headers:          map[string]string{"X-Shop": "acme"},
		SigningSecretEnc: enc,
	}
}

func TestProcess_TwoXXDeliversAndVerifiesSignature(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0).UTC()
	payload := []byte(`{"hello":"world"}`)
	var got http.Header
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		gotBody, _ = io.ReadAll(r.Body)
		ts, _ := strconv.ParseInt(r.Header.Get(sign.HeaderTimestamp), 10, 64)
		if err := sign.Verify([]byte(testSecret), ts, gotBody, r.Header.Get(sign.HeaderSignature), now); err != nil {
			t.Errorf("verify: %v", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	fs := &fakeStore{}
	p, logs := testPool(t, fs, now)
	job := sealedJob(t, p, srv.URL, payload)
	p.Process(t.Context(), job)

	if len(fs.records) != 1 {
		t.Fatalf("records = %d", len(fs.records))
	}
	rec := fs.records[0]
	if rec.Status != domain.DeliveryDelivered || rec.AttemptCount != 1 || rec.StatusCode == nil || *rec.StatusCode != 200 {
		t.Fatalf("record = %+v", rec)
	}
	if got.Get("User-Agent") != UserAgent {
		t.Fatalf("UA = %q", got.Get("User-Agent"))
	}
	if got.Get(sign.HeaderIdempotencyKey) != job.Delivery.EventID {
		t.Fatalf("idempotency = %q", got.Get(sign.HeaderIdempotencyKey))
	}
	if got.Get("X-Shop") != "acme" {
		t.Fatalf("static header missing")
	}
	if !bytes.Equal(gotBody, payload) {
		t.Fatalf("body = %s", gotBody)
	}
	assertLogSafe(t, logs.String(), job)
}

func TestProcess_FiveHundredRetries(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	fs := &fakeStore{}
	p, _ := testPool(t, fs, time.Now().UTC())
	p.Process(t.Context(), sealedJob(t, p, srv.URL, []byte(`{"n":1}`)))
	if len(fs.records) != 1 {
		t.Fatal(fs.records)
	}
	rec := fs.records[0]
	if rec.Status != domain.DeliveryRetrying || rec.AttemptCount != 1 || rec.RetryDelay != time.Second {
		t.Fatalf("record = %+v", rec)
	}
}

func TestProcess_RedirectIsFailure(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://example.com/nope", http.StatusFound)
	}))
	t.Cleanup(srv.Close)

	fs := &fakeStore{}
	p, _ := testPool(t, fs, time.Now().UTC())
	p.Process(t.Context(), sealedJob(t, p, srv.URL, []byte(`{}`)))
	rec := fs.records[0]
	if rec.Status != domain.DeliveryRetrying || rec.StatusCode == nil || *rec.StatusCode != http.StatusFound {
		t.Fatalf("record = %+v", rec)
	}
}

func TestProcess_SlowMockFails(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(requestTimeout + 400*time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	fs := &fakeStore{}
	p, logs := testPool(t, fs, time.Now().UTC())
	p.Process(t.Context(), sealedJob(t, p, srv.URL, []byte(`{"secret-payload":true}`)))
	if len(fs.records) != 1 {
		t.Fatal(fs.records)
	}
	rec := fs.records[0]
	if rec.Status != domain.DeliveryRetrying || rec.StatusCode != nil {
		t.Fatalf("record = %+v", rec)
	}
	if rec.ErrorMessage == nil || *rec.ErrorMessage != "http: request timeout" {
		t.Fatalf("error = %v", rec.ErrorMessage)
	}
	if strings.Contains(logs.String(), "secret-payload") || strings.Contains(logs.String(), testSecret) {
		t.Fatalf("log leaked secret or payload: %s", logs.String())
	}
}

func TestNewPool_RequiresStore(t *testing.T) {
	t.Parallel()
	_, err := NewPool(Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), WorkerID: "w"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func assertLogSafe(t *testing.T, raw string, job domain.DeliveryClaim) {
	t.Helper()
	if !strings.Contains(raw, job.Delivery.ID) || !strings.Contains(raw, job.Delivery.EventID) {
		t.Fatalf("log missing ids: %s", raw)
	}
	if !strings.Contains(raw, `"attempt"`) {
		t.Fatalf("log missing attempt: %s", raw)
	}
	if strings.Contains(raw, testSecret) || strings.Contains(raw, string(job.Payload)) {
		t.Fatalf("log leaked secret or payload: %s", raw)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(strings.Split(raw, "\n")[0])), &m); err != nil {
		t.Fatalf("log json: %v (%s)", err, raw)
	}
}
