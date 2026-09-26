package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/drkwv34/courier-outbox/migrations"
)

// Migrate applies every pending embedded migration to the database at dsn
// and returns the versions it applied, in order. An up-to-date database
// yields an empty slice.
func Migrate(ctx context.Context, dsn string) ([]int64, error) {
	connCfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("store: parse database url: %w", err)
	}
	db := stdlib.OpenDB(*connCfg)
	defer func() { _ = db.Close() }()

	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		return nil, fmt.Errorf("store: init migrations: %w", err)
	}
	results, err := provider.Up(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: apply migrations: %w", err)
	}

	applied := make([]int64, 0, len(results))
	for _, r := range results {
		applied = append(applied, r.Source.Version)
	}
	return applied, nil
}
