package store

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// Redis wraps the client used for leases and rate limits. Redis is an
// accelerator, never the only copy of state (ADR 0001).
type Redis struct {
	client *redis.Client
}

// OpenRedis creates a client for url (redis:// or rediss://). It does not
// dial eagerly.
func OpenRedis(url string) (*Redis, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("store: parse redis url: %w", err)
	}
	return &Redis{client: redis.NewClient(opts)}, nil
}

// Ping checks that Redis answers.
func (r *Redis) Ping(ctx context.Context) error {
	if err := r.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("store: ping redis: %w", err)
	}
	return nil
}

// Close releases the client's connections.
func (r *Redis) Close() error { return r.client.Close() }
