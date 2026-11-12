package worker

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/drkwv34/courier-outbox/internal/domain"
)

func TestPrometheus_ObserveAttempt(t *testing.T) {
	t.Parallel()

	p := NewPrometheus()
	p.ObserveAttempt(domain.DeliveryDelivered, 25*time.Millisecond)
	p.ObserveAttempt(domain.DeliveryRetrying, 10*time.Millisecond)
	p.ObserveAttempt(domain.DeliveryDeadLettered, 5*time.Millisecond)

	body := scrape(t, p)
	for _, name := range []string{
		"courier_deliveries_total",
		"courier_attempt_duration_seconds",
		"courier_dlq_total",
	} {
		if !strings.Contains(body, name) {
			t.Errorf("missing %s in %s", name, body)
		}
	}
	if !strings.Contains(body, `courier_deliveries_total{status="delivered"} 1`) {
		t.Errorf("delivered counter: %s", body)
	}
	if !strings.Contains(body, `courier_deliveries_total{status="retrying"} 1`) {
		t.Errorf("retrying counter: %s", body)
	}
	if !strings.Contains(body, `courier_deliveries_total{status="dead_lettered"} 1`) {
		t.Errorf("dead_lettered counter: %s", body)
	}
	if !strings.Contains(body, "courier_dlq_total 1") {
		t.Errorf("dlq: %s", body)
	}
	if strings.Contains(body, "event_id") || strings.Contains(body, "http://") {
		t.Fatal("metrics leaked ids or URLs")
	}
}

func TestPrometheus_ZeroNames(t *testing.T) {
	t.Parallel()

	body := scrape(t, NewPrometheus())
	for _, name := range []string{
		"courier_deliveries_total",
		"courier_attempt_duration_seconds",
		"courier_dlq_total",
	} {
		if !strings.Contains(body, name) {
			t.Errorf("missing %s at zero: %s", name, body)
		}
	}
}

func TestObserveAttempt_NilObserver(t *testing.T) {
	t.Parallel()

	var p *Prometheus
	p.ObserveAttempt(domain.DeliveryDelivered, time.Millisecond)
}

func TestEmptyHandler_OmitsDeliverySeries(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", nil)
	EmptyHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, name := range []string{
		"courier_deliveries_total",
		"courier_attempt_duration_seconds",
		"courier_dlq_total",
	} {
		if strings.Contains(body, name) {
			t.Errorf("empty handler registered %s: %s", name, body)
		}
	}
}

func TestServeMetrics_AfterDelivered(t *testing.T) {
	t.Parallel()

	p := NewPrometheus()
	p.ObserveAttempt(domain.DeliveryDelivered, 10*time.Millisecond)

	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- ServeMetrics(ctx, addr, p.Handler()) }()

	var body string
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+addr+"/metrics", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d body %s", resp.StatusCode, raw)
		}
		body = string(raw)
		break
	}
	if body == "" {
		t.Fatal("worker metrics HTTP never answered")
	}
	if !strings.Contains(body, `courier_deliveries_total{status="delivered"}`) ||
		strings.Contains(body, `courier_deliveries_total{status="delivered"} 0`) {
		t.Fatalf("delivered counter: %s", body)
	}
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ServeMetrics did not return")
	}
}

func scrape(t *testing.T, p *Prometheus) string {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", nil)
	p.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body %s", rec.Code, rec.Body)
	}
	b, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
