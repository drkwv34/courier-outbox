package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/drkwv34/courier-outbox/internal/store"
)

func migrate() error {
	cfg, logger, err := loadConfig()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	applied, err := store.Migrate(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	if len(applied) == 0 {
		logger.Info("schema up to date")
		return nil
	}
	logger.Info("migrations applied", slog.Any("versions", applied))
	return nil
}
