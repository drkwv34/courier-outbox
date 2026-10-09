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
