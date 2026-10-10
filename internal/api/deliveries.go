package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/drkwv34/courier-outbox/internal/domain"
)

// DeliveryStore is the persistence port used by delivery handlers.
type DeliveryStore interface {
	ListDeliveries(ctx context.Context, apiKeyID string, filter domain.DeliveryFilter) (domain.DeliveryPage, error)
	GetDelivery(ctx context.Context, apiKeyID, id string) (domain.DeliveryView, []domain.Attempt, error)
	ReplayDelivery(ctx context.Context, apiKeyID, id string) (domain.DeliveryView, error)
}

type deliveryHandlers struct {
	logger *slog.Logger
	store  DeliveryStore
}

type deliveryResponse struct {
	ID             string            `json:"id"`
	EventID        string            `json:"event_id"`
	SubscriptionID string            `json:"subscription_id"`
	EventType      string            `json:"event_type"`
	Status         string            `json:"status"`
	AttemptCount   int               `json:"attempt_count"`
	NextAttemptAt  time.Time         `json:"next_attempt_at"`
	DeliveredAt    *time.Time        `json:"delivered_at,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	Attempts       []attemptResponse `json:"attempts,omitempty"`
}

type attemptResponse struct {
	StatusCode   *int      `json:"status_code"`
	ErrorMessage *string   `json:"error_message"`
	DurationMs   int       `json:"duration_ms"`
	RequestID    string    `json:"request_id"`
	CreatedAt    time.Time `json:"created_at"`
}

type deliveryListResponse struct {
	Items  []deliveryResponse `json:"items"`
	Limit  int                `json:"limit"`
	Offset int                `json:"offset"`
	Total  int                `json:"total"`
}

func (h deliveryHandlers) list(w http.ResponseWriter, r *http.Request) {
	apiKeyID, ok := CallerKeyID(r.Context())
	if !ok {
		writeError(w, h.logger, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	filter, err := parseDeliveryFilter(r)
	if err != nil {
		writeDomainError(w, h.logger, r, err)
		return
	}
	page, err := h.store.ListDeliveries(r.Context(), apiKeyID, filter)
	if err != nil {
		writeDomainError(w, h.logger, r, err)
		return
	}
	writeJSON(w, h.logger, http.StatusOK, toDeliveryList(page))
}

func (h deliveryHandlers) get(w http.ResponseWriter, r *http.Request) {
	apiKeyID, ok := CallerKeyID(r.Context())
	if !ok {
		writeError(w, h.logger, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	id, err := domain.ParseDeliveryID(chi.URLParam(r, "id"))
	if err != nil {
		writeDomainError(w, h.logger, r, err)
		return
	}
	view, attempts, err := h.store.GetDelivery(r.Context(), apiKeyID, id)
	if err != nil {
		writeDomainError(w, h.logger, r, err)
		return
	}
	writeJSON(w, h.logger, http.StatusOK, toDeliveryDetail(view, attempts))
}

func (h deliveryHandlers) replay(w http.ResponseWriter, r *http.Request) {
	apiKeyID, ok := CallerKeyID(r.Context())
	if !ok {
		writeError(w, h.logger, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	id, err := domain.ParseDeliveryID(chi.URLParam(r, "id"))
	if err != nil {
		writeDomainError(w, h.logger, r, err)
		return
	}
	view, err := h.store.ReplayDelivery(r.Context(), apiKeyID, id)
	if err != nil {
		writeDomainError(w, h.logger, r, err)
		return
	}
	h.logger.InfoContext(r.Context(), "delivery replayed",
		slog.String("request_id", middleware.GetReqID(r.Context())),
		slog.String("api_key_id", apiKeyID),
		slog.String("delivery_id", id))
	writeJSON(w, h.logger, http.StatusAccepted, toDeliveryResponse(view, nil))
}

func parseDeliveryFilter(r *http.Request) (domain.DeliveryFilter, error) {
	q := r.URL.Query()
	var f domain.DeliveryFilter
	if raw := q.Get("status"); raw != "" {
		s := domain.DeliveryStatus(raw)
		if !s.Valid() {
			return domain.DeliveryFilter{}, &domain.ValidationError{Field: "status", Reason: "must be a delivery status"}
		}
		f.Status = s
	}
	if raw := q.Get("subscription_id"); raw != "" {
		id, err := domain.ParseSubscriptionID(raw)
		if err != nil {
			return domain.DeliveryFilter{}, &domain.ValidationError{Field: "subscription_id", Reason: "must be a uuid"}
		}
		f.SubscriptionID = id
	}
	if raw := q.Get("type"); raw != "" {
		t, err := domain.ParseEventType(raw)
		if err != nil {
			return domain.DeliveryFilter{}, err
		}
		f.EventType = t
	}
	if raw := q.Get("created_after"); raw != "" {
		t, err := parseQueryTime("created_after", raw)
		if err != nil {
			return domain.DeliveryFilter{}, err
		}
		f.CreatedAfter = &t
	}
	if raw := q.Get("created_before"); raw != "" {
		t, err := parseQueryTime("created_before", raw)
		if err != nil {
			return domain.DeliveryFilter{}, err
		}
		f.CreatedBefore = &t
	}
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > domain.MaxDeliveryLimit {
			return domain.DeliveryFilter{}, &domain.ValidationError{Field: "limit", Reason: "must be 1-100"}
		}
		f.Limit = n
	}
	if raw := q.Get("offset"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return domain.DeliveryFilter{}, &domain.ValidationError{Field: "offset", Reason: "must be >= 0"}
		}
		f.Offset = n
	}
	return f, nil
}

func parseQueryTime(field, raw string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, &domain.ValidationError{Field: field, Reason: "must be rfc 3339"}
	}
	return t.UTC(), nil
}

func toDeliveryList(page domain.DeliveryPage) deliveryListResponse {
	items := make([]deliveryResponse, 0, len(page.Items))
	for _, v := range page.Items {
		items = append(items, toDeliveryResponse(v, nil))
	}
	return deliveryListResponse{Items: items, Limit: page.Limit, Offset: page.Offset, Total: page.Total}
}

func toDeliveryDetail(v domain.DeliveryView, attempts []domain.Attempt) deliveryResponse {
	out := toDeliveryResponse(v, attempts)
	if out.Attempts == nil {
		out.Attempts = []attemptResponse{}
	}
	return out
}

func toDeliveryResponse(v domain.DeliveryView, attempts []domain.Attempt) deliveryResponse {
	out := deliveryResponse{
		ID:             v.ID,
		EventID:        v.EventID,
		SubscriptionID: v.SubscriptionID,
		EventType:      v.EventType,
		Status:         string(v.Status),
		AttemptCount:   v.AttemptCount,
		NextAttemptAt:  v.NextAttemptAt.UTC(),
		CreatedAt:      v.CreatedAt.UTC(),
		UpdatedAt:      v.UpdatedAt.UTC(),
	}
	if !v.DeliveredAt.IsZero() {
		t := v.DeliveredAt.UTC()
		out.DeliveredAt = &t
	}
	if attempts != nil {
		out.Attempts = make([]attemptResponse, 0, len(attempts))
		for _, a := range attempts {
			out.Attempts = append(out.Attempts, attemptResponse{
				StatusCode:   a.StatusCode,
				ErrorMessage: a.ErrorMessage,
				DurationMs:   a.DurationMs,
				RequestID:    a.RequestID,
				CreatedAt:    a.CreatedAt.UTC(),
			})
		}
	}
	return out
}
