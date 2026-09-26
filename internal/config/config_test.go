package config

import (
	"errors"
	"log/slog"
	"maps"
	"strings"
	"testing"
)

const testPepper = "0123456789abcdef0123456789abcdef"

func envFrom(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func baseEnv() map[string]string {
	return map[string]string{
		"DATABASE_URL":           "postgres://courier:pw@localhost:5432/courier?sslmode=disable",
		"REDIS_URL":              "redis://localhost:6379/0",
		"COURIER_API_KEY_PEPPER": testPepper,
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
		wantErrIn string
	}{
		{name: "defaults", env: baseEnv(), wantAddr: ":8080", wantLevel: slog.LevelInfo},
		{name: "explicit", env: with(map[string]string{"HTTP_ADDR": "127.0.0.1:9000", "LOG_LEVEL": "debug"}), wantAddr: "127.0.0.1:9000", wantLevel: slog.LevelDebug},
		{name: "postgresql scheme", env: with(map[string]string{"DATABASE_URL": "postgresql://db/courier"}), wantAddr: ":8080", wantLevel: slog.LevelInfo},
		{name: "rediss scheme", env: with(map[string]string{"REDIS_URL": "rediss://cache:6380"}), wantAddr: ":8080", wantLevel: slog.LevelInfo},
		{name: "bad addr", env: with(map[string]string{"HTTP_ADDR": "8080"}), wantErrIn: "HTTP_ADDR"},
		{name: "bad level", env: with(map[string]string{"LOG_LEVEL": "loud"}), wantErrIn: "LOG_LEVEL"},
		{name: "missing database url", env: with(map[string]string{"DATABASE_URL": ""}), wantErrIn: "DATABASE_URL is required"},
		{name: "wrong database scheme", env: with(map[string]string{"DATABASE_URL": "mysql://db/courier"}), wantErrIn: "DATABASE_URL scheme"},
		{name: "database url without host", env: with(map[string]string{"DATABASE_URL": "postgres:///courier"}), wantErrIn: "DATABASE_URL has no host"},
		{name: "missing redis url", env: with(map[string]string{"REDIS_URL": ""}), wantErrIn: "REDIS_URL is required"},
		{name: "wrong redis scheme", env: with(map[string]string{"REDIS_URL": "http://cache"}), wantErrIn: "REDIS_URL scheme"},
		{name: "missing pepper", env: with(map[string]string{"COURIER_API_KEY_PEPPER": ""}), wantErrIn: "COURIER_API_KEY_PEPPER"},
		{name: "short pepper", env: with(map[string]string{"COURIER_API_KEY_PEPPER": testPepper[:31]}), wantErrIn: "COURIER_API_KEY_PEPPER"},
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
			if string(cfg.APIKeyPepper) != tt.env["COURIER_API_KEY_PEPPER"] {
				t.Fatal("Load() did not carry the pepper through")
			}
		})
	}
}

func TestLoad_ReportsAllProblemsTogether(t *testing.T) {
	t.Parallel()

	_, err := Load(envFrom(map[string]string{"LOG_LEVEL": "loud"}))
	for _, want := range []string{"LOG_LEVEL", "DATABASE_URL", "REDIS_URL", "COURIER_API_KEY_PEPPER"} {
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
}
