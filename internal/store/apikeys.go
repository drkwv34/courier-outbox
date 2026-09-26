package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/drkwv34/courier-outbox/internal/domain"
)

// CreateAPIKey inserts a key record. Only the prefix and hash are stored.
// A prefix collision returns domain.ErrConflict.
func (p *Postgres) CreateAPIKey(ctx context.Context, name, prefix string, hash []byte) (domain.APIKey, error) {
	const q = `
		INSERT INTO api_keys (name, prefix, key_hash)
		VALUES ($1, $2, $3)
		RETURNING id::text, name, prefix, key_hash, created_at, revoked_at`

	k, err := scanAPIKey(p.pool.QueryRow(ctx, q, name, prefix, hash))
	if err != nil {
		if isUniqueViolation(err) {
			return domain.APIKey{}, fmt.Errorf("store: insert api key: %w", domain.ErrConflict)
		}
		return domain.APIKey{}, fmt.Errorf("store: insert api key: %w", err)
	}
	return k, nil
}

// APIKeyByPrefix returns the key with the given prefix, including revoked
// keys; callers decide what revocation means. A missing prefix returns
// domain.ErrNotFound.
func (p *Postgres) APIKeyByPrefix(ctx context.Context, prefix string) (domain.APIKey, error) {
	const q = `
		SELECT id::text, name, prefix, key_hash, created_at, revoked_at
		FROM api_keys
		WHERE prefix = $1`

	k, err := scanAPIKey(p.pool.QueryRow(ctx, q, prefix))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.APIKey{}, fmt.Errorf("store: select api key: %w", domain.ErrNotFound)
		}
		return domain.APIKey{}, fmt.Errorf("store: select api key: %w", err)
	}
	return k, nil
}

func scanAPIKey(row pgx.Row) (domain.APIKey, error) {
	var k domain.APIKey
	err := row.Scan(&k.ID, &k.Name, &k.Prefix, &k.Hash, &k.CreatedAt, &k.RevokedAt)
	return k, err
}
