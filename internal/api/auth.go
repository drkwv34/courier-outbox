package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/drkwv34/courier-outbox/internal/domain"
)

// APIKeyLookup finds a stored API key by its public prefix. Implementations
// return domain.ErrNotFound for unknown prefixes.
type APIKeyLookup interface {
	APIKeyByPrefix(ctx context.Context, prefix string) (domain.APIKey, error)
}

type ctxKey int

const apiKeyIDKey ctxKey = iota

// APIKeyID returns the authenticated API key id stored by the auth
// middleware. Every tenant-owned query must filter by it (FR-AUTH-003).
func APIKeyID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(apiKeyIDKey).(string)
	return id, ok && id != ""
}

// requireAPIKey authenticates "Authorization: Bearer <api_key>". Every
// rejection is the same 401 so callers cannot tell which check failed.
func requireAPIKey(logger *slog.Logger, keys APIKeyLookup, hasher domain.APIKeyHasher) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok {
				unauthorized(w, logger)
				return
			}
			prefix, err := domain.ParseAPIKeyPrefix(raw)
			if err != nil {
				unauthorized(w, logger)
				return
			}

			key, err := keys.APIKeyByPrefix(r.Context(), prefix)
			switch {
			case errors.Is(err, domain.ErrNotFound):
				unauthorized(w, logger)
				return
			case err != nil:
				logger.ErrorContext(r.Context(), "authenticate api key",
					slog.String("request_id", middleware.GetReqID(r.Context())),
					slog.String("api_key_prefix", prefix),
					slog.Any("err", err))
				writeError(w, logger, http.StatusInternalServerError, "internal", "internal error")
				return
			}
			if key.Revoked() || !hasher.Verify(raw, key.Hash) {
				unauthorized(w, logger)
				return
			}

			ctx := context.WithValue(r.Context(), apiKeyIDKey, key.ID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func bearerToken(header string) (string, bool) {
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	return token, token != ""
}

func unauthorized(w http.ResponseWriter, logger *slog.Logger) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="courier"`)
	writeError(w, logger, http.StatusUnauthorized, "unauthorized", "missing or invalid api key")
}
