package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres is the system-of-record adapter backed by a pgx connection pool.
type Postgres struct {
	pool *pgxpool.Pool
}

// OpenPostgres creates a connection pool for dsn. It does not dial eagerly,
// so the process can start while the database is still coming up; use Ping
// (or /readyz) to check connectivity.
func OpenPostgres(ctx context.Context, dsn string) (*Postgres, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("store: parse database url: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("store: open postgres pool: %w", err)
	}
	return &Postgres{pool: pool}, nil
}

// Ping checks that a connection can be acquired and the server answers.
func (p *Postgres) Ping(ctx context.Context) error {
	if err := p.pool.Ping(ctx); err != nil {
		return fmt.Errorf("store: ping postgres: %w", err)
	}
	return nil
}

// Close releases every pooled connection.
func (p *Postgres) Close() { p.pool.Close() }

const sqlStateUniqueViolation = "23505"

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == sqlStateUniqueViolation
}
