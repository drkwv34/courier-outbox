package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/drkwv34/courier-outbox/internal/domain"
	"github.com/drkwv34/courier-outbox/internal/store"
	"github.com/drkwv34/courier-outbox/internal/worker"
)

func runWorker() error {
	cfg, logger, err := loadConfig()
	if err != nil {
		return err
	}
	envelope, err := domain.NewEnvelope(cfg.EncryptionKey)
	if err != nil {
		return fmt.Errorf("encryption envelope: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pg, err := store.OpenPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pg.Close()

	id := cfg.WorkerID
	if id == "" {
		id = defaultWorkerID()
	}
	backoff := domain.DefaultBackoff
	if cfg.BackoffOverride != nil {
		backoff = domain.OverrideBackoff(*cfg.BackoffOverride)
	}

	pool, err := worker.NewPool(worker.Config{
		Store:       pg,
		Envelope:    envelope,
		Logger:      logger,
		Client:      worker.NewClient(),
		WorkerID:    id,
		Concurrency: cfg.WorkerConcurrency,
		ClaimLimit:  cfg.WorkerClaimLimit,
		Lease:       cfg.WorkerLeaseTTL,
		Poll:        cfg.WorkerPollInterval,
		Backoff:     backoff,
	})
	if err != nil {
		return err
	}

	logger.Info("worker starting",
		slog.String("worker_id", id),
		slog.Int("concurrency", cfg.WorkerConcurrency),
		slog.Int("claim_limit", cfg.WorkerClaimLimit),
		slog.Duration("lease", cfg.WorkerLeaseTTL),
	)
	if err := pool.Run(ctx); err != nil {
		return fmt.Errorf("worker: %w", err)
	}
	logger.Info("worker stopped")
	return nil
}

func defaultWorkerID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "courier"
	}
	return fmt.Sprintf("%s-%d", host, os.Getpid())
}
