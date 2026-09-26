package api

import (
	"log/slog"
	"net/http"
)

// healthz is the liveness probe. It must not touch dependencies; readiness
// (Postgres + Redis) belongs to /readyz.
func healthz(logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, logger, http.StatusOK, map[string]string{"status": "ok"})
	}
}
