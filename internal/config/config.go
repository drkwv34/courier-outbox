// Package config loads and validates process configuration from the
// environment. Configuration is read once at startup; invalid values fail
// fast instead of surfacing later at request time.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
)

// ErrInvalid is wrapped by every validation failure returned from Load.
var ErrInvalid = errors.New("invalid config")

// Config is the validated runtime configuration.
//
// Fields for DATABASE_URL, REDIS_URL, COURIER_ENCRYPTION_KEY, SSRF_PROTECTION
// and ALLOW_HTTP_CALLBACKS are added by the feature changes that consume them.
type Config struct {
	HTTPAddr string
	LogLevel slog.Level
}

// Load reads configuration using getenv (usually os.Getenv) and validates it.
// All problems are reported together so operators can fix them in one pass.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		HTTPAddr: valueOr(getenv("HTTP_ADDR"), ":8080"),
	}

	var errs []error

	if _, _, err := net.SplitHostPort(cfg.HTTPAddr); err != nil {
		errs = append(errs, fmt.Errorf("%w: HTTP_ADDR %q: %v", ErrInvalid, cfg.HTTPAddr, err))
	}

	level, err := parseLevel(valueOr(getenv("LOG_LEVEL"), "info"))
	if err != nil {
		errs = append(errs, err)
	}
	cfg.LogLevel = level

	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}
	return cfg, nil
}

func parseLevel(s string) (slog.Level, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(strings.ToUpper(s))); err != nil {
		return 0, fmt.Errorf("%w: LOG_LEVEL %q: want debug|info|warn|error", ErrInvalid, s)
	}
	return level, nil
}

func valueOr(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
