package worker

import (
	"io"
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
