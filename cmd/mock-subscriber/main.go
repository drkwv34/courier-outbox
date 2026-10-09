// Command mock-subscriber is a local-only webhook target for Docker Compose
// demos and smoke tests. It logs each request (including signature headers)
// and replies with a fixed status. It does not verify HMAC; integration
// tests use httptest servers that do.
package main

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"
)

const maxBody = 256 << 10

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	addr := os.Getenv("MOCK_ADDR")
	if addr == "" {
		addr = ":9090"
	}
	status := http.StatusOK
	if v := os.Getenv("MOCK_RESPONSE_STATUS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 100 || n > 599 {
			fmt.Fprintf(os.Stderr, "mock-subscriber: invalid MOCK_RESPONSE_STATUS %q\n", v)
			os.Exit(1)
		}
		status = n
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		n, err := io.Copy(io.Discard, io.LimitReader(r.Body, maxBody))
		logger.Info("webhook received",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int64("body_bytes", n),
			slog.String("idempotency_key", r.Header.Get("X-Courier-Idempotency-Key")),
			slog.String("timestamp", r.Header.Get("X-Courier-Timestamp")),
			slog.Bool("has_signature", r.Header.Get("X-Courier-Signature") != ""),
			slog.Any("read_err", err),
		)
		w.WriteHeader(status)
	})

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	logger.Info("mock-subscriber listening", slog.String("addr", addr), slog.Int("response_status", status))
	if err := srv.ListenAndServe(); err != nil {
		logger.Error("server stopped", slog.Any("err", err))
		os.Exit(1)
	}
}
