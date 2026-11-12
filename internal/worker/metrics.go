package worker

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/drkwv34/courier-outbox/internal/domain"
)

// Observer records delivery-attempt metrics. Implementations must use only
// bounded label values (status), never ids or URLs.
type Observer interface {
	ObserveAttempt(status domain.DeliveryStatus, duration time.Duration)
}

// Prometheus is a process-local registry for courier delivery metrics.
type Prometheus struct {
	reg        *prometheus.Registry
	deliveries *prometheus.CounterVec
	duration   prometheus.Histogram
	dlq        prometheus.Counter
}

// NewPrometheus registers the FR-API-003 series on a dedicated registry.
func NewPrometheus() *Prometheus {
	reg := prometheus.NewRegistry()
	deliveries := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "courier_deliveries_total",
		Help: "Delivery attempts recorded by the worker, labeled by resulting status.",
	}, []string{"status"})
	duration := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "courier_attempt_duration_seconds",
		Help:    "Outbound webhook attempt duration in seconds.",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	})
	dlq := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "courier_dlq_total",
		Help: "Deliveries that transitioned to dead_lettered.",
	})
	reg.MustRegister(deliveries, duration, dlq)
	p := &Prometheus{reg: reg, deliveries: deliveries, duration: duration, dlq: dlq}
	for _, st := range []string{
		string(domain.DeliveryDelivered),
		string(domain.DeliveryRetrying),
		string(domain.DeliveryDeadLettered),
	} {
		p.deliveries.WithLabelValues(st)
	}
	return p
}

// ObserveAttempt increments delivery counters and observes attempt duration.
func (p *Prometheus) ObserveAttempt(status domain.DeliveryStatus, duration time.Duration) {
	if p == nil {
		return
	}
	if duration < 0 {
		duration = 0
	}
	p.duration.Observe(duration.Seconds())
	switch status {
	case domain.DeliveryDelivered, domain.DeliveryRetrying, domain.DeliveryDeadLettered:
		p.deliveries.WithLabelValues(string(status)).Inc()
	}
	if status == domain.DeliveryDeadLettered {
		p.dlq.Inc()
	}
}

// Handler returns the Prometheus text exposition handler.
func (p *Prometheus) Handler() http.Handler {
	if p == nil {
		return http.NotFoundHandler()
	}
	return promhttp.HandlerFor(p.reg, promhttp.HandlerOpts{Registry: p.reg})
}

// EmptyHandler is a public Prometheus exposition with no courier delivery
// series. The API process uses it so GET /metrics is not a zero-series trap.
func EmptyHandler() http.Handler {
	reg := prometheus.NewRegistry()
	return promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg})
}

const metricsShutdownTimeout = 10 * time.Second

// ServeMetrics serves GET /metrics on addr until ctx is cancelled.
func ServeMetrics(ctx context.Context, addr string, handler http.Handler) error {
	if handler == nil {
		handler = http.NotFoundHandler()
	}
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", handler)
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()
	select {
	case err := <-errCh:
		if err != nil {
			return err
		}
		return nil
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), metricsShutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return <-errCh
	}
}
