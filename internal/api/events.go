package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/drkwv34/courier-outbox/internal/domain"
)

const (
	idempotentReplayHeader = "Idempotent-Replay"
	idempotentReplayValue  = "true"
)

// EventStore is the persistence port used by event handlers.
type EventStore interface {
	EnqueueEvent(ctx context.Context, e domain.Event) (domain.EnqueueResult, error)
	GetEvent(ctx context.Context, apiKeyID, id string) (domain.Event, []domain.Delivery, error)
}

type eventHandlers struct {
	logger *slog.Logger
	store  EventStore
}

type createEventRequest struct {
	Type           string          `json:"type"`
	Payload        json.RawMessage `json:"payload"`
	IdempotencyKey string          `json:"idempotency_key"`
	OccurredAt     *string         `json:"occurred_at"`
}

type enqueueResponse struct {
	EventID     string   `json:"event_id"`
	DeliveryIDs []string `json:"delivery_ids"`
}

type deliverySummary struct {
	ID             string    `json:"id"`
	SubscriptionID string    `json:"subscription_id"`
	Status         string    `json:"status"`
	AttemptCount   int       `json:"attempt_count"`
	NextAttemptAt  time.Time `json:"next_attempt_at"`
}

type eventResponse struct {
	ID             string            `json:"id"`
	Type           string            `json:"type"`
	Payload        json.RawMessage   `json:"payload"`
	IdempotencyKey string            `json:"idempotency_key"`
	OccurredAt     time.Time         `json:"occurred_at"`
	CreatedAt      time.Time         `json:"created_at"`
	Deliveries     []deliverySummary `json:"deliveries"`
}

func (h eventHandlers) create(w http.ResponseWriter, r *http.Request) {
	apiKeyID, ok := CallerKeyID(r.Context())
	if !ok {
		writeError(w, h.logger, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	var req createEventRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeDomainError(w, h.logger, r, err)
		return
	}
	var occurred *time.Time
	if req.OccurredAt != nil {
		t, err := domain.ParseOccurredAt(*req.OccurredAt)
		if err != nil {
			writeDomainError(w, h.logger, r, err)
			return
		}
		occurred = &t
	}
	ev, err := domain.NewEvent(apiKeyID, req.Type, req.Payload, req.IdempotencyKey, occurred)
	if err != nil {
		writeDomainError(w, h.logger, r, err)
		return
	}
	res, err := h.store.EnqueueEvent(r.Context(), ev)
	if err != nil {
		writeDomainError(w, h.logger, r, err)
		return
	}
	h.logger.InfoContext(r.Context(), "event enqueued",
		slog.String("request_id", middleware.GetReqID(r.Context())),
		slog.String("api_key_id", apiKeyID),
		slog.String("event_id", res.EventID),
		slog.Bool("replay", res.Replay),
		slog.Int("payload_bytes", len(ev.Payload)))

	body := enqueueResponse{EventID: res.EventID, DeliveryIDs: res.DeliveryIDs}
	if body.DeliveryIDs == nil {
		body.DeliveryIDs = []string{}
	}
	if res.Replay {
		setIdempotentReplay(w)
		writeJSON(w, h.logger, http.StatusOK, body)
		return
	}
	writeJSON(w, h.logger, http.StatusCreated, body)
}

func (h eventHandlers) get(w http.ResponseWriter, r *http.Request) {
	apiKeyID, ok := CallerKeyID(r.Context())
	if !ok {
		writeError(w, h.logger, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	id, err := domain.ParseEventID(chi.URLParam(r, "id"))
	if err != nil {
		writeDomainError(w, h.logger, r, err)
		return
	}
	ev, dels, err := h.store.GetEvent(r.Context(), apiKeyID, id)
	if err != nil {
		writeDomainError(w, h.logger, r, err)
		return
	}
	writeJSON(w, h.logger, http.StatusOK, toEventResponse(ev, dels))
}

func setIdempotentReplay(w http.ResponseWriter) {
	w.Header().Set(idempotentReplayHeader, idempotentReplayValue)
}

func toEventResponse(e domain.Event, dels []domain.Delivery) eventResponse {
	summaries := make([]deliverySummary, 0, len(dels))
	for _, d := range dels {
		summaries = append(summaries, deliverySummary{
			ID:             d.ID,
			SubscriptionID: d.SubscriptionID,
			Status:         string(d.Status),
			AttemptCount:   d.AttemptCount,
			NextAttemptAt:  d.NextAttemptAt.UTC(),
		})
	}
	return eventResponse{
		ID:             e.ID,
		Type:           e.Type,
		Payload:        json.RawMessage(e.Payload),
		IdempotencyKey: e.IdempotencyKey,
		OccurredAt:     e.OccurredAt.UTC(),
		CreatedAt:      e.CreatedAt.UTC(),
		Deliveries:     summaries,
	}
}
