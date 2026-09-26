package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func okCheck(name string) ReadinessCheck {
	return ReadinessCheck{Name: name, Check: func(context.Context) error { return nil }}
}

func failCheck(name string) ReadinessCheck {
	return ReadinessCheck{Name: name, Check: func(context.Context) error { return errors.New("dial tcp: secret-host refused") }}
}

func hangCheck(name string) ReadinessCheck {
	return ReadinessCheck{Name: name, Check: func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }}
}

func TestReadyz(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		checks     []ReadinessCheck
		wantStatus int
		wantDown   []string
	}{
		{name: "all healthy", checks: []ReadinessCheck{okCheck("postgres"), okCheck("redis")}, wantStatus: http.StatusOK},
		{name: "redis down", checks: []ReadinessCheck{okCheck("postgres"), failCheck("redis")}, wantStatus: http.StatusServiceUnavailable, wantDown: []string{"redis"}},
		{name: "both down", checks: []ReadinessCheck{failCheck("postgres"), failCheck("redis")}, wantStatus: http.StatusServiceUnavailable, wantDown: []string{"postgres", "redis"}},
		{name: "hung dependency times out", checks: []ReadinessCheck{hangCheck("postgres"), okCheck("redis")}, wantStatus: http.StatusServiceUnavailable, wantDown: []string{"postgres"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			readyz(discardLogger(), tt.checks)(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/readyz", nil))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantStatus == http.StatusOK {
				var body map[string]string
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["status"] != "ready" {
					t.Fatalf("body = %s, want {\"status\":\"ready\"}", rec.Body)
				}
				return
			}
			body := decodeError(t, rec)
			if body.Code != "unavailable" {
				t.Fatalf("code = %q, want unavailable", body.Code)
			}
			for _, dep := range tt.wantDown {
				if !strings.Contains(body.Message, dep) {
					t.Fatalf("message %q does not name %s", body.Message, dep)
				}
			}
			if strings.Contains(rec.Body.String(), "secret-host") {
				t.Fatalf("readiness leaked driver error: %s", rec.Body)
			}
		})
	}
}
