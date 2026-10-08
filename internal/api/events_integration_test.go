//go:build integration

package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

type enqueueJSON struct {
	EventID     string   `json:"event_id"`
	DeliveryIDs []string `json:"delivery_ids"`
}

type eventJSON struct {
	ID             string          `json:"id"`
	Type           string          `json:"type"`
	Payload        json.RawMessage `json:"payload"`
	IdempotencyKey string          `json:"idempotency_key"`
	Deliveries     []struct {
		ID             string `json:"id"`
		SubscriptionID string `json:"subscription_id"`
		Status         string `json:"status"`
		AttemptCount   int    `json:"attempt_count"`
	} `json:"deliveries"`
}

func TestEvents_EnqueueMatchingAndReplay(t *testing.T) {
	t.Parallel()

	e := newEnv(t, redisServer.URL)
	a := e.issueKey(t)
	b := e.issueKey(t)

	starID := createSub(t, e, a, `{"target_url":"https://example.com/hooks","event_types":["*"]}`)
	namedID := createSub(t, e, a, `{"target_url":"https://example.com/paid","event_types":["order.created"]}`)
	_ = createSub(t, e, a, `{"target_url":"https://example.com/other","event_types":["order.paid"]}`)
	disabled := createSub(t, e, a, `{"target_url":"https://example.com/off","event_types":["*"]}`)
	status, _ := e.do(t, http.MethodPost, "/v1/subscriptions/"+disabled+"/disable", a, "")
	if status != http.StatusOK {
		t.Fatalf("disable = %d", status)
	}
	_ = createSub(t, e, b, `{"target_url":"https://example.com/other-key","event_types":["*"]}`)

	body := `{"type":"order.created","payload":{"order_id":"1"},"idempotency_key":"enq-1"}`
	status, hdr, raw := e.doHeader(t, http.MethodPost, "/v1/events", a, body)
	if status != http.StatusCreated {
		t.Fatalf("create = %d %s", status, raw)
	}
	if hdr.Get("Idempotent-Replay") != "" {
		t.Fatalf("201 replay header = %q", hdr.Get("Idempotent-Replay"))
	}
	var created enqueueJSON
	if err := json.Unmarshal(raw, &created); err != nil {
		t.Fatal(err)
	}
	if created.EventID == "" || len(created.DeliveryIDs) != 2 {
		t.Fatalf("create body = %+v", created)
	}

	status, hdr, raw = e.doHeader(t, http.MethodPost, "/v1/events", a, body)
	if status != http.StatusOK || hdr.Get("Idempotent-Replay") != "true" {
		t.Fatalf("replay = %d header %q body %s", status, hdr.Get("Idempotent-Replay"), raw)
	}
	var replayed enqueueJSON
	if err := json.Unmarshal(raw, &replayed); err != nil {
		t.Fatal(err)
	}
	if replayed.EventID != created.EventID {
		t.Fatalf("replay id %s vs %s", replayed.EventID, created.EventID)
	}

	status, raw = e.do(t, http.MethodGet, "/v1/events/"+created.EventID, a, "")
	if status != http.StatusOK {
		t.Fatalf("get = %d %s", status, raw)
	}
	var got eventJSON
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if string(got.Payload) == "" || !strings.Contains(string(got.Payload), "order_id") {
		t.Fatalf("payload missing: %s", raw)
	}
	if len(got.Deliveries) != 2 {
		t.Fatalf("deliveries = %+v", got.Deliveries)
	}
	subs := map[string]string{}
	for _, d := range got.Deliveries {
		if d.Status != "pending" || d.AttemptCount != 0 {
			t.Fatalf("delivery = %+v", d)
		}
		subs[d.SubscriptionID] = d.ID
	}
	if _, ok := subs[starID]; !ok {
		t.Fatal("missing wildcard delivery")
	}
	if _, ok := subs[namedID]; !ok {
		t.Fatal("missing named delivery")
	}

	status, raw = e.do(t, http.MethodGet, "/v1/events/"+created.EventID, b, "")
	if status != http.StatusNotFound {
		t.Fatalf("foreign get = %d %s", status, raw)
	}
	if strings.Contains(string(raw), "order_id") {
		t.Fatalf("foreign get leaked payload: %s", raw)
	}

	status, raw = e.do(t, http.MethodPost, "/v1/events", b, body)
	if status != http.StatusCreated {
		t.Fatalf("other key same idempotency = %d %s", status, raw)
	}
	var other enqueueJSON
	if err := json.Unmarshal(raw, &other); err != nil {
		t.Fatal(err)
	}
	if other.EventID == created.EventID {
		t.Fatal("idempotency key leaked across API keys")
	}
}

func TestEvents_ParallelDuplicate(t *testing.T) {
	t.Parallel()

	e := newEnv(t, redisServer.URL)
	key := e.issueKey(t)
	_ = createSub(t, e, key, `{"target_url":"https://example.com/hooks","event_types":["*"]}`)

	body := `{"type":"order.created","payload":{"n":1},"idempotency_key":"parallel-1"}`
	const n = 20
	type result struct {
		status int
		replay string
		body   enqueueJSON
		err    string
	}
	ch := make(chan result, n)
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, e.server.URL+"/v1/events", strings.NewReader(body))
			if err != nil {
				ch <- result{err: err.Error()}
				return
			}
			req.Header.Set("Authorization", "Bearer "+key)
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				ch <- result{err: err.Error()}
				return
			}
			raw, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err != nil {
				ch <- result{err: err.Error()}
				return
			}
			var out enqueueJSON
			if err := json.Unmarshal(raw, &out); err != nil {
				ch <- result{err: string(raw)}
				return
			}
			ch <- result{status: resp.StatusCode, replay: resp.Header.Get("Idempotent-Replay"), body: out}
		}()
	}
	wg.Wait()
	close(ch)

	var eventID string
	created, replayed := 0, 0
	for r := range ch {
		if r.err != "" {
			t.Fatalf("request error: %s", r.err)
		}
		if r.body.EventID == "" {
			t.Fatalf("empty event id: %+v", r)
		}
		if eventID == "" {
			eventID = r.body.EventID
		}
		if r.body.EventID != eventID {
			t.Fatalf("event ids diverged: %s vs %s", r.body.EventID, eventID)
		}
		switch {
		case r.status == http.StatusCreated && r.replay == "":
			created++
		case r.status == http.StatusOK && r.replay == "true":
			replayed++
		default:
			t.Fatalf("unexpected result %+v", r)
		}
	}
	if created+replayed != n {
		t.Fatalf("created=%d replayed=%d want %d total", created, replayed, n)
	}
	if created > 1 {
		t.Fatalf("created %d events, want at most 1", created)
	}
}

func TestEvents_OversizedPayloadIntegration(t *testing.T) {
	t.Parallel()

	e := newEnv(t, redisServer.URL)
	key := e.issueKey(t)
	body := `{"type":"a.b","idempotency_key":"big","payload":{"x":"` + strings.Repeat("a", 256*1024) + `"}}`
	status, raw := e.do(t, http.MethodPost, "/v1/events", key, body)
	if status != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d body %s", status, raw)
	}
	var env struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(raw, &env); err != nil || env.Code != "payload_too_large" {
		t.Fatalf("body = %s", raw)
	}
}

func createSub(t *testing.T, e env, bearer, body string) string {
	t.Helper()
	status, raw := e.do(t, http.MethodPost, "/v1/subscriptions", bearer, body)
	if status != http.StatusCreated {
		t.Fatalf("create sub = %d %s", status, raw)
	}
	var s subJSON
	if err := json.Unmarshal(raw, &s); err != nil || s.ID == "" {
		t.Fatalf("sub body = %s err %v", raw, err)
	}
	return s.ID
}
