//go:build integration

package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/drkwv34/courier-outbox/internal/domain"
)

func TestOpenAPIAndMetrics_AfterDelivery(t *testing.T) {
	t.Parallel()

	e := newEnvPolicy(t, redisServer.URL, domain.URLPolicy{AllowHTTP: true, ProtectSSRF: false})
	key := e.issueKey(t)

	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(mock.Close)

	_ = createSub(t, e, key, `{"target_url":"`+mock.URL+`/hooks","event_types":["*"]}`)
	body := `{"type":"order.created","payload":{"order_id":"metrics-1"},"idempotency_key":"metrics-1"}`
	status, raw := e.do(t, http.MethodPost, "/v1/events", key, body)
	if status != http.StatusCreated {
		t.Fatalf("enqueue = %d %s", status, raw)
	}
	var created enqueueJSON
	if err := json.Unmarshal(raw, &created); err != nil || len(created.DeliveryIDs) != 1 {
		t.Fatalf("enqueue body = %s err %v", raw, err)
	}

	startWorker(t, e)
	waitDeliveryStatus(t, e, key, created.DeliveryIDs[0], "delivered")

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, e.server.URL+"/openapi.json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	openDoc, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("openapi = %d %s", resp.StatusCode, openDoc)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("openapi Content-Type = %q", ct)
	}
	var doc map[string]any
	if err := json.Unmarshal(openDoc, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["openapi"] == nil {
		t.Fatal("missing openapi field")
	}

	req, err = http.NewRequestWithContext(t.Context(), http.MethodGet, e.server.URL+"/metrics", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	metricsBody, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("metrics = %d %s", resp.StatusCode, metricsBody)
	}
	text := string(metricsBody)
	for _, name := range []string{
		"courier_deliveries_total",
		"courier_attempt_duration_seconds",
		"courier_dlq_total",
	} {
		if !strings.Contains(text, name) {
			t.Errorf("missing %s in %s", name, text)
		}
	}
	if !strings.Contains(text, `courier_deliveries_total{status="delivered"}`) {
		t.Errorf("missing delivered series: %s", text)
	}
}
