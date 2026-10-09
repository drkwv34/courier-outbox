package config

import (
	"encoding/base64"
	"errors"
	"log/slog"
	"maps"
	"strings"
	"testing"
	"time"
)

const testPepper = "0123456789abcdef0123456789abcdef"

var testEncKey = base64.StdEncoding.EncodeToString([]byte("local-dev-only-enc-key-32bytes!!"))

func envFrom(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func baseEnv() map[string]string {
	return map[string]string{
		"DATABASE_URL":           "postgres://courier:pw@localhost:5432/courier?sslmode=disable",
		"REDIS_URL":              "redis://localhost:6379/0",
		"COURIER_API_KEY_PEPPER": testPepper,
		"COURIER_ENCRYPTION_KEY": testEncKey,
	}
}

func with(overrides map[string]string) map[string]string {
	env := baseEnv()
	maps.Copy(env, overrides)
	return env
}

func TestLoad(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		env       map[string]string
		wantAddr  string
		wantLevel slog.Level
		wantSSRF  bool
		wantHTTP  bool
		wantErrIn string
	}{
		{name: "defaults", env: baseEnv(), wantAddr: ":8080", wantLevel: slog.LevelInfo, wantSSRF: true, wantHTTP: false},
		{name: "explicit", env: with(map[string]string{"HTTP_ADDR": "127.0.0.1:9000", "LOG_LEVEL": "debug"}), wantAddr: "127.0.0.1:9000", wantLevel: slog.LevelDebug, wantSSRF: true, wantHTTP: false},
		{name: "postgresql scheme", env: with(map[string]string{"DATABASE_URL": "postgresql://db/courier"}), wantAddr: ":8080", wantLevel: slog.LevelInfo, wantSSRF: true},
		{name: "rediss scheme", env: with(map[string]string{"REDIS_URL": "rediss://cache:6380"}), wantAddr: ":8080", wantLevel: slog.LevelInfo, wantSSRF: true},
		{name: "ssrf off http on", env: with(map[string]string{"SSRF_PROTECTION": "false", "ALLOW_HTTP_CALLBACKS": "true"}), wantAddr: ":8080", wantLevel: slog.LevelInfo, wantSSRF: false, wantHTTP: true},
		{name: "bools case-insensitive", env: with(map[string]string{"SSRF_PROTECTION": "FALSE", "ALLOW_HTTP_CALLBACKS": "TRUE"}), wantAddr: ":8080", wantLevel: slog.LevelInfo, wantSSRF: false, wantHTTP: true},
		{name: "bad addr", env: with(map[string]string{"HTTP_ADDR": "8080"}), wantErrIn: "HTTP_ADDR"},
		{name: "bad level", env: with(map[string]string{"LOG_LEVEL": "loud"}), wantErrIn: "LOG_LEVEL"},
		{name: "missing database url", env: with(map[string]string{"DATABASE_URL": ""}), wantErrIn: "DATABASE_URL is required"},
		{name: "wrong database scheme", env: with(map[string]string{"DATABASE_URL": "mysql://db/courier"}), wantErrIn: "DATABASE_URL scheme"},
		{name: "database url without host", env: with(map[string]string{"DATABASE_URL": "postgres:///courier"}), wantErrIn: "DATABASE_URL has no host"},
		{name: "missing redis url", env: with(map[string]string{"REDIS_URL": ""}), wantErrIn: "REDIS_URL is required"},
		{name: "wrong redis scheme", env: with(map[string]string{"REDIS_URL": "http://cache"}), wantErrIn: "REDIS_URL scheme"},
		{name: "missing pepper", env: with(map[string]string{"COURIER_API_KEY_PEPPER": ""}), wantErrIn: "COURIER_API_KEY_PEPPER"},
		{name: "short pepper", env: with(map[string]string{"COURIER_API_KEY_PEPPER": testPepper[:31]}), wantErrIn: "COURIER_API_KEY_PEPPER"},
		{name: "missing encryption key", env: with(map[string]string{"COURIER_ENCRYPTION_KEY": ""}), wantErrIn: "COURIER_ENCRYPTION_KEY is required"},
		{name: "short encryption key", env: with(map[string]string{"COURIER_ENCRYPTION_KEY": base64.StdEncoding.EncodeToString([]byte("short"))}), wantErrIn: "COURIER_ENCRYPTION_KEY"},
		{name: "invalid encryption key encoding", env: with(map[string]string{"COURIER_ENCRYPTION_KEY": "%%%not-base64%%%"}), wantErrIn: "COURIER_ENCRYPTION_KEY"},
		{name: "invalid ssrf bool", env: with(map[string]string{"SSRF_PROTECTION": "yes"}), wantErrIn: "SSRF_PROTECTION"},
		{name: "invalid http bool", env: with(map[string]string{"ALLOW_HTTP_CALLBACKS": "1"}), wantErrIn: "ALLOW_HTTP_CALLBACKS"},
		{name: "bad worker concurrency", env: with(map[string]string{"WORKER_CONCURRENCY": "0"}), wantErrIn: "WORKER_CONCURRENCY"},
		{name: "bad claim limit", env: with(map[string]string{"WORKER_CLAIM_LIMIT": "-1"}), wantErrIn: "WORKER_CLAIM_LIMIT"},
		{name: "bad lease ttl", env: with(map[string]string{"WORKER_LEASE_TTL": "forever"}), wantErrIn: "WORKER_LEASE_TTL"},
		{name: "bad poll interval", env: with(map[string]string{"WORKER_POLL_INTERVAL": "0s"}), wantErrIn: "WORKER_POLL_INTERVAL"},
		{name: "bad backoff ms", env: with(map[string]string{"COURIER_BACKOFF_MS": "0"}), wantErrIn: "COURIER_BACKOFF_MS"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := Load(envFrom(tt.env))
			if tt.wantErrIn != "" {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("Load() error = %v, want ErrInvalid", err)
				}
				if !strings.Contains(err.Error(), tt.wantErrIn) {
					t.Fatalf("Load() error = %q, want it to mention %q", err, tt.wantErrIn)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() unexpected error: %v", err)
			}
			if cfg.HTTPAddr != tt.wantAddr || cfg.LogLevel != tt.wantLevel {
				t.Fatalf("Load() = addr %q level %v, want addr %q level %v", cfg.HTTPAddr, cfg.LogLevel, tt.wantAddr, tt.wantLevel)
			}
			if cfg.SSRFProtection != tt.wantSSRF || cfg.AllowHTTPCallbacks != tt.wantHTTP {
				t.Fatalf("Load() SSRF=%v HTTP=%v, want SSRF=%v HTTP=%v", cfg.SSRFProtection, cfg.AllowHTTPCallbacks, tt.wantSSRF, tt.wantHTTP)
			}
			if string(cfg.APIKeyPepper) != tt.env["COURIER_API_KEY_PEPPER"] {
				t.Fatal("Load() did not carry the pepper through")
			}
			if len(cfg.EncryptionKey) != EncryptionKeyBytes {
				t.Fatalf("EncryptionKey length = %d, want %d", len(cfg.EncryptionKey), EncryptionKeyBytes)
			}
		})
	}
}

func TestLoad_WorkerDefaultsAndOverride(t *testing.T) {
	t.Parallel()

	cfg, err := Load(envFrom(baseEnv()))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WorkerConcurrency != 4 || cfg.WorkerClaimLimit != 8 || cfg.WorkerLeaseTTL != 30*time.Second || cfg.WorkerPollInterval != 500*time.Millisecond {
		t.Fatalf("defaults conc=%d limit=%d lease=%s poll=%s", cfg.WorkerConcurrency, cfg.WorkerClaimLimit, cfg.WorkerLeaseTTL, cfg.WorkerPollInterval)
	}
	if cfg.BackoffOverride != nil {
		t.Fatalf("BackoffOverride = %v, want nil", cfg.BackoffOverride)
	}

	cfg, err = Load(envFrom(with(map[string]string{
		"WORKER_ID":            "w-1",
		"WORKER_CONCURRENCY":   "2",
		"WORKER_CLAIM_LIMIT":   "3",
		"WORKER_LEASE_TTL":     "15s",
		"WORKER_POLL_INTERVAL": "100ms",
		"COURIER_BACKOFF_MS":   "10",
	})))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WorkerID != "w-1" || cfg.WorkerConcurrency != 2 || cfg.WorkerClaimLimit != 3 || cfg.WorkerLeaseTTL != 15*time.Second || cfg.WorkerPollInterval != 100*time.Millisecond {
		t.Fatalf("explicit = %+v", cfg)
	}
	if cfg.BackoffOverride == nil || *cfg.BackoffOverride != 10*time.Millisecond {
		t.Fatalf("BackoffOverride = %v", cfg.BackoffOverride)
	}
}

func TestLoad_RawBase64EncryptionKey(t *testing.T) {
	t.Parallel()

	raw := base64.RawStdEncoding.EncodeToString([]byte("local-dev-only-enc-key-32bytes!!"))
	cfg, err := Load(envFrom(with(map[string]string{"COURIER_ENCRYPTION_KEY": raw})))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(cfg.EncryptionKey); got != "local-dev-only-enc-key-32bytes!!" {
		t.Fatalf("decoded key = %q", got)
	}
}

func TestLoad_ReportsAllProblemsTogether(t *testing.T) {
	t.Parallel()

	_, err := Load(envFrom(map[string]string{"LOG_LEVEL": "loud"}))
	for _, want := range []string{"LOG_LEVEL", "DATABASE_URL", "REDIS_URL", "COURIER_API_KEY_PEPPER", "COURIER_ENCRYPTION_KEY"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("Load() error = %v, want it to mention %s", err, want)
		}
	}
}

func TestLoad_ErrorsDoNotLeakCredentials(t *testing.T) {
	t.Parallel()

	_, err := Load(envFrom(with(map[string]string{"DATABASE_URL": "mysql://user:hunter2@db/x"})))
	if err == nil || strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("Load() error = %v, must not echo credentials", err)
	}

	secret := base64.StdEncoding.EncodeToString([]byte("super-secret-encryption-key!!32"))
	_, err = Load(envFrom(with(map[string]string{"COURIER_ENCRYPTION_KEY": "not-valid-%%-" + secret})))
	if err == nil || strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "super-secret") {
		t.Fatalf("Load() error = %v, must not echo encryption key", err)
	}
}
