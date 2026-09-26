// Package config loads and validates process configuration from the
// environment. Configuration is read once at startup; invalid values fail
// fast instead of surfacing later at request time.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strings"
)

// ErrInvalid is wrapped by every validation failure returned from Load.
var ErrInvalid = errors.New("invalid config")

// MinPepperBytes is the minimum length of COURIER_API_KEY_PEPPER.
const MinPepperBytes = 32

// Config is the validated runtime configuration.
//
// Fields for COURIER_ENCRYPTION_KEY, SSRF_PROTECTION and ALLOW_HTTP_CALLBACKS
// are added by the feature changes that consume them.
type Config struct {
	HTTPAddr    string
	LogLevel    slog.Level
	DatabaseURL string
	RedisURL    string
	// APIKeyPepper keys the HMAC used to hash API keys at rest (ADR 0004).
	APIKeyPepper []byte
}

// Load reads configuration using getenv (usually os.Getenv) and validates it.
// All problems are reported together so operators can fix them in one pass.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		HTTPAddr:     valueOr(getenv("HTTP_ADDR"), ":8080"),
		DatabaseURL:  strings.TrimSpace(getenv("DATABASE_URL")),
		RedisURL:     strings.TrimSpace(getenv("REDIS_URL")),
		APIKeyPepper: []byte(getenv("COURIER_API_KEY_PEPPER")),
	}

	var errs []error

	if _, _, err := net.SplitHostPort(cfg.HTTPAddr); err != nil {
		errs = append(errs, fmt.Errorf("%w: HTTP_ADDR %q: %w", ErrInvalid, cfg.HTTPAddr, err))
	}

	level, err := parseLevel(valueOr(getenv("LOG_LEVEL"), "info"))
	if err != nil {
		errs = append(errs, err)
	}
	cfg.LogLevel = level

	if err := validateURL("DATABASE_URL", cfg.DatabaseURL, "postgres", "postgresql"); err != nil {
		errs = append(errs, err)
	}
	if err := validateURL("REDIS_URL", cfg.RedisURL, "redis", "rediss"); err != nil {
		errs = append(errs, err)
	}
	if len(cfg.APIKeyPepper) < MinPepperBytes {
		errs = append(errs, fmt.Errorf("%w: COURIER_API_KEY_PEPPER must be at least %d bytes", ErrInvalid, MinPepperBytes))
	}

	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}
	return cfg, nil
}

// validateURL checks that raw is a URL with one of the allowed schemes and a
// host. Error messages never echo the value because it may carry credentials.
func validateURL(name, raw string, schemes ...string) error {
	if raw == "" {
		return fmt.Errorf("%w: %s is required", ErrInvalid, name)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: %s is not a valid URL", ErrInvalid, name)
	}
	for _, s := range schemes {
		if u.Scheme == s {
			if u.Host == "" {
				return fmt.Errorf("%w: %s has no host", ErrInvalid, name)
			}
			return nil
		}
	}
	return fmt.Errorf("%w: %s scheme must be one of %s", ErrInvalid, name, strings.Join(schemes, ", "))
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
