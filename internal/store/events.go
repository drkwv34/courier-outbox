package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/drkwv34/courier-outbox/internal/domain"
)

const enqueueConflictRetries = 8

var errEnqueueConflictRetry = errors.New("enqueue conflict retry")

const eventColumns = `id::text, api_key_id::text, type, payload, idempotency_key, occurred_at, created_at`
const deliveryColumns = `id::text, event_id::text, subscription_id::text, status, attempt_count, next_attempt_at, created_at, updated_at`

// EnqueueEvent inserts an event and pending deliveries for matching enabled
// subscriptions in one transaction. A duplicate (api_key_id, idempotency_key)
// returns the original ids with Replay true.
func (p *Postgres) EnqueueEvent(ctx context.Context, e domain.Event) (domain.EnqueueResult, error) {
	var last error
	for range enqueueConflictRetries {
		res, err := p.enqueueOnce(ctx, e)
		if err == nil {
			return res, nil
		}
		if !errors.Is(err, errEnqueueConflictRetry) {
			return domain.EnqueueResult{}, err
		}
		last = err
	}
	return domain.EnqueueResult{}, fmt.Errorf("store: enqueue event: %w", last)
}

func (p *Postgres) enqueueOnce(ctx context.Context, e domain.Event) (domain.EnqueueResult, error) {
	var out domain.EnqueueResult
	err := p.inTx(ctx, func(tx pgx.Tx) error {
		var occurredAt any
		if !e.OccurredAt.IsZero() {
			occurredAt = e.OccurredAt
		}

		const insertEvent = `
			INSERT INTO events (api_key_id, type, payload, idempotency_key, occurred_at)
			VALUES ($1::uuid, $2, $3::jsonb, $4, COALESCE($5::timestamptz, now()))
			ON CONFLICT (api_key_id, idempotency_key) DO NOTHING
			RETURNING id::text`

		var id string
		err := tx.QueryRow(ctx, insertEvent, e.APIKeyID, e.Type, e.Payload, e.IdempotencyKey, occurredAt).Scan(&id)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			if isUniqueViolation(err) {
				return errEnqueueConflictRetry
			}
			return fmt.Errorf("store: insert event: %w", err)
		}
		if errors.Is(err, pgx.ErrNoRows) || id == "" {
			eventID, deliveryIDs, lookupErr := lookupEnqueue(ctx, tx, e.APIKeyID, e.IdempotencyKey)
			if lookupErr != nil {
				return lookupErr
			}
			if eventID == "" {
				return errEnqueueConflictRetry
			}
			out = domain.EnqueueResult{EventID: eventID, DeliveryIDs: deliveryIDs, Replay: true}
			return nil
		}

		deliveryIDs, err := insertMatchingDeliveries(ctx, tx, id, e.APIKeyID, e.Type)
		if err != nil {
			return err
		}
		out = domain.EnqueueResult{EventID: id, DeliveryIDs: deliveryIDs, Replay: false}
		return nil
	})
	if err != nil {
		return domain.EnqueueResult{}, err
	}
	return out, nil
}

func insertMatchingDeliveries(ctx context.Context, tx pgx.Tx, eventID, apiKeyID, eventType string) ([]string, error) {
	const q = `
		SELECT ` + subscriptionColumns + `
		FROM subscriptions
		WHERE api_key_id = $1::uuid AND enabled = true`

	rows, err := tx.Query(ctx, q, apiKeyID)
	if err != nil {
		return nil, fmt.Errorf("store: list matching subscriptions: %w", err)
	}
	defer rows.Close()

	var matching []domain.Subscription
	for rows.Next() {
		s, err := scanSubscription(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list matching subscriptions: %w", err)
		}
		if domain.MatchesEventType(s.EventTypes, eventType) {
			matching = append(matching, s)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list matching subscriptions: %w", err)
	}
	rows.Close()

	ids := make([]string, 0, len(matching))
	const ins = `
		INSERT INTO deliveries (event_id, subscription_id, status, attempt_count, next_attempt_at)
		VALUES ($1::uuid, $2::uuid, 'pending', 0, now())
		RETURNING id::text`
	for _, s := range matching {
		var did string
		if err := tx.QueryRow(ctx, ins, eventID, s.ID).Scan(&did); err != nil {
			return nil, fmt.Errorf("store: insert delivery: %w", err)
		}
		ids = append(ids, did)
	}
	return ids, nil
}

func lookupEnqueue(ctx context.Context, tx pgx.Tx, apiKeyID, idempotencyKey string) (string, []string, error) {
	const q = `
		SELECT id::text
		FROM events
		WHERE api_key_id = $1::uuid AND idempotency_key = $2`

	var eventID string
	err := tx.QueryRow(ctx, q, apiKeyID, idempotencyKey).Scan(&eventID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil, nil
		}
		return "", nil, fmt.Errorf("store: select event by idempotency key: %w", err)
	}

	const dq = `
		SELECT id::text
		FROM deliveries
		WHERE event_id = $1::uuid
		ORDER BY created_at ASC`

	rows, err := tx.Query(ctx, dq, eventID)
	if err != nil {
		return "", nil, fmt.Errorf("store: select deliveries: %w", err)
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", nil, fmt.Errorf("store: select deliveries: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return "", nil, fmt.Errorf("store: select deliveries: %w", err)
	}
	return eventID, ids, nil
}

// GetEvent returns an event owned by apiKeyID and its deliveries.
func (p *Postgres) GetEvent(ctx context.Context, apiKeyID, id string) (domain.Event, []domain.Delivery, error) {
	const q = `
		SELECT ` + eventColumns + `
		FROM events
		WHERE id = $1::uuid AND api_key_id = $2::uuid`

	e, err := scanEvent(p.pool.QueryRow(ctx, q, id, apiKeyID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Event{}, nil, fmt.Errorf("store: select event: %w", domain.ErrNotFound)
		}
		return domain.Event{}, nil, fmt.Errorf("store: select event: %w", err)
	}

	const dq = `
		SELECT ` + deliveryColumns + `
		FROM deliveries
		WHERE event_id = $1::uuid
		ORDER BY created_at ASC`

	rows, err := p.pool.Query(ctx, dq, id)
	if err != nil {
		return domain.Event{}, nil, fmt.Errorf("store: select deliveries: %w", err)
	}
	defer rows.Close()

	dels := make([]domain.Delivery, 0)
	for rows.Next() {
		d, err := scanDelivery(rows)
		if err != nil {
			return domain.Event{}, nil, fmt.Errorf("store: select deliveries: %w", err)
		}
		dels = append(dels, d)
	}
	if err := rows.Err(); err != nil {
		return domain.Event{}, nil, fmt.Errorf("store: select deliveries: %w", err)
	}
	return e, dels, nil
}

func scanEvent(row pgx.Row) (domain.Event, error) {
	var e domain.Event
	if err := row.Scan(&e.ID, &e.APIKeyID, &e.Type, &e.Payload, &e.IdempotencyKey, &e.OccurredAt, &e.CreatedAt); err != nil {
		return domain.Event{}, err
	}
	return e, nil
}

func scanDelivery(row pgx.Row) (domain.Delivery, error) {
	var d domain.Delivery
	var status string
	if err := row.Scan(&d.ID, &d.EventID, &d.SubscriptionID, &status, &d.AttemptCount, &d.NextAttemptAt, &d.CreatedAt, &d.UpdatedAt); err != nil {
		return domain.Delivery{}, err
	}
	d.Status = domain.DeliveryStatus(status)
	return d, nil
}
