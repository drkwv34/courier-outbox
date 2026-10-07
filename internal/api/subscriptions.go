package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/drkwv34/courier-outbox/internal/domain"
)

// SubscriptionStore is the persistence port used by subscription handlers.
type SubscriptionStore interface {
	CreateSubscription(ctx context.Context, s domain.Subscription, secretEnc []byte) (domain.Subscription, error)
	ListSubscriptions(ctx context.Context, apiKeyID string) ([]domain.Subscription, error)
	GetSubscription(ctx context.Context, apiKeyID, id string) (domain.Subscription, error)
	UpdateSubscription(ctx context.Context, apiKeyID, id string, u domain.SubscriptionUpdate) (domain.Subscription, error)
	DisableSubscription(ctx context.Context, apiKeyID, id string) (domain.Subscription, error)
}

type subscriptionHandlers struct {
	logger   *slog.Logger
	store    SubscriptionStore
	policy   domain.URLPolicy
	envelope domain.Envelope
	rand     io.Reader
}

type createSubscriptionRequest struct {
	TargetURL   string            `json:"target_url"`
	EventTypes  []string          `json:"event_types"`
	Headers     map[string]string `json:"headers"`
	Description string            `json:"description"`
}

type patchSubscriptionRequest struct {
	TargetURL    *string            `json:"target_url"`
	EventTypes   *[]string          `json:"event_types"`
	Headers      *map[string]string `json:"headers"`
	Description  *string            `json:"description"`
	Enabled      *bool              `json:"enabled"`
	RotateSecret *bool              `json:"rotate_secret"`
}

type subscriptionResponse struct {
	ID            string            `json:"id"`
	TargetURL     string            `json:"target_url"`
	EventTypes    []string          `json:"event_types"`
	Headers       map[string]string `json:"headers"`
	Enabled       bool              `json:"enabled"`
	Description   string            `json:"description"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	SigningSecret string            `json:"signing_secret,omitempty"`
}

type subscriptionListResponse struct {
	Items []subscriptionResponse `json:"items"`
}

func (h subscriptionHandlers) create(w http.ResponseWriter, r *http.Request) {
	apiKeyID, ok := CallerKeyID(r.Context())
	if !ok {
		writeError(w, h.logger, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	var req createSubscriptionRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeDomainError(w, h.logger, r, err)
		return
	}
	target, types, headers, desc, err := h.validateWrite(req.TargetURL, req.EventTypes, req.Headers, req.Description)
	if err != nil {
		writeDomainError(w, h.logger, r, err)
		return
	}
	secret, blob, err := h.newSecret()
	if err != nil {
		writeDomainError(w, h.logger, r, err)
		return
	}
	sub, err := h.store.CreateSubscription(r.Context(), domain.Subscription{
		APIKeyID:    apiKeyID,
		TargetURL:   target,
		EventTypes:  types,
		Headers:     headers,
		Enabled:     true,
		Description: desc,
	}, blob)
	if err != nil {
		writeDomainError(w, h.logger, r, err)
		return
	}
	writeJSON(w, h.logger, http.StatusCreated, toResponse(sub, secret))
}

func (h subscriptionHandlers) list(w http.ResponseWriter, r *http.Request) {
	apiKeyID, ok := CallerKeyID(r.Context())
	if !ok {
		writeError(w, h.logger, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	subs, err := h.store.ListSubscriptions(r.Context(), apiKeyID)
	if err != nil {
		writeDomainError(w, h.logger, r, err)
		return
	}
	items := make([]subscriptionResponse, 0, len(subs))
	for _, s := range subs {
		items = append(items, toResponse(s, nil))
	}
	writeJSON(w, h.logger, http.StatusOK, subscriptionListResponse{Items: items})
}

func (h subscriptionHandlers) get(w http.ResponseWriter, r *http.Request) {
	apiKeyID, id, ok := h.callerAndID(w, r)
	if !ok {
		return
	}
	sub, err := h.store.GetSubscription(r.Context(), apiKeyID, id)
	if err != nil {
		writeDomainError(w, h.logger, r, err)
		return
	}
	writeJSON(w, h.logger, http.StatusOK, toResponse(sub, nil))
}

func (h subscriptionHandlers) disable(w http.ResponseWriter, r *http.Request) {
	apiKeyID, id, ok := h.callerAndID(w, r)
	if !ok {
		return
	}
	sub, err := h.store.DisableSubscription(r.Context(), apiKeyID, id)
	if err != nil {
		writeDomainError(w, h.logger, r, err)
		return
	}
	writeJSON(w, h.logger, http.StatusOK, toResponse(sub, nil))
}

func (h subscriptionHandlers) patch(w http.ResponseWriter, r *http.Request) {
	apiKeyID, id, ok := h.callerAndID(w, r)
	if !ok {
		return
	}
	var req patchSubscriptionRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeDomainError(w, h.logger, r, err)
		return
	}
	var u domain.SubscriptionUpdate
	if req.TargetURL != nil {
		if err := h.policy.Validate(*req.TargetURL); err != nil {
			writeDomainError(w, h.logger, r, err)
			return
		}
		u.TargetURL = req.TargetURL
	}
	if req.EventTypes != nil {
		types, err := domain.NormalizeEventTypes(*req.EventTypes)
		if err != nil {
			writeDomainError(w, h.logger, r, err)
			return
		}
		u.EventTypes = &types
	}
	if req.Headers != nil {
		headers, err := domain.NormalizeHeaders(*req.Headers)
		if err != nil {
			writeDomainError(w, h.logger, r, err)
			return
		}
		u.Headers = &headers
	}
	if req.Description != nil {
		desc, err := domain.NormalizeDescription(*req.Description)
		if err != nil {
			writeDomainError(w, h.logger, r, err)
			return
		}
		u.Description = &desc
	}
	u.Enabled = req.Enabled

	var secret []byte
	if req.RotateSecret != nil && *req.RotateSecret {
		var err error
		secret, u.SecretEnc, err = h.newSecret()
		if err != nil {
			writeDomainError(w, h.logger, r, err)
			return
		}
	}
	sub, err := h.store.UpdateSubscription(r.Context(), apiKeyID, id, u)
	if err != nil {
		writeDomainError(w, h.logger, r, err)
		return
	}
	writeJSON(w, h.logger, http.StatusOK, toResponse(sub, secret))
}

func (h subscriptionHandlers) callerAndID(w http.ResponseWriter, r *http.Request) (apiKeyID, id string, ok bool) {
	apiKeyID, ok = CallerKeyID(r.Context())
	if !ok {
		writeError(w, h.logger, http.StatusInternalServerError, "internal", "internal error")
		return "", "", false
	}
	id, err := domain.ParseSubscriptionID(chi.URLParam(r, "id"))
	if err != nil {
		writeDomainError(w, h.logger, r, err)
		return "", "", false
	}
	return apiKeyID, id, true
}

func (h subscriptionHandlers) validateWrite(target string, types []string, headers map[string]string, desc string) (string, []string, map[string]string, string, error) {
	if err := h.policy.Validate(target); err != nil {
		return "", nil, nil, "", err
	}
	normTypes, err := domain.NormalizeEventTypes(types)
	if err != nil {
		return "", nil, nil, "", err
	}
	normHeaders, err := domain.NormalizeHeaders(headers)
	if err != nil {
		return "", nil, nil, "", err
	}
	normDesc, err := domain.NormalizeDescription(desc)
	if err != nil {
		return "", nil, nil, "", err
	}
	return target, normTypes, normHeaders, normDesc, nil
}

func (h subscriptionHandlers) newSecret() ([]byte, []byte, error) {
	r := h.rand
	if r == nil {
		r = rand.Reader
	}
	secret, err := domain.GenerateSigningSecret(r)
	if err != nil {
		return nil, nil, err
	}
	blob, err := h.envelope.Seal(r, secret)
	if err != nil {
		return nil, nil, err
	}
	return secret, blob, nil
}

func toResponse(s domain.Subscription, secret []byte) subscriptionResponse {
	headers := s.Headers
	if headers == nil {
		headers = map[string]string{}
	}
	types := s.EventTypes
	if types == nil {
		types = []string{}
	}
	out := subscriptionResponse{
		ID:          s.ID,
		TargetURL:   s.TargetURL,
		EventTypes:  types,
		Headers:     headers,
		Enabled:     s.Enabled,
		Description: s.Description,
		CreatedAt:   s.CreatedAt.UTC(),
		UpdatedAt:   s.UpdatedAt.UTC(),
	}
	if len(secret) > 0 {
		out.SigningSecret = base64.StdEncoding.EncodeToString(secret)
	}
	return out
}
