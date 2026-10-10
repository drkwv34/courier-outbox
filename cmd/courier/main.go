// Command courier is the single courier-outbox binary.
//
// Subcommands (SRS §4.3): serve, migrate, keys create, worker.
package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/drkwv34/courier-outbox/internal/api"
	"github.com/drkwv34/courier-outbox/internal/config"
	"github.com/drkwv34/courier-outbox/internal/domain"
	"github.com/drkwv34/courier-outbox/internal/store"
)

const shutdownTimeout = 10 * time.Second

const usage = `usage: courier <command>

commands:
  serve                     run the HTTP API (default)
  migrate                   apply pending database migrations
  keys create --name <name> create an API key and print it once
  worker                    claim, sign, and deliver due webhooks`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "courier:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := "serve"
	if len(args) > 0 {
		cmd = args[0]
	}

	switch cmd {
	case "serve":
		return serve()
	case "migrate":
		return migrate()
	case "keys":
		return keys(args[1:])
	case "worker":
		return runWorker()
	case "-h", "--help", "help":
		fmt.Println(usage)
		return nil
	default:
		return fmt.Errorf("unknown command %q\n%s", cmd, usage)
	}
}

// loadConfig reads and validates configuration and installs the JSON logger.
func loadConfig() (config.Config, *slog.Logger, error) {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return config.Config{}, nil, fmt.Errorf("load config: %w", err)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)
	return cfg, logger, nil
}

func serve() error {
	cfg, logger, err := loadConfig()
	if err != nil {
		return err
	}
	hasher, err := domain.NewAPIKeyHasher(cfg.APIKeyPepper, config.MinPepperBytes)
	if err != nil {
		return fmt.Errorf("api key hasher: %w", err)
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

	rdb, err := store.OpenRedis(cfg.RedisURL)
	if err != nil {
		return err
	}
	defer func() { _ = rdb.Close() }()

	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: api.NewRouter(api.Deps{
			Logger: logger,
			Keys:   pg,
			Hasher: hasher,
			Readiness: []api.ReadinessCheck{
				{Name: "postgres", Check: pg.Ping},
				{Name: "redis", Check: rdb.Ping},
			},
			Subscriptions: pg,
			Events:        pg,
			Deliveries:    pg,
			URLPolicy: domain.URLPolicy{
				AllowHTTP:   cfg.AllowHTTPCallbacks,
				ProtectSSRF: cfg.SSRFProtection,
			},
			Envelope: envelope,
			Rand:     rand.Reader,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server listening", slog.String("addr", cfg.HTTPAddr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	case <-ctx.Done():
	}

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("http shutdown: %w", err)
	}
	return nil
}
