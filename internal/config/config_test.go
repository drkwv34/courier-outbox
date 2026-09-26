package config

import (
	"errors"
	"log/slog"
	"testing"
)

func envFrom(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoad(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		env       map[string]string
		wantAddr  string
		wantLevel slog.Level
		wantErr   bool
	}{
		{name: "defaults", env: map[string]string{}, wantAddr: ":8080", wantLevel: slog.LevelInfo},
		{name: "explicit", env: map[string]string{"HTTP_ADDR": "127.0.0.1:9000", "LOG_LEVEL": "debug"}, wantAddr: "127.0.0.1:9000", wantLevel: slog.LevelDebug},
		{name: "bad addr", env: map[string]string{"HTTP_ADDR": "8080"}, wantErr: true},
		{name: "bad level", env: map[string]string{"LOG_LEVEL": "loud"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := Load(envFrom(tt.env))
			if tt.wantErr {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("Load() error = %v, want ErrInvalid", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() unexpected error: %v", err)
			}
			if cfg.HTTPAddr != tt.wantAddr || cfg.LogLevel != tt.wantLevel {
				t.Fatalf("Load() = %+v, want addr %q level %v", cfg, tt.wantAddr, tt.wantLevel)
			}
		})
	}
}
