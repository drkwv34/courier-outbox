package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequireAPIKey_Matrix(t *testing.T) {
	t.Parallel()

	h := testHasher(t)
	keys := &fakeKeys{}
	valid := keys.issue(t, h, "key-valid", false)
	revoked := keys.issue(t, h, "key-revoked", true)
	wrongSecret := valid[:len(valid)-1] + flipLast(valid)
	unknownPrefix := "co_zzzzzzzz_" + valid[len("co_xxxxxxxx_"):]

	tests := []struct {
		name       string
		header     string
		wantStatus int
		wantID     string
	}{
		{name: "valid key", header: "Bearer " + valid, wantStatus: http.StatusNoContent, wantID: "key-valid"},
		{name: "scheme is case-insensitive", header: "bearer " + valid, wantStatus: http.StatusNoContent, wantID: "key-valid"},
		{name: "no header", header: "", wantStatus: http.StatusUnauthorized},
		{name: "basic scheme", header: "Basic " + valid, wantStatus: http.StatusUnauthorized},
		{name: "bearer without token", header: "Bearer ", wantStatus: http.StatusUnauthorized},
		{name: "raw key without scheme", header: valid, wantStatus: http.StatusUnauthorized},
		{name: "malformed key", header: "Bearer not-a-key", wantStatus: http.StatusUnauthorized},
		{name: "unknown prefix", header: "Bearer " + unknownPrefix, wantStatus: http.StatusUnauthorized},
		{name: "wrong secret", header: "Bearer " + wrongSecret, wantStatus: http.StatusUnauthorized},
		{name: "revoked key", header: "Bearer " + revoked, wantStatus: http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotID string
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotID, _ = CallerKeyID(r.Context())
				w.WriteHeader(http.StatusNoContent)
			})
			mw := requireAPIKey(discardLogger(), keys, h)(next)

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/x", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			rec := httptest.NewRecorder()
			mw.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if gotID != tt.wantID {
				t.Fatalf("APIKeyID = %q, want %q", gotID, tt.wantID)
			}
			if tt.wantStatus == http.StatusUnauthorized {
				assertUnauthorized(t, rec)
			}
		})
	}
}

func TestRequireAPIKey_RejectionsAreIndistinguishable(t *testing.T) {
	t.Parallel()

	h := testHasher(t)
	keys := &fakeKeys{}
	valid := keys.issue(t, h, "k", false)
	revoked := keys.issue(t, h, "r", true)
	mw := requireAPIKey(discardLogger(), keys, h)(http.NotFoundHandler())

	var bodies []string
	for _, header := range []string{"", "Bearer junk", "Bearer co_zzzzzzzz_" + valid[12:], "Bearer " + valid[:len(valid)-1] + flipLast(valid), "Bearer " + revoked} {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/x", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req)
		bodies = append(bodies, rec.Body.String())
	}
	for i := 1; i < len(bodies); i++ {
		if bodies[i] != bodies[0] {
			t.Fatalf("rejection %d body %q differs from %q", i, bodies[i], bodies[0])
		}
	}
}

func TestRequireAPIKey_StoreErrorIsInternal(t *testing.T) {
	t.Parallel()

	h := testHasher(t)
	keys := &fakeKeys{}
	valid := keys.issue(t, h, "k", false)
	keys.err = errStoreDown

	called := false
	mw := requireAPIKey(discardLogger(), keys, h)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/x", nil)
	req.Header.Set("Authorization", "Bearer "+valid)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if called {
		t.Fatal("next handler ran despite store error")
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	body := decodeError(t, rec)
	if body.Code != "internal" || strings.Contains(rec.Body.String(), "connection refused") {
		t.Fatalf("body = %s, want generic internal error", rec.Body)
	}
}

func TestCallerKeyID_EmptyContext(t *testing.T) {
	t.Parallel()

	if id, ok := CallerKeyID(t.Context()); ok || id != "" {
		t.Fatalf("CallerKeyID(empty) = %q, %v; want \"\", false", id, ok)
	}
}

func flipLast(s string) string {
	if s[len(s)-1] == 'a' {
		return "b"
	}
	return "a"
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) errorBody {
	t.Helper()
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body %q: %v", rec.Body, err)
	}
	return body
}

func assertUnauthorized(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if got := rec.Header().Get("WWW-Authenticate"); !strings.HasPrefix(got, "Bearer") {
		t.Fatalf("WWW-Authenticate = %q, want Bearer challenge", got)
	}
	if body := decodeError(t, rec); body.Code != "unauthorized" {
		t.Fatalf("code = %q, want unauthorized", body.Code)
	}
}
