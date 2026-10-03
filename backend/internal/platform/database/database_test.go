package database

import (
	"context"
	"testing"
	"time"

	"relay/internal/platform/config"
)

// Unit test: no database needed — the pool connects lazily.
func TestOpen_AppliesPoolSettingsFromConfig(t *testing.T) {
	cfg := config.Config{
		DatabaseURL:       "postgres://u:p@127.0.0.1:1/none?sslmode=disable",
		DBMaxConns:        7,
		DBMinConns:        0, // don't start background connections to a fake address
		DBMaxConnLifetime: 42 * time.Minute,
		DBMaxConnIdleTime: 3 * time.Minute,
	}

	pool, err := Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer pool.Close()

	got := pool.Config()
	if got.MaxConns != 7 || got.MinConns != 0 ||
		got.MaxConnLifetime != 42*time.Minute || got.MaxConnIdleTime != 3*time.Minute {
		t.Fatalf("pool config not applied: max=%d min=%d life=%s idle=%s",
			got.MaxConns, got.MinConns, got.MaxConnLifetime, got.MaxConnIdleTime)
	}
	if s := Stats(pool); s.Max != 7 {
		t.Fatalf("Stats().Max = %d, want 7", s.Max)
	}
}

func TestOpen_BadURL_ReturnsError(t *testing.T) {
	_, err := Open(context.Background(), config.Config{DatabaseURL: "::not a url::", DBMaxConns: 1})
	if err == nil {
		t.Fatal("Open() = nil error, want error for a malformed URL")
	}
}
