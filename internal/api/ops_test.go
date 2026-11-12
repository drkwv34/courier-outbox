package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/drkwv34/courier-outbox/internal/worker"
	"github.com/drkwv34/courier-outbox/openapi"
)

func TestOpenAPI_JSON(t *testing.T) {
	t.Parallel()

	doc, err := openapi.JSON()
	if err != nil {
		t.Fatal(err)
	}
	h := NewRouter(Deps{
		Logger:      discardLogger(),
		Keys:        &fakeKeys{},
		Hasher:      testHasher(t),
		OpenAPIJSON: doc,
		Metrics:     worker.EmptyHandler(),
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q", ct)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["openapi"] == nil {
		t.Fatal("missing openapi field")
	}
	paths, ok := body["paths"].(map[string]any)
	if !ok {
		t.Fatalf("paths type %T", body["paths"])
	}
	for _, p := range []string{"/healthz", "/v1/events", "/v1/deliveries", "/openapi.json", "/metrics", "/docs"} {
		if _, ok := paths[p]; !ok {
			t.Errorf("missing path %s", p)
		}
	}
}

func TestDocs_PublicHTML(t *testing.T) {
	t.Parallel()

	h := NewRouter(Deps{
		Logger: discardLogger(),
		Keys:   &fakeKeys{},
		Hasher: testHasher(t),
	})
	for _, path := range []string{"/docs", "/docs/"} {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d body %s", path, rec.Code, rec.Body)
		}
		ct := rec.Header().Get("Content-Type")
		if !strings.Contains(ct, "text/html") {
			t.Fatalf("%s Content-Type = %q", path, ct)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "/openapi.json") {
			t.Fatalf("%s missing openapi.json reference: %s", path, body)
		}
		for _, cdn := range []string{"cdn.jsdelivr", "unpkg.com", "cdnjs.cloudflare"} {
			if strings.Contains(body, cdn) {
				t.Fatalf("%s loads CDN %s", path, cdn)
			}
		}
	}
}

func TestMetrics_APIOmitsDeliverySeries(t *testing.T) {
	t.Parallel()

	h := NewRouter(Deps{
		Logger:  discardLogger(),
		Keys:    &fakeKeys{},
		Hasher:  testHasher(t),
		Metrics: worker.EmptyHandler(),
	})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body %s", rec.Code, rec.Body)
	}
	raw, _ := io.ReadAll(rec.Body)
	text := string(raw)
	for _, name := range []string{
		"courier_deliveries_total",
		"courier_attempt_duration_seconds",
		"courier_dlq_total",
	} {
		if strings.Contains(text, name) {
			t.Errorf("API /metrics must not register %s: %s", name, text)
		}
	}
}
