package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/drkwv34/courier-outbox/internal/domain"
)

func TestSubscriptions_CreateListGetRotateDisable(t *testing.T) {
	t.Parallel()

	h, keys, subs, valid := newSubscriptionRouter(t)
	other := keys.issue(t, testHasher(t), "key-other", false)

	createBody := `{"target_url":"https://example.com/hooks","event_types":["order.created"],"headers":{"X-Shop":"acme"},"description":"primary"}`
	rec := doJSON(t, h, http.MethodPost, "/v1/subscriptions", valid, createBody)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d body %s", rec.Code, rec.Body)
	}
	created := decodeSub(t, rec)
	if created.SigningSecret == "" || created.ID == "" || created.TargetURL != "https://example.com/hooks" {
		t.Fatalf("create body = %+v", created)
	}
	raw, err := base64.StdEncoding.DecodeString(created.SigningSecret)
	if err != nil || len(raw) != domain.SigningSecretBytes {
		t.Fatalf("signing_secret = %q err %v", created.SigningSecret, err)
	}
	if bytes.Equal(subs.ciphertext(created.ID), raw) {
		t.Fatal("store persisted plaintext secret")
	}

	rec = doJSON(t, h, http.MethodGet, "/v1/subscriptions/"+created.ID, valid, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d body %s", rec.Code, rec.Body)
	}
	got := decodeSub(t, rec)
	if got.SigningSecret != "" {
		t.Fatal("GET re-exposed signing_secret")
	}
	if strings.Contains(rec.Body.String(), "signing_secret") {
		t.Fatal("GET body mentioned signing_secret")
	}

	rec = doJSON(t, h, http.MethodGet, "/v1/subscriptions", valid, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d body %s", rec.Code, rec.Body)
	}
	var listed subscriptionListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Items) != 1 || listed.Items[0].SigningSecret != "" {
		t.Fatalf("list = %+v", listed)
	}

	rec = doJSON(t, h, http.MethodPatch, "/v1/subscriptions/"+created.ID, valid, `{"description":"updated"}`)
	patched := decodeSub(t, rec)
	if rec.Code != http.StatusOK || patched.Description != "updated" || patched.TargetURL != created.TargetURL || patched.SigningSecret != "" {
		t.Fatalf("patch = %d %+v", rec.Code, patched)
	}

	rec = doJSON(t, h, http.MethodPatch, "/v1/subscriptions/"+created.ID, valid, `{"rotate_secret":true}`)
	rotated := decodeSub(t, rec)
	if rec.Code != http.StatusOK || rotated.SigningSecret == "" || rotated.SigningSecret == created.SigningSecret {
		t.Fatalf("rotate = %d %+v", rec.Code, rotated)
	}

	rec = doJSON(t, h, http.MethodPost, "/v1/subscriptions/"+created.ID+"/disable", valid, "")
	if rec.Code != http.StatusOK || decodeSub(t, rec).Enabled {
		t.Fatalf("disable = %d %s", rec.Code, rec.Body)
	}
	rec = doJSON(t, h, http.MethodPost, "/v1/subscriptions/"+created.ID+"/disable", valid, "")
	if rec.Code != http.StatusOK || decodeSub(t, rec).Enabled {
		t.Fatalf("second disable = %d %s", rec.Code, rec.Body)
	}

	rec = doJSON(t, h, http.MethodGet, "/v1/subscriptions/"+created.ID, other, "")
	if rec.Code != http.StatusNotFound || decodeError(t, rec).Code != "not_found" {
		t.Fatalf("foreign get = %d %s", rec.Code, rec.Body)
	}
	rec = doJSON(t, h, http.MethodPatch, "/v1/subscriptions/"+created.ID, other, `{"description":"steal"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("foreign patch = %d %s", rec.Code, rec.Body)
	}
	rec = doJSON(t, h, http.MethodPost, "/v1/subscriptions/"+created.ID+"/disable", other, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("foreign disable = %d %s", rec.Code, rec.Body)
	}

	rec = doJSON(t, h, http.MethodGet, "/v1/subscriptions", other, "")
	var otherList subscriptionListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &otherList); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || len(otherList.Items) != 0 {
		t.Fatalf("other list = %d %+v", rec.Code, otherList)
	}
}

func TestSubscriptions_Validation(t *testing.T) {
	t.Parallel()

	h, _, _, valid := newSubscriptionRouter(t)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		want   int
		code   string
	}{
		{name: "missing url", method: http.MethodPost, path: "/v1/subscriptions", body: `{}`, want: http.StatusBadRequest, code: "validation_failed"},
		{name: "http rejected", method: http.MethodPost, path: "/v1/subscriptions", body: `{"target_url":"http://example.com/hooks"}`, want: http.StatusBadRequest, code: "validation_failed"},
		{name: "loopback", method: http.MethodPost, path: "/v1/subscriptions", body: `{"target_url":"https://127.0.0.1/hooks"}`, want: http.StatusBadRequest, code: "validation_failed"},
		{name: "star mixed", method: http.MethodPost, path: "/v1/subscriptions", body: `{"target_url":"https://example.com/h","event_types":["*","a.b"]}`, want: http.StatusBadRequest, code: "validation_failed"},
		{name: "empty types", method: http.MethodPost, path: "/v1/subscriptions", body: `{"target_url":"https://example.com/h","event_types":[]}`, want: http.StatusBadRequest, code: "validation_failed"},
		{name: "unknown field", method: http.MethodPost, path: "/v1/subscriptions", body: `{"target_url":"https://example.com/h","nope":1}`, want: http.StatusBadRequest, code: "validation_failed"},
		{name: "malformed id", method: http.MethodGet, path: "/v1/subscriptions/not-a-uuid", body: "", want: http.StatusBadRequest, code: "validation_failed"},
		{name: "unknown id", method: http.MethodGet, path: "/v1/subscriptions/11111111-2222-3333-4444-555555555555", body: "", want: http.StatusNotFound, code: "not_found"},
		{name: "empty list", method: http.MethodGet, path: "/v1/subscriptions", body: "", want: http.StatusOK, code: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := doJSON(t, h, tt.method, tt.path, valid, tt.body)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.want, rec.Body)
			}
			if tt.code != "" && decodeError(t, rec).Code != tt.code {
				t.Fatalf("code = %s, want %s", rec.Body, tt.code)
			}
		})
	}
}

func TestSubscriptions_OversizedBody(t *testing.T) {
	t.Parallel()

	h, _, _, valid := newSubscriptionRouter(t)
	body := `{"target_url":"https://example.com/h","description":"` + strings.Repeat("a", maxJSONBody) + `"}`
	rec := doJSON(t, h, http.MethodPost, "/v1/subscriptions", valid, body)
	if rec.Code != http.StatusRequestEntityTooLarge || decodeError(t, rec).Code != "payload_too_large" {
		t.Fatalf("status = %d body %s", rec.Code, rec.Body)
	}
}

func TestSubscriptions_StoreErrorIsInternal(t *testing.T) {
	t.Parallel()

	h, keys, subs, _ := newSubscriptionRouter(t)
	valid := keys.issue(t, testHasher(t), "k-err", false)
	subs.err = errStoreDown
	rec := doJSON(t, h, http.MethodGet, "/v1/subscriptions", valid, "")
	if rec.Code != http.StatusInternalServerError || decodeError(t, rec).Code != "internal" {
		t.Fatalf("status = %d body %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "connection refused") {
		t.Fatal("leaked store error")
	}
}

func newSubscriptionRouter(t *testing.T) (http.Handler, *fakeKeys, *fakeSubs, string) {
	t.Helper()
	hasher := testHasher(t)
	keys := &fakeKeys{}
	valid := keys.issue(t, hasher, "key-1", false)
	subs := &fakeSubs{}
	h := NewRouter(Deps{
		Logger:        discardLogger(),
		Keys:          keys,
		Hasher:        hasher,
		Readiness:     []ReadinessCheck{okCheck("postgres"), okCheck("redis")},
		Subscriptions: subs,
		URLPolicy:     testPolicy(),
		Envelope:      testEnvelope(t),
	})
	return h, keys, subs, valid
}

func doJSON(t *testing.T, h http.Handler, method, path, rawKey, body string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *bytes.Reader
	if body == "" {
		rdr = bytes.NewReader(nil)
	} else {
		rdr = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequestWithContext(t.Context(), method, path, rdr)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeSub(t *testing.T, rec *httptest.ResponseRecorder) subscriptionResponse {
	t.Helper()
	var s subscriptionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
		t.Fatalf("decode subscription %s: %v", rec.Body, err)
	}
	return s
}
