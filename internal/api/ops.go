package api

import (
	"log/slog"
	"net/http"
)

func serveOpenAPI(logger *slog.Logger, doc []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if len(doc) == 0 {
			writeError(w, logger, http.StatusNotFound, "not_found", "resource not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(doc); err != nil {
			logger.Warn("write openapi", slog.Any("err", err))
		}
	}
}
