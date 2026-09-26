package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouter(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewRouter(logger)

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantKey    string
		wantValue  string
	}{
		{name: "healthz ok", method: http.MethodGet, path: "/healthz", wantStatus: http.StatusOK, wantKey: "status", wantValue: "ok"},
		{name: "unknown route", method: http.MethodGet, path: "/nope", wantStatus: http.StatusNotFound, wantKey: "code", wantValue: "not_found"},
		{name: "wrong method", method: http.MethodPost, path: "/healthz", wantStatus: http.StatusMethodNotAllowed, wantKey: "code", wantValue: "method_not_allowed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, nil))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Fatalf("Content-Type = %q, want application/json", ct)
			}
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body[tt.wantKey] != tt.wantValue {
				t.Fatalf("body[%q] = %q, want %q", tt.wantKey, body[tt.wantKey], tt.wantValue)
			}
		})
	}
}
