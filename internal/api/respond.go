package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// errorBody is the stable error envelope (SRS FR-ERR-001). Code is a
// machine-readable snake_case identifier; Message is safe to show to clients.
type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, logger *slog.Logger, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		logger.Warn("encode response", slog.Any("err", err))
	}
}

func writeError(w http.ResponseWriter, logger *slog.Logger, status int, code, message string) {
	writeJSON(w, logger, status, errorBody{Code: code, Message: message})
}
