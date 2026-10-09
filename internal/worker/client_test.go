package worker

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPostWebhook_OK(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != UserAgent {
			t.Errorf("User-Agent = %q", r.Header.Get("User-Agent"))
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"ok":true}` {
			t.Errorf("body = %s", body)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	hdr := make(http.Header)
	hdr.Set("User-Agent", UserAgent)
	status, err := postWebhook(t.Context(), NewClient(), srv.URL, []byte(`{"ok":true}`), hdr)
	if err != nil || status != http.StatusOK {
		t.Fatalf("status=%d err=%v", status, err)
	}
}

func TestPostWebhook_DoesNotFollowRedirect(t *testing.T) {
	t.Parallel()

	followed := false
	final := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		followed = true
	}))
	t.Cleanup(final.Close)

	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusFound)
	}))
	t.Cleanup(redir.Close)

	status, err := postWebhook(t.Context(), NewClient(), redir.URL, []byte(`{}`), make(http.Header))
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusFound {
		t.Fatalf("status = %d, want 302", status)
	}
	if followed {
		t.Fatal("client followed the redirect")
	}
}

func TestPostWebhook_SlowIsTimeout(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(requestTimeout + 500*time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	start := time.Now()
	_, err := postWebhook(t.Context(), NewClient(), srv.URL, []byte(`{}`), make(http.Header))
	if err == nil {
		t.Fatal("expected timeout")
	}
	if !isTimeout(err) && !strings.Contains(strings.ToLower(err.Error()), "timeout") && !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("err = %v, want timeout", err)
	}
	if time.Since(start) > 8*time.Second {
		t.Fatalf("timeout took too long: %s", time.Since(start))
	}
}
