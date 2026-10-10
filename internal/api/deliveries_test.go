package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/drkwv34/courier-outbox/internal/domain"
)

func TestDeliveries_ListGetReplay(t *testing.T) {
	t.Parallel()

	h, keys, dels, valid := newDeliveryRouter(t)
	other := keys.issue(t, testHasher(t), "key-other", false)

	dead := dels.seed(t, "key-1", domain.DeliveryDeadLettered, 6, "order.created")
	pending := dels.seed(t, "key-1", domain.DeliveryPending, 0, "order.paid")
	_ = dels.seed(t, "key-other", domain.DeliveryDeadLettered, 6, "order.created")

	rec := doJSON(t, h, http.MethodGet, "/v1/deliveries?status=dead_lettered", valid, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d %s", rec.Code, rec.Body)
	}
	listed := decodeDeliveryList(t, rec)
	if listed.Total != 1 || len(listed.Items) != 1 || listed.Items[0].ID != dead.ID {
		t.Fatalf("list = %+v", listed)
	}
	if listed.Items[0].Status != "dead_lettered" || strings.Contains(rec.Body.String(), "payload") {
		t.Fatalf("list body = %s", rec.Body)
	}

	rec = doJSON(t, h, http.MethodGet, "/v1/deliveries/"+dead.ID, valid, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get = %d %s", rec.Code, rec.Body)
	}
	got := decodeDelivery(t, rec)
	if got.ID != dead.ID || len(got.Attempts) != 2 {
		t.Fatalf("detail = %+v", got)
	}
	if got.Attempts[0].StatusCode == nil || *got.Attempts[0].StatusCode != 500 {
		t.Fatalf("attempts = %+v", got.Attempts)
	}
	if strings.Contains(rec.Body.String(), "whsec") || strings.Contains(rec.Body.String(), "order_id") {
		t.Fatalf("leaked secret or payload: %s", rec.Body)
	}

	rec = doJSON(t, h, http.MethodGet, "/v1/deliveries/"+dead.ID, other, "")
	if rec.Code != http.StatusNotFound || decodeError(t, rec).Code != "not_found" {
		t.Fatalf("foreign get = %d %s", rec.Code, rec.Body)
	}

	rec = doJSON(t, h, http.MethodPost, "/v1/deliveries/"+dead.ID+"/replay", valid, "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("replay = %d %s", rec.Code, rec.Body)
	}
	replayed := decodeDelivery(t, rec)
	if replayed.Status != "pending" || replayed.AttemptCount != 6 {
		t.Fatalf("replay body = %+v", replayed)
	}

	rec = doJSON(t, h, http.MethodPost, "/v1/deliveries/"+pending.ID+"/replay", valid, "")
	if rec.Code != http.StatusConflict || decodeError(t, rec).Code != "invalid_transition" {
		t.Fatalf("replay pending = %d %s", rec.Code, rec.Body)
	}
}

func TestDeliveries_Validation(t *testing.T) {
	t.Parallel()

	h, _, _, valid := newDeliveryRouter(t)
	okID := "cccccccc-dddd-eeee-ffff-000000000001"

	tests := []struct {
		name   string
		method string
		path   string
		want   int
		code   string
	}{
		{name: "bad status", method: http.MethodGet, path: "/v1/deliveries?status=nope", want: http.StatusBadRequest, code: "validation_failed"},
		{name: "bad subscription", method: http.MethodGet, path: "/v1/deliveries?subscription_id=nope", want: http.StatusBadRequest, code: "validation_failed"},
		{name: "bad type", method: http.MethodGet, path: "/v1/deliveries?type=Order.Created", want: http.StatusBadRequest, code: "validation_failed"},
		{name: "bad created_after", method: http.MethodGet, path: "/v1/deliveries?created_after=tomorrow", want: http.StatusBadRequest, code: "validation_failed"},
		{name: "bad limit", method: http.MethodGet, path: "/v1/deliveries?limit=0", want: http.StatusBadRequest, code: "validation_failed"},
		{name: "limit too big", method: http.MethodGet, path: "/v1/deliveries?limit=101", want: http.StatusBadRequest, code: "validation_failed"},
		{name: "bad offset", method: http.MethodGet, path: "/v1/deliveries?offset=-1", want: http.StatusBadRequest, code: "validation_failed"},
		{name: "malformed id", method: http.MethodGet, path: "/v1/deliveries/not-a-uuid", want: http.StatusBadRequest, code: "validation_failed"},
		{name: "unknown id", method: http.MethodGet, path: "/v1/deliveries/" + okID, want: http.StatusNotFound, code: "not_found"},
		{name: "replay unknown", method: http.MethodPost, path: "/v1/deliveries/" + okID + "/replay", want: http.StatusNotFound, code: "not_found"},
		{name: "replay malformed", method: http.MethodPost, path: "/v1/deliveries/nope/replay", want: http.StatusBadRequest, code: "validation_failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := doJSON(t, h, tt.method, tt.path, valid, "")
			if rec.Code != tt.want || decodeError(t, rec).Code != tt.code {
				t.Fatalf("status = %d body %s, want %d %s", rec.Code, rec.Body, tt.want, tt.code)
			}
		})
	}
}

func TestDeliveries_Unauthorized(t *testing.T) {
	t.Parallel()

	h, _, _, _ := newDeliveryRouter(t)
	for _, path := range []string{"/v1/deliveries", "/v1/deliveries/cccccccc-dddd-eeee-ffff-000000000001", "/v1/deliveries/cccccccc-dddd-eeee-ffff-000000000001/replay"} {
		method := http.MethodGet
		if strings.HasSuffix(path, "/replay") {
			method = http.MethodPost
		}
		rec := doJSON(t, h, method, path, "co_aaaaaaaa_nope", "")
		if rec.Code != http.StatusUnauthorized || decodeError(t, rec).Code != "unauthorized" {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body)
		}
	}
}

func TestDeliveries_StoreErrorIsInternal(t *testing.T) {
	t.Parallel()

	h, keys, dels, _ := newDeliveryRouter(t)
	valid := keys.issue(t, testHasher(t), "k-err", false)
	dels.err = errStoreDown
	rec := doJSON(t, h, http.MethodGet, "/v1/deliveries", valid, "")
	if rec.Code != http.StatusInternalServerError || decodeError(t, rec).Code != "internal" {
		t.Fatalf("status = %d body %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "connection refused") {
		t.Fatal("leaked store error")
	}
}

func TestDeliveries_ListFilters(t *testing.T) {
	t.Parallel()

	h, _, dels, valid := newDeliveryRouter(t)
	a := dels.seed(t, "key-1", domain.DeliveryRetrying, 2, "order.created")
	_ = dels.seed(t, "key-1", domain.DeliveryPending, 0, "order.paid")

	rec := doJSON(t, h, http.MethodGet, "/v1/deliveries?type=order.created&limit=1&offset=0", valid, "")
	listed := decodeDeliveryList(t, rec)
	if rec.Code != http.StatusOK || listed.Total != 1 || listed.Items[0].ID != a.ID || listed.Limit != 1 {
		t.Fatalf("filtered = %+v status %d", listed, rec.Code)
	}

	sub := a.SubscriptionID
	rec = doJSON(t, h, http.MethodGet, "/v1/deliveries?subscription_id="+sub, valid, "")
	listed = decodeDeliveryList(t, rec)
	if listed.Total != 1 || listed.Items[0].SubscriptionID != sub {
		t.Fatalf("by sub = %+v", listed)
	}

	after := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	rec = doJSON(t, h, http.MethodGet, "/v1/deliveries?created_after="+after, valid, "")
	listed = decodeDeliveryList(t, rec)
	if rec.Code != http.StatusOK || listed.Total != 0 || len(listed.Items) != 0 {
		t.Fatalf("future after = %+v %s", listed, rec.Body)
	}
}

func newDeliveryRouter(t *testing.T) (http.Handler, *fakeKeys, *fakeDeliveries, string) {
	t.Helper()
	hasher := testHasher(t)
	keys := &fakeKeys{}
	valid := keys.issue(t, hasher, "key-1", false)
	dels := &fakeDeliveries{}
	h := NewRouter(Deps{
		Logger:        discardLogger(),
		Keys:          keys,
		Hasher:        hasher,
		Readiness:     []ReadinessCheck{okCheck("postgres"), okCheck("redis")},
		Subscriptions: &fakeSubs{},
		Events:        &fakeEvents{},
		Deliveries:    dels,
		URLPolicy:     testPolicy(),
		Envelope:      testEnvelope(t),
	})
	return h, keys, dels, valid
}

func decodeDeliveryList(t *testing.T, rec *httptest.ResponseRecorder) deliveryListResponse {
	t.Helper()
	var out deliveryListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode list: %v body %s", err, rec.Body)
	}
	return out
}

func decodeDelivery(t *testing.T, rec *httptest.ResponseRecorder) deliveryResponse {
	t.Helper()
	var out deliveryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode delivery: %v body %s", err, rec.Body)
	}
	return out
}
