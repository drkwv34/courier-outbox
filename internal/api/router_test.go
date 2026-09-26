package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouter(t *testing.T) {
	t.Parallel()

	hasher := testHasher(t)
	keys := &fakeKeys{}
	valid := keys.issue(t, hasher, "key-1", false)

	h := NewRouter(Deps{
		Logger:    discardLogger(),
		Keys:      keys,
		Hasher:    hasher,
		Readiness: []ReadinessCheck{okCheck("postgres"), okCheck("redis")},
	})

	tests := []struct {
		name       string
		method     string
		path       string
		auth       string
		wantStatus int
		wantKey    string
		wantValue  string
	}{
		{name: "healthz ok", method: http.MethodGet, path: "/healthz", wantStatus: http.StatusOK, wantKey: "status", wantValue: "ok"},
		{name: "readyz ok", method: http.MethodGet, path: "/readyz", wantStatus: http.StatusOK, wantKey: "status", wantValue: "ready"},
		{name: "unknown route", method: http.MethodGet, path: "/nope", wantStatus: http.StatusNotFound, wantKey: "code", wantValue: "not_found"},
		{name: "wrong method", method: http.MethodPost, path: "/healthz", wantStatus: http.StatusMethodNotAllowed, wantKey: "code", wantValue: "method_not_allowed"},
		{name: "v1 root without key", method: http.MethodGet, path: "/v1", wantStatus: http.StatusUnauthorized, wantKey: "code", wantValue: "unauthorized"},
		{name: "v1 path without key", method: http.MethodPost, path: "/v1/events", wantStatus: http.StatusUnauthorized, wantKey: "code", wantValue: "unauthorized"},
		{name: "v1 with bad key", method: http.MethodGet, path: "/v1/subscriptions", auth: "Bearer co_aaaaaaaa_nope", wantStatus: http.StatusUnauthorized, wantKey: "code", wantValue: "unauthorized"},
		{name: "v1 with valid key reaches empty api", method: http.MethodGet, path: "/v1/subscriptions", auth: "Bearer " + valid, wantStatus: http.StatusNotFound, wantKey: "code", wantValue: "not_found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, nil)
			if tt.auth != "" {
				req.Header.Set("Authorization", tt.auth)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
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
