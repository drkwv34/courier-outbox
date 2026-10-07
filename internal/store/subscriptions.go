package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/drkwv34/courier-outbox/internal/domain"
)

const subscriptionColumns = `id::text, api_key_id::text, target_url, event_types, headers, enabled, description, created_at, updated_at`

// CreateSubscription inserts a subscription. secretEnc is the AES-GCM blob;
// plaintext is never stored.
func (p *Postgres) CreateSubscription(ctx context.Context, s domain.Subscription, secretEnc []byte) (domain.Subscription, error) {
	typesJSON, err := json.Marshal(s.EventTypes)
	if err != nil {
		return domain.Subscription{}, fmt.Errorf("store: marshal event_types: %w", err)
	}
	headers := s.Headers
	if headers == nil {
		headers = map[string]string{}
	}
	headersJSON, err := json.Marshal(headers)
	if err != nil {
		return domain.Subscription{}, fmt.Errorf("store: marshal headers: %w", err)
	}

	const q = `
		INSERT INTO subscriptions (api_key_id, target_url, event_types, headers, signing_secret_enc, enabled, description)
		VALUES ($1::uuid, $2, $3::jsonb, $4::jsonb, $5, $6, $7)
		RETURNING ` + subscriptionColumns

	out, err := scanSubscription(p.pool.QueryRow(ctx, q, s.APIKeyID, s.TargetURL, typesJSON, headersJSON, secretEnc, s.Enabled, s.Description))
	if err != nil {
		return domain.Subscription{}, fmt.Errorf("store: insert subscription: %w", err)
	}
	return out, nil
}

// ListSubscriptions returns the caller's subscriptions, oldest first.
func (p *Postgres) ListSubscriptions(ctx context.Context, apiKeyID string) ([]domain.Subscription, error) {
	const q = `
		SELECT ` + subscriptionColumns + `
		FROM subscriptions
		WHERE api_key_id = $1::uuid
		ORDER BY created_at ASC`

	rows, err := p.pool.Query(ctx, q, apiKeyID)
	if err != nil {
		return nil, fmt.Errorf("store: list subscriptions: %w", err)
	}
	defer rows.Close()

	out := make([]domain.Subscription, 0)
	for rows.Next() {
		s, err := scanSubscription(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list subscriptions: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list subscriptions: %w", err)
	}
	return out, nil
}

// GetSubscription returns one subscription owned by apiKeyID.
func (p *Postgres) GetSubscription(ctx context.Context, apiKeyID, id string) (domain.Subscription, error) {
	const q = `
		SELECT ` + subscriptionColumns + `
		FROM subscriptions
		WHERE id = $1::uuid AND api_key_id = $2::uuid`

	s, err := scanSubscription(p.pool.QueryRow(ctx, q, id, apiKeyID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Subscription{}, fmt.Errorf("store: select subscription: %w", domain.ErrNotFound)
		}
		return domain.Subscription{}, fmt.Errorf("store: select subscription: %w", err)
	}
	return s, nil
}

// UpdateSubscription applies a partial update. Zero rows is ErrNotFound.
func (p *Postgres) UpdateSubscription(ctx context.Context, apiKeyID, id string, u domain.SubscriptionUpdate) (domain.Subscription, error) {
	target := optionalString(u.TargetURL)
	typesJSON, err := optionalJSON(u.EventTypes)
	if err != nil {
		return domain.Subscription{}, fmt.Errorf("store: marshal event_types: %w", err)
	}
	headersJSON, err := optionalJSON(u.Headers)
	if err != nil {
		return domain.Subscription{}, fmt.Errorf("store: marshal headers: %w", err)
	}
	desc := optionalString(u.Description)
	var enabled any
	if u.Enabled != nil {
		enabled = *u.Enabled
	}
	var secret any
	if len(u.SecretEnc) > 0 {
		secret = u.SecretEnc
	}

	const q = `
		UPDATE subscriptions SET
			target_url = COALESCE($3, target_url),
			event_types = COALESCE($4::jsonb, event_types),
			headers = COALESCE($5::jsonb, headers),
			description = COALESCE($6, description),
			enabled = COALESCE($7, enabled),
			signing_secret_enc = COALESCE($8, signing_secret_enc),
			updated_at = now()
		WHERE id = $1::uuid AND api_key_id = $2::uuid
		RETURNING ` + subscriptionColumns

	s, err := scanSubscription(p.pool.QueryRow(ctx, q, id, apiKeyID, target, typesJSON, headersJSON, desc, enabled, secret))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Subscription{}, fmt.Errorf("store: update subscription: %w", domain.ErrNotFound)
		}
		return domain.Subscription{}, fmt.Errorf("store: update subscription: %w", err)
	}
	return s, nil
}

// DisableSubscription sets enabled=false. It is idempotent.
func (p *Postgres) DisableSubscription(ctx context.Context, apiKeyID, id string) (domain.Subscription, error) {
	enabled := false
	return p.UpdateSubscription(ctx, apiKeyID, id, domain.SubscriptionUpdate{Enabled: &enabled})
}

func optionalString(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

func optionalJSON[T any](p *T) (any, error) {
	if p == nil {
		return nil, nil
	}
	b, err := json.Marshal(*p)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func scanSubscription(row pgx.Row) (domain.Subscription, error) {
	var s domain.Subscription
	var typesJSON, headersJSON []byte
	if err := row.Scan(&s.ID, &s.APIKeyID, &s.TargetURL, &typesJSON, &headersJSON, &s.Enabled, &s.Description, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return domain.Subscription{}, err
	}
	if err := json.Unmarshal(typesJSON, &s.EventTypes); err != nil {
		return domain.Subscription{}, fmt.Errorf("unmarshal event_types: %w", err)
	}
	s.Headers = map[string]string{}
	if len(headersJSON) > 0 {
		if err := json.Unmarshal(headersJSON, &s.Headers); err != nil {
			return domain.Subscription{}, fmt.Errorf("unmarshal headers: %w", err)
		}
	}
	return s, nil
}
