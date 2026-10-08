package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEvents_CreateGetReplay(t *testing.T) {
	t.Parallel()

	h, keys, _, valid := newEventRouter(t)
	other := keys.issue(t, testHasher(t), "key-other", false)

	body := `{"type":"order.created","payload":{"order_id":"1"},"idempotency_key":"enq-1"}`
	rec := doJSON(t, h, http.MethodPost, "/v1/events", valid, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d body %s", rec.Code, rec.Body)
	}
	if rec.Header().Get(idempotentReplayHeader) != "" {
		t.Fatalf("201 must omit replay header, got %q", rec.Header().Get(idempotentReplayHeader))
	}
	created := decodeEnqueue(t, rec)
	if created.EventID == "" || len(created.DeliveryIDs) != 1 {
		t.Fatalf("create body = %+v", created)
	}

	rec = doJSON(t, h, http.MethodPost, "/v1/events", valid, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("replay status = %d body %s", rec.Code, rec.Body)
	}
	if rec.Header().Get(idempotentReplayHeader) != idempotentReplayValue {
		t.Fatalf("replay header = %q, want %q", rec.Header().Get(idempotentReplayHeader), idempotentReplayValue)
	}
	replayed := decodeEnqueue(t, rec)
	if replayed.EventID != created.EventID {
		t.Fatalf("replay id %s vs %s", replayed.EventID, created.EventID)
	}
	if len(replayed.DeliveryIDs) != len(created.DeliveryIDs) {
		t.Fatalf("replay deliveries %v vs %v", replayed.DeliveryIDs, created.DeliveryIDs)
	}

	rec = doJSON(t, h, http.MethodGet, "/v1/events/"+created.EventID, valid, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d body %s", rec.Code, rec.Body)
	}
	got := decodeEvent(t, rec)
	if got.ID != created.EventID || got.Type != "order.created" || string(got.Payload) == "" {
		t.Fatalf("get body = %+v", got)
	}
	if !json.Valid(got.Payload) || got.Payload[0] != '{' {
		t.Fatalf("payload = %s", got.Payload)
	}
	if len(got.Deliveries) != 1 || got.Deliveries[0].Status != "pending" {
		t.Fatalf("deliveries = %+v", got.Deliveries)
	}
	if got.OccurredAt.IsZero() || got.CreatedAt.IsZero() {
		t.Fatal("missing timestamps")
	}

	rec = doJSON(t, h, http.MethodGet, "/v1/events/"+created.EventID, other, "")
	if rec.Code != http.StatusNotFound || decodeError(t, rec).Code != "not_found" {
		t.Fatalf("foreign get = %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "order_id") {
		t.Fatalf("foreign get leaked payload: %s", rec.Body)
	}
}

func TestEvents_Validation(t *testing.T) {
	t.Parallel()

	h, _, _, valid := newEventRouter(t)
	okID := "aaaaaaaa-bbbb-cccc-dddd-000000000001"

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		want   int
		code   string
	}{
		{name: "missing type", method: http.MethodPost, path: "/v1/events", body: `{"payload":{},"idempotency_key":"k"}`, want: http.StatusBadRequest, code: "validation_failed"},
		{name: "invalid type", method: http.MethodPost, path: "/v1/events", body: `{"type":"Order.Created","payload":{},"idempotency_key":"k"}`, want: http.StatusBadRequest, code: "validation_failed"},
		{name: "array payload", method: http.MethodPost, path: "/v1/events", body: `{"type":"a.b","payload":[],"idempotency_key":"k"}`, want: http.StatusBadRequest, code: "validation_failed"},
		{name: "null payload", method: http.MethodPost, path: "/v1/events", body: `{"type":"a.b","payload":null,"idempotency_key":"k"}`, want: http.StatusBadRequest, code: "validation_failed"},
		{name: "empty key", method: http.MethodPost, path: "/v1/events", body: `{"type":"a.b","payload":{},"idempotency_key":""}`, want: http.StatusBadRequest, code: "validation_failed"},
		{name: "long key", method: http.MethodPost, path: "/v1/events", body: `{"type":"a.b","payload":{},"idempotency_key":"` + strings.Repeat("k", 129) + `"}`, want: http.StatusBadRequest, code: "validation_failed"},
		{name: "bad occurred_at", method: http.MethodPost, path: "/v1/events", body: `{"type":"a.b","payload":{},"idempotency_key":"k","occurred_at":"tomorrow"}`, want: http.StatusBadRequest, code: "validation_failed"},
		{name: "unknown field", method: http.MethodPost, path: "/v1/events", body: `{"type":"a.b","payload":{},"idempotency_key":"k","nope":1}`, want: http.StatusBadRequest, code: "validation_failed"},
		{name: "malformed id", method: http.MethodGet, path: "/v1/events/not-a-uuid", body: "", want: http.StatusBadRequest, code: "validation_failed"},
		{name: "unknown id", method: http.MethodGet, path: "/v1/events/" + okID, body: "", want: http.StatusNotFound, code: "not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := doJSON(t, h, tt.method, tt.path, valid, tt.body)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.want, rec.Body)
			}
			if decodeError(t, rec).Code != tt.code {
				t.Fatalf("code = %s, want %s", rec.Body, tt.code)
			}
		})
	}
}

func TestEvents_OversizedBody(t *testing.T) {
	t.Parallel()

	h, _, _, valid := newEventRouter(t)
	body := `{"type":"a.b","idempotency_key":"k","payload":{"x":"` + strings.Repeat("a", maxJSONBody) + `"}}`
	rec := doJSON(t, h, http.MethodPost, "/v1/events", valid, body)
	if rec.Code != http.StatusRequestEntityTooLarge || decodeError(t, rec).Code != "payload_too_large" {
		t.Fatalf("status = %d body %s", rec.Code, rec.Body)
	}
}

func TestEvents_OccurredAtAccepted(t *testing.T) {
	t.Parallel()

	h, _, _, valid := newEventRouter(t)
	ts := time.Date(2026, 10, 7, 15, 4, 5, 0, time.UTC).Format(time.RFC3339)
	body := `{"type":"order.created","payload":{},"idempotency_key":"ts-1","occurred_at":"` + ts + `"}`
	rec := doJSON(t, h, http.MethodPost, "/v1/events", valid, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body %s", rec.Code, rec.Body)
	}
	created := decodeEnqueue(t, rec)
	rec = doJSON(t, h, http.MethodGet, "/v1/events/"+created.EventID, valid, "")
	got := decodeEvent(t, rec)
	if !got.OccurredAt.Equal(time.Date(2026, 10, 7, 15, 4, 5, 0, time.UTC)) {
		t.Fatalf("occurred_at = %s", got.OccurredAt)
	}
}

func TestEvents_StoreErrorIsInternal(t *testing.T) {
	t.Parallel()

	h, keys, events, _ := newEventRouter(t)
	valid := keys.issue(t, testHasher(t), "k-err", false)
	events.err = errStoreDown
	rec := doJSON(t, h, http.MethodPost, "/v1/events", valid, `{"type":"a.b","payload":{},"idempotency_key":"k"}`)
	if rec.Code != http.StatusInternalServerError || decodeError(t, rec).Code != "internal" {
		t.Fatalf("status = %d body %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "connection refused") {
		t.Fatal("leaked store error")
	}
}

func TestEvents_Unauthorized(t *testing.T) {
	t.Parallel()

	h, _, _, _ := newEventRouter(t)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/events", strings.NewReader(`{"type":"a.b","payload":{},"idempotency_key":"k"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized || decodeError(t, rec).Code != "unauthorized" {
		t.Fatalf("status = %d body %s", rec.Code, rec.Body)
	}
}

func TestSetIdempotentReplay(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	setIdempotentReplay(rec)
	if rec.Header().Get(idempotentReplayHeader) != idempotentReplayValue {
		t.Fatalf("header = %q", rec.Header().Get(idempotentReplayHeader))
	}
}

func newEventRouter(t *testing.T) (http.Handler, *fakeKeys, *fakeEvents, string) {
	t.Helper()
	hasher := testHasher(t)
	keys := &fakeKeys{}
	valid := keys.issue(t, hasher, "key-1", false)
	events := &fakeEvents{}
	h := NewRouter(Deps{
		Logger:        discardLogger(),
		Keys:          keys,
		Hasher:        hasher,
		Readiness:     []ReadinessCheck{okCheck("postgres"), okCheck("redis")},
		Subscriptions: &fakeSubs{},
		Events:        events,
		URLPolicy:     testPolicy(),
		Envelope:      testEnvelope(t),
	})
	return h, keys, events, valid
}

func decodeEnqueue(t *testing.T, rec *httptest.ResponseRecorder) enqueueResponse {
	t.Helper()
	var out enqueueResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode enqueue: %v", err)
	}
	return out
}

func decodeEvent(t *testing.T, rec *httptest.ResponseRecorder) eventResponse {
	t.Helper()
	var out eventResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode event: %v", err)
	}
	return out
}
