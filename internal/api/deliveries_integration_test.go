//go:build integration

package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/drkwv34/courier-outbox/internal/domain"
	"github.com/drkwv34/courier-outbox/internal/worker"
)

type deliveryJSON struct {
	ID             string `json:"id"`
	EventID        string `json:"event_id"`
	SubscriptionID string `json:"subscription_id"`
	EventType      string `json:"event_type"`
	Status         string `json:"status"`
	AttemptCount   int    `json:"attempt_count"`
	Attempts       []struct {
		StatusCode   *int    `json:"status_code"`
		ErrorMessage *string `json:"error_message"`
		DurationMs   int     `json:"duration_ms"`
		RequestID    string  `json:"request_id"`
	} `json:"attempts"`
}

type deliveryListJSON struct {
	Items  []deliveryJSON `json:"items"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
	Total  int            `json:"total"`
}

func TestDeliveries_ForceDLQListReplayDelivered(t *testing.T) {
	t.Parallel()

	e := newEnvPolicy(t, redisServer.URL, domain.URLPolicy{AllowHTTP: true, ProtectSSRF: false})
	key := e.issueKey(t)

	var succeed atomic.Bool
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if succeed.Load() {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(mock.Close)

	subID := createSub(t, e, key, `{"target_url":"`+mock.URL+`/hooks","event_types":["*"]}`)
	body := `{"type":"order.created","payload":{"order_id":"dlq-1"},"idempotency_key":"dlq-1"}`
	status, raw := e.do(t, http.MethodPost, "/v1/events", key, body)
	if status != http.StatusCreated {
		t.Fatalf("enqueue = %d %s", status, raw)
	}
	var created enqueueJSON
	if err := json.Unmarshal(raw, &created); err != nil || len(created.DeliveryIDs) != 1 {
		t.Fatalf("enqueue body = %s err %v", raw, err)
	}
	deliveryID := created.DeliveryIDs[0]

	startWorker(t, e)

	waitDeliveryStatus(t, e, key, deliveryID, "dead_lettered")

	status, raw = e.do(t, http.MethodGet, "/v1/deliveries?status=dead_lettered", key, "")
	if status != http.StatusOK {
		t.Fatalf("list = %d %s", status, raw)
	}
	var listed deliveryListJSON
	if err := json.Unmarshal(raw, &listed); err != nil {
		t.Fatal(err)
	}
	if listed.Total != 1 || len(listed.Items) != 1 || listed.Items[0].ID != deliveryID || listed.Items[0].Status != "dead_lettered" {
		t.Fatalf("list body = %s", raw)
	}
	if listed.Items[0].SubscriptionID != subID || listed.Items[0].EventType != "order.created" {
		t.Fatalf("list item = %+v", listed.Items[0])
	}
	if strings.Contains(string(raw), "order_id") || strings.Contains(string(raw), "signing_secret") {
		t.Fatalf("list leaked payload or secret: %s", raw)
	}

	status, raw = e.do(t, http.MethodGet, "/v1/deliveries/"+deliveryID, key, "")
	if status != http.StatusOK {
		t.Fatalf("detail = %d %s", status, raw)
	}
	var detail deliveryJSON
	if err := json.Unmarshal(raw, &detail); err != nil {
		t.Fatal(err)
	}
	failedAttempts := len(detail.Attempts)
	if failedAttempts != domain.MaxAttempts || detail.AttemptCount != domain.MaxAttempts {
		t.Fatalf("before replay attempts=%d count=%d body=%s", failedAttempts, detail.AttemptCount, raw)
	}
	if detail.Attempts[0].StatusCode == nil || *detail.Attempts[0].StatusCode != http.StatusInternalServerError {
		t.Fatalf("first attempt = %+v", detail.Attempts[0])
	}

	other := e.issueKey(t)
	status, _ = e.do(t, http.MethodGet, "/v1/deliveries/"+deliveryID, other, "")
	if status != http.StatusNotFound {
		t.Fatalf("foreign get = %d", status)
	}

	status, raw = e.do(t, http.MethodPost, "/v1/deliveries/"+deliveryID+"/replay", other, "")
	if status != http.StatusNotFound {
		t.Fatalf("foreign replay = %d %s", status, raw)
	}

	succeed.Store(true)
	status, raw = e.do(t, http.MethodPost, "/v1/deliveries/"+deliveryID+"/replay", key, "")
	if status != http.StatusAccepted {
		t.Fatalf("replay = %d %s", status, raw)
	}
	var replayed deliveryJSON
	if err := json.Unmarshal(raw, &replayed); err != nil || replayed.Status != "pending" || replayed.AttemptCount != domain.MaxAttempts {
		t.Fatalf("replay body = %s", raw)
	}

	status, raw = e.do(t, http.MethodPost, "/v1/deliveries/"+deliveryID+"/replay", key, "")
	if status != http.StatusConflict {
		t.Fatalf("second replay = %d %s", status, raw)
	}
	var env struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(raw, &env); err != nil || env.Code != "invalid_transition" {
		t.Fatalf("second replay body = %s", raw)
	}

	waitDeliveryStatus(t, e, key, deliveryID, "delivered")

	status, raw = e.do(t, http.MethodGet, "/v1/deliveries/"+deliveryID, key, "")
	if status != http.StatusOK {
		t.Fatalf("after deliver = %d %s", status, raw)
	}
	if err := json.Unmarshal(raw, &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Status != "delivered" {
		t.Fatalf("status = %s", detail.Status)
	}
	if len(detail.Attempts) <= failedAttempts {
		t.Fatalf("attempts after success = %d, want > %d", len(detail.Attempts), failedAttempts)
	}
	last := detail.Attempts[len(detail.Attempts)-1]
	if last.StatusCode == nil || *last.StatusCode != http.StatusOK {
		t.Fatalf("last attempt = %+v", last)
	}
}

func startWorker(t *testing.T, e env) {
	t.Helper()
	envelope, err := domain.NewEnvelope([]byte("local-dev-only-enc-key-32bytes!!"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := worker.NewPool(worker.Config{
		Store:       e.pg,
		Envelope:    envelope,
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Client:      worker.NewClient(),
		WorkerID:    "it-dlq",
		Concurrency: 2,
		ClaimLimit:  8,
		Lease:       30 * time.Second,
		Poll:        20 * time.Millisecond,
		Backoff:     domain.OverrideBackoff(10 * time.Millisecond),
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
}

func waitDeliveryStatus(t *testing.T, e env, bearer, id, want string) {
	t.Helper()
	deadline := time.Now().Add(25 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		status, raw := e.do(t, http.MethodGet, "/v1/deliveries/"+id, bearer, "")
		if status == http.StatusOK {
			var d deliveryJSON
			if err := json.Unmarshal(raw, &d); err == nil {
				last = d.Status
				if d.Status == want {
					return
				}
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("status = %s, want %s", last, want)
}
