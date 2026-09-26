// Package api is the HTTP transport layer: routing, middleware, request
// decoding, response encoding, and the single place where domain/store errors
// are mapped to HTTP status codes and the {code, message} error envelope.
package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/drkwv34/courier-outbox/internal/domain"
)

// Deps are the collaborators the HTTP layer needs.
type Deps struct {
	Logger *slog.Logger
	// Keys and Hasher authenticate every /v1 request.
	Keys   APIKeyLookup
	Hasher domain.APIKeyHasher
	// Readiness lists the dependencies /readyz probes.
	Readiness []ReadinessCheck
}

// NewRouter builds the root HTTP handler.
func NewRouter(d Deps) http.Handler {
	logger := d.Logger

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	notFound := func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, logger, http.StatusNotFound, "not_found", "resource not found")
	}
	methodNotAllowed := func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, logger, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
	r.NotFound(notFound)
	r.MethodNotAllowed(methodNotAllowed)

	r.Get("/healthz", healthz(logger))
	r.Get("/readyz", readyz(logger, d.Readiness))

	// The whole /v1 tree sits behind auth, including paths with no route, so
	// nothing under /v1 is reachable anonymously by accident.
	v1 := chi.NewRouter()
	v1.NotFound(notFound)
	v1.MethodNotAllowed(methodNotAllowed)
	r.Mount("/v1", requireAPIKey(logger, d.Keys, d.Hasher)(v1))

	return r
}
