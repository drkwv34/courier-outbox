package main

import (
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/drkwv34/courier-outbox/internal/config"
	"github.com/drkwv34/courier-outbox/internal/domain"
	"github.com/drkwv34/courier-outbox/internal/store"
)

const keysUsage = `usage: courier keys create --name <name>`

// apiKeyCreator persists a new key record.
type apiKeyCreator interface {
	CreateAPIKey(ctx context.Context, name, prefix string, hash []byte) (domain.APIKey, error)
}

func keys(args []string) error {
	if len(args) == 0 || args[0] != "create" {
		return errors.New(keysUsage)
	}

	fs := flag.NewFlagSet("keys create", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	name := fs.String("name", "", "human-readable key name (required)")
	if err := fs.Parse(args[1:]); err != nil {
		return fmt.Errorf("%w\n%s", err, keysUsage)
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments %q\n%s", fs.Args(), keysUsage)
	}
	if err := domain.ValidateAPIKeyName(*name); err != nil {
		return fmt.Errorf("%w\n%s", err, keysUsage)
	}

	cfg, _, err := loadConfig()
	if err != nil {
		return err
	}
	hasher, err := domain.NewAPIKeyHasher(cfg.APIKeyPepper, config.MinPepperBytes)
	if err != nil {
		return fmt.Errorf("api key hasher: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pg, err := store.OpenPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pg.Close()

	return createAPIKey(ctx, os.Stdout, pg, hasher, rand.Reader, *name)
}

// createAPIKey mints a key, stores its prefix and hash, and writes the raw
// key to out. This is the only time the raw key is ever visible.
func createAPIKey(ctx context.Context, out io.Writer, repo apiKeyCreator, hasher domain.APIKeyHasher, entropy io.Reader, name string) error {
	name = strings.TrimSpace(name)
	if err := domain.ValidateAPIKeyName(name); err != nil {
		return err
	}
	gen, err := hasher.Generate(entropy)
	if err != nil {
		return fmt.Errorf("generate api key: %w", err)
	}
	key, err := repo.CreateAPIKey(ctx, name, gen.Prefix, gen.Hash)
	if err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return fmt.Errorf("api key prefix collision, run the command again: %w", err)
		}
		return fmt.Errorf("store api key: %w", err)
	}

	_, err = fmt.Fprintf(out, `API key created. Copy it now: it is not stored and cannot be shown again.

  id:     %s
  name:   %s
  prefix: %s
  key:    %s

Use it as: Authorization: Bearer <key>
`, key.ID, key.Name, key.Prefix, gen.Raw)
	return err
}
