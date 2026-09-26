// Package api is the HTTP transport layer: routing, middleware, request
// decoding, response encoding, and the single place where domain/store errors
// are mapped to HTTP status codes and the {code, message} error envelope.
package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter builds the root HTTP handler.
func NewRouter(logger *slog.Logger) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, logger, http.StatusNotFound, "not_found", "resource not found")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, logger, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	})

	r.Get("/healthz", healthz(logger))

	return r
}
