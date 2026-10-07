// Package config loads and validates process configuration from the
// environment. Configuration is read once at startup; invalid values fail
// fast instead of surfacing later at request time.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strings"
)

// ErrInvalid is wrapped by every validation failure returned from Load.
var ErrInvalid = errors.New("invalid config")

const (
	// MinPepperBytes is the minimum length of COURIER_API_KEY_PEPPER.
	MinPepperBytes = 32
	// EncryptionKeyBytes is the AES-256 key length of COURIER_ENCRYPTION_KEY.
	EncryptionKeyBytes = 32
)

// Config is the validated runtime configuration.
type Config struct {
	HTTPAddr    string
	LogLevel    slog.Level
	DatabaseURL string
	RedisURL    string
	// APIKeyPepper keys the HMAC used to hash API keys at rest (ADR 0004).
	APIKeyPepper []byte
	// EncryptionKey is the AES-256-GCM key for signing secrets at rest.
	EncryptionKey []byte
	// SSRFProtection rejects private/loopback/link-local IP literals on
	// subscription target URLs. Default true.
	SSRFProtection bool
	// AllowHTTPCallbacks permits http:// target URLs. Default false.
	AllowHTTPCallbacks bool
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

	key, err := parseEncryptionKey(getenv("COURIER_ENCRYPTION_KEY"))
	if err != nil {
		errs = append(errs, err)
	}
	cfg.EncryptionKey = key

	ssrf, err := parseBool("SSRF_PROTECTION", getenv("SSRF_PROTECTION"), true)
	if err != nil {
		errs = append(errs, err)
	}
	cfg.SSRFProtection = ssrf

	allowHTTP, err := parseBool("ALLOW_HTTP_CALLBACKS", getenv("ALLOW_HTTP_CALLBACKS"), false)
	if err != nil {
		errs = append(errs, err)
	}
	cfg.AllowHTTPCallbacks = allowHTTP

	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}
	return cfg, nil
}

// parseEncryptionKey decodes standard or raw-standard base64 into exactly 32
// bytes. Error messages never echo the value.
func parseEncryptionKey(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("%w: COURIER_ENCRYPTION_KEY is required", ErrInvalid)
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		key, err = base64.RawStdEncoding.DecodeString(raw)
	}
	if err != nil || len(key) != EncryptionKeyBytes {
		return nil, fmt.Errorf("%w: COURIER_ENCRYPTION_KEY must be %d bytes (base64)", ErrInvalid, EncryptionKeyBytes)
	}
	return key, nil
}

func parseBool(name, raw string, defaultVal bool) (bool, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return defaultVal, nil
	}
	switch strings.ToLower(s) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("%w: %s must be true or false", ErrInvalid, name)
	}
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
