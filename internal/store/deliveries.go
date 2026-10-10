package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/drkwv34/courier-outbox/internal/domain"
)

const deliveryViewColumns = `
	d.id::text,
	d.event_id::text,
	d.subscription_id::text,
	d.status,
	d.attempt_count,
	d.next_attempt_at,
	d.lease_owner,
	d.lease_until,
	d.delivered_at,
	d.created_at,
	d.updated_at,
	e.type`

// ClaimDue locks up to limit due deliveries for workerID, sets the lease,
// and returns enough data to POST. The transaction is committed before return.
func (p *Postgres) ClaimDue(ctx context.Context, workerID string, lease time.Duration, limit int) ([]domain.DeliveryClaim, error) {
	if workerID == "" {
		return nil, fmt.Errorf("store: claim due: worker id is required")
	}
	if lease <= 0 {
		return nil, fmt.Errorf("store: claim due: lease must be positive")
	}
	if limit < 1 {
		return nil, fmt.Errorf("store: claim due: limit must be positive")
	}

	const q = `
		WITH due AS (
			SELECT d.id
			FROM deliveries d
			INNER JOIN subscriptions s ON s.id = d.subscription_id
			WHERE d.status IN ('pending', 'retrying')
			  AND d.next_attempt_at <= now()
			  AND (d.lease_until IS NULL OR d.lease_until < now())
			  AND s.enabled = true
			ORDER BY d.next_attempt_at
			FOR UPDATE OF d SKIP LOCKED
			LIMIT $3
		),
		claimed AS (
			UPDATE deliveries d
			SET lease_owner = $1,
			    lease_until = now() + ($2::double precision * interval '1 second'),
			    updated_at = now()
			FROM due
			WHERE d.id = due.id
			RETURNING d.id, d.event_id, d.subscription_id, d.status, d.attempt_count,
			          d.next_attempt_at, d.lease_owner, d.lease_until, d.delivered_at,
			          d.created_at, d.updated_at
		)
		SELECT
			claimed.id::text,
			claimed.event_id::text,
			claimed.subscription_id::text,
			claimed.status,
			claimed.attempt_count,
			claimed.next_attempt_at,
			claimed.lease_owner,
			claimed.lease_until,
			claimed.delivered_at,
			claimed.created_at,
			claimed.updated_at,
			e.payload,
			s.target_url,
			s.headers,
			s.signing_secret_enc
		FROM claimed
		INNER JOIN events e ON e.id = claimed.event_id
		INNER JOIN subscriptions s ON s.id = claimed.subscription_id`

	var out []domain.DeliveryClaim
	err := p.inTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, q, workerID, lease.Seconds(), limit)
		if err != nil {
			return fmt.Errorf("store: claim due: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			c, err := scanClaim(rows)
			if err != nil {
				return fmt.Errorf("store: claim due: %w", err)
			}
			out = append(out, c)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("store: claim due: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []domain.DeliveryClaim{}
	}
	return out, nil
}

// RecordAttempt inserts an attempt row and applies the fenced delivery
// update. Zero rows from the update is domain.ErrLeaseLost; the attempt is
// still stored.
func (p *Postgres) RecordAttempt(ctx context.Context, rec domain.AttemptRecord) error {
	lost := false
	err := p.inTx(ctx, func(tx pgx.Tx) error {
		const ins = `
			INSERT INTO delivery_attempts (delivery_id, status_code, error_message, duration_ms, request_id)
			VALUES ($1::uuid, $2, $3, $4, $5)`
		if _, err := tx.Exec(ctx, ins, rec.DeliveryID, rec.StatusCode, rec.ErrorMessage, rec.DurationMs, rec.RequestID); err != nil {
			return fmt.Errorf("store: insert attempt: %w", err)
		}

		var delaySecs any
		if rec.Status == domain.DeliveryRetrying {
			delaySecs = rec.RetryDelay.Seconds()
		}

		const upd = `
			UPDATE deliveries
			SET status = $3,
			    attempt_count = $4,
			    next_attempt_at = CASE
			        WHEN $5::double precision IS NULL THEN next_attempt_at
			        ELSE now() + ($5::double precision * interval '1 second')
			    END,
			    delivered_at = CASE WHEN $3 = 'delivered' THEN now() ELSE delivered_at END,
			    lease_owner = NULL,
			    lease_until = NULL,
			    updated_at = now()
			WHERE id = $1::uuid AND lease_owner = $2
			RETURNING id`
		var id string
		err := tx.QueryRow(ctx, upd, rec.DeliveryID, rec.WorkerID, string(rec.Status), rec.AttemptCount, delaySecs).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			lost = true
			return nil
		}
		if err != nil {
			return fmt.Errorf("store: update delivery after attempt: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if lost {
		return fmt.Errorf("store: record attempt: %w", domain.ErrLeaseLost)
	}
	return nil
}

// ListDeliveries returns a page of deliveries owned by apiKeyID.
func (p *Postgres) ListDeliveries(ctx context.Context, apiKeyID string, filter domain.DeliveryFilter) (domain.DeliveryPage, error) {
	limit := filter.LimitOrDefault()
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	var status any
	if filter.Status != "" {
		status = string(filter.Status)
	}
	var subID any
	if filter.SubscriptionID != "" {
		subID = filter.SubscriptionID
	}
	var eventType any
	if filter.EventType != "" {
		eventType = filter.EventType
	}

	const where = `
		FROM deliveries d
		INNER JOIN events e ON e.id = d.event_id
		WHERE e.api_key_id = $1::uuid
		  AND ($2::text IS NULL OR d.status = $2)
		  AND ($3::uuid IS NULL OR d.subscription_id = $3::uuid)
		  AND ($4::text IS NULL OR e.type = $4)
		  AND ($5::timestamptz IS NULL OR d.created_at >= $5)
		  AND ($6::timestamptz IS NULL OR d.created_at < $6)`

	var total int
	countQ := `SELECT count(*)` + where
	if err := p.pool.QueryRow(ctx, countQ, apiKeyID, status, subID, eventType, filter.CreatedAfter, filter.CreatedBefore).Scan(&total); err != nil {
		return domain.DeliveryPage{}, fmt.Errorf("store: count deliveries: %w", err)
	}

	listQ := `SELECT` + deliveryViewColumns + where + `
		ORDER BY d.created_at DESC, d.id DESC
		LIMIT $7 OFFSET $8`
	rows, err := p.pool.Query(ctx, listQ, apiKeyID, status, subID, eventType, filter.CreatedAfter, filter.CreatedBefore, limit, offset)
	if err != nil {
		return domain.DeliveryPage{}, fmt.Errorf("store: list deliveries: %w", err)
	}
	defer rows.Close()

	items := make([]domain.DeliveryView, 0)
	for rows.Next() {
		v, err := scanDeliveryView(rows)
		if err != nil {
			return domain.DeliveryPage{}, fmt.Errorf("store: list deliveries: %w", err)
		}
		items = append(items, v)
	}
	if err := rows.Err(); err != nil {
		return domain.DeliveryPage{}, fmt.Errorf("store: list deliveries: %w", err)
	}
	return domain.DeliveryPage{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}

// GetDelivery returns an owned delivery and its attempts oldest-first.
func (p *Postgres) GetDelivery(ctx context.Context, apiKeyID, id string) (domain.DeliveryView, []domain.Attempt, error) {
	const q = `
		SELECT` + deliveryViewColumns + `
		FROM deliveries d
		INNER JOIN events e ON e.id = d.event_id
		WHERE d.id = $1::uuid AND e.api_key_id = $2::uuid`

	view, err := scanDeliveryView(p.pool.QueryRow(ctx, q, id, apiKeyID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.DeliveryView{}, nil, fmt.Errorf("store: select delivery: %w", domain.ErrNotFound)
		}
		return domain.DeliveryView{}, nil, fmt.Errorf("store: select delivery: %w", err)
	}

	const aq = `
		SELECT delivery_id::text, status_code, error_message, duration_ms, COALESCE(request_id, ''), created_at
		FROM delivery_attempts
		WHERE delivery_id = $1::uuid
		ORDER BY created_at ASC, id ASC`
	rows, err := p.pool.Query(ctx, aq, id)
	if err != nil {
		return domain.DeliveryView{}, nil, fmt.Errorf("store: select attempts: %w", err)
	}
	defer rows.Close()

	attempts := make([]domain.Attempt, 0)
	for rows.Next() {
		a, err := scanAttempt(rows)
		if err != nil {
			return domain.DeliveryView{}, nil, fmt.Errorf("store: select attempts: %w", err)
		}
		attempts = append(attempts, a)
	}
	if err := rows.Err(); err != nil {
		return domain.DeliveryView{}, nil, fmt.Errorf("store: select attempts: %w", err)
	}
	return view, attempts, nil
}

// ReplayDelivery moves an owned dead-lettered delivery back to pending.
func (p *Postgres) ReplayDelivery(ctx context.Context, apiKeyID, id string) (domain.DeliveryView, error) {
	var out domain.DeliveryView
	err := p.inTx(ctx, func(tx pgx.Tx) error {
		const sel = `
			SELECT` + deliveryViewColumns + `
			FROM deliveries d
			INNER JOIN events e ON e.id = d.event_id
			WHERE d.id = $1::uuid AND e.api_key_id = $2::uuid
			FOR UPDATE OF d`
		view, err := scanDeliveryView(tx.QueryRow(ctx, sel, id, apiKeyID))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("store: replay delivery: %w", domain.ErrNotFound)
			}
			return fmt.Errorf("store: replay delivery: %w", err)
		}
		if _, err := view.Replay(); err != nil {
			return fmt.Errorf("store: replay delivery: %w", err)
		}

		const upd = `
			UPDATE deliveries d
			SET status = 'pending',
			    next_attempt_at = now(),
			    lease_owner = NULL,
			    lease_until = NULL,
			    updated_at = now()
			FROM events e
			WHERE d.id = $1::uuid
			  AND d.status = 'dead_lettered'
			  AND e.id = d.event_id
			  AND e.api_key_id = $2::uuid
			RETURNING` + deliveryViewColumns
		out, err = scanDeliveryView(tx.QueryRow(ctx, upd, id, apiKeyID))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("store: replay delivery: %w", domain.ErrInvalidTransition)
			}
			return fmt.Errorf("store: replay delivery: %w", err)
		}
		return nil
	})
	if err != nil {
		return domain.DeliveryView{}, err
	}
	return out, nil
}

func scanClaim(row pgx.Row) (domain.DeliveryClaim, error) {
	var c domain.DeliveryClaim
	var status string
	var leaseOwner *string
	var leaseUntil, deliveredAt *time.Time
	var headersJSON []byte
	if err := row.Scan(
		&c.Delivery.ID,
		&c.Delivery.EventID,
		&c.Delivery.SubscriptionID,
		&status,
		&c.Delivery.AttemptCount,
		&c.Delivery.NextAttemptAt,
		&leaseOwner,
		&leaseUntil,
		&deliveredAt,
		&c.Delivery.CreatedAt,
		&c.Delivery.UpdatedAt,
		&c.Payload,
		&c.TargetURL,
		&headersJSON,
		&c.SigningSecretEnc,
	); err != nil {
		return domain.DeliveryClaim{}, err
	}
	c.Delivery.Status = domain.DeliveryStatus(status)
	if leaseOwner != nil {
		c.Delivery.LeaseOwner = *leaseOwner
	}
	if leaseUntil != nil {
		c.Delivery.LeaseUntil = *leaseUntil
	}
	if deliveredAt != nil {
		c.Delivery.DeliveredAt = *deliveredAt
	}
	c.Headers = map[string]string{}
	if len(headersJSON) > 0 {
		if err := json.Unmarshal(headersJSON, &c.Headers); err != nil {
			return domain.DeliveryClaim{}, fmt.Errorf("unmarshal headers: %w", err)
		}
	}
	return c, nil
}

func scanDeliveryView(row pgx.Row) (domain.DeliveryView, error) {
	var v domain.DeliveryView
	var status string
	var leaseOwner *string
	var leaseUntil, deliveredAt *time.Time
	if err := row.Scan(
		&v.ID,
		&v.EventID,
		&v.SubscriptionID,
		&status,
		&v.AttemptCount,
		&v.NextAttemptAt,
		&leaseOwner,
		&leaseUntil,
		&deliveredAt,
		&v.CreatedAt,
		&v.UpdatedAt,
		&v.EventType,
	); err != nil {
		return domain.DeliveryView{}, err
	}
	v.Status = domain.DeliveryStatus(status)
	if leaseOwner != nil {
		v.LeaseOwner = *leaseOwner
	}
	if leaseUntil != nil {
		v.LeaseUntil = *leaseUntil
	}
	if deliveredAt != nil {
		v.DeliveredAt = *deliveredAt
	}
	return v, nil
}

func scanAttempt(row pgx.Row) (domain.Attempt, error) {
	var a domain.Attempt
	if err := row.Scan(&a.DeliveryID, &a.StatusCode, &a.ErrorMessage, &a.DurationMs, &a.RequestID, &a.CreatedAt); err != nil {
		return domain.Attempt{}, err
	}
	return a, nil
}
