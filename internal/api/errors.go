package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/drkwv34/courier-outbox/internal/domain"
)

func writeDomainError(w http.ResponseWriter, logger *slog.Logger, r *http.Request, err error) {
	var (
		maxErr *http.MaxBytesError
		ve     *domain.ValidationError
		syn    *json.SyntaxError
		typ    *json.UnmarshalTypeError
	)
	switch {
	case errors.As(err, &maxErr):
		writeError(w, logger, http.StatusRequestEntityTooLarge, "payload_too_large", "request body too large")
	case errors.As(err, &ve):
		writeError(w, logger, http.StatusBadRequest, "validation_failed", ve.Error())
	case errors.Is(err, domain.ErrValidation):
		writeError(w, logger, http.StatusBadRequest, "validation_failed", "validation failed")
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, logger, http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, domain.ErrConflict):
		writeError(w, logger, http.StatusConflict, "conflict", "conflict")
	case errors.Is(err, domain.ErrInvalidTransition):
		writeError(w, logger, http.StatusConflict, "invalid_transition", "delivery is not dead_lettered")
	case errors.As(err, &syn), errors.As(err, &typ), errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF), isUnknownJSONField(err):
		msg := "invalid json"
		if isUnknownJSONField(err) {
			msg = "unexpected field"
		}
		writeError(w, logger, http.StatusBadRequest, "validation_failed", msg)
	default:
		logger.ErrorContext(r.Context(), "request failed",
			slog.String("request_id", middleware.GetReqID(r.Context())),
			slog.Any("err", err))
		writeError(w, logger, http.StatusInternalServerError, "internal", "internal error")
	}
}
