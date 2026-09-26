//go:build integration

// Package storetest starts disposable Postgres and Redis containers for
// integration tests (ADR 0005). It is compiled only with -tags=integration.
package storetest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

const (
	postgresImage = "postgres:16-alpine"
	redisImage    = "redis:7-alpine"
)

// Server is a running dependency container. Terminate it when the package's
// tests finish (typically from TestMain).
type Server struct {
	URL       string
	container testcontainers.Container
}

// Terminate stops and removes the container.
func (s *Server) Terminate(ctx context.Context) error {
	return testcontainers.TerminateContainer(s.container)
}

// StartPostgres starts a Postgres 16 container. URL points at its default
// database and is meant for creating per-test databases with FreshDatabase.
func StartPostgres(ctx context.Context) (*Server, error) {
	ctr, err := tcpostgres.Run(ctx, postgresImage,
		tcpostgres.WithDatabase("courier"),
		tcpostgres.WithUsername("courier"),
		tcpostgres.WithPassword("courier"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		return nil, fmt.Errorf("start postgres: %w", err)
	}
	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = testcontainers.TerminateContainer(ctr)
		return nil, fmt.Errorf("postgres dsn: %w", err)
	}
	return &Server{URL: dsn, container: ctr}, nil
}

// StartRedis starts a Redis 7 container.
func StartRedis(ctx context.Context) (*Server, error) {
	ctr, err := tcredis.Run(ctx, redisImage)
	if err != nil {
		return nil, fmt.Errorf("start redis: %w", err)
	}
	u, err := ctr.ConnectionString(ctx)
	if err != nil {
		_ = testcontainers.TerminateContainer(ctr)
		return nil, fmt.Errorf("redis url: %w", err)
	}
	return &Server{URL: u, container: ctr}, nil
}

// FreshDatabase creates an empty database on the server at adminDSN and
// returns a DSN for it, giving each test an isolated schema.
func FreshDatabase(t *testing.T, adminDSN string) string {
	t.Helper()

	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("random db name: %v", err)
	}
	name := "t_" + hex.EncodeToString(b[:])

	conn, err := pgx.Connect(t.Context(), adminDSN)
	if err != nil {
		t.Fatalf("connect admin: %v", err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	if _, err := conn.Exec(t.Context(), "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("create database: %v", err)
	}

	u, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatalf("parse admin dsn: %v", err)
	}
	u.Path = "/" + name
	return u.String()
}
