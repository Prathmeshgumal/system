// Package db opens the connection pool and applies migrations.
package db

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // registers the "pgx5://" scheme
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"

	"relay/migrations"
)

// Open creates the pool. pgxpool connects lazily, so this succeeds even if
// Postgres is down — /readyz reports 503 until it's reachable.
func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	return pool, nil
}

// Migrate applies every migration that hasn't run yet. Running it again is a no-op:
// golang-migrate records the current version in the schema_migrations table.
func Migrate(url string) error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return err
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, migrateURL(url))
	if err != nil {
		return fmt.Errorf("connect for migrations: %w", err)
	}
	defer m.Close()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

// MigrateWithRetry keeps trying for up to `wait`. On `make up-full` the api can
// start before Postgres accepts connections, so the first attempts may fail.
func MigrateWithRetry(ctx context.Context, url string, wait time.Duration) error {
	deadline := time.Now().Add(wait)
	for {
		err := Migrate(url)
		if err == nil || time.Now().After(deadline) {
			return err
		}
		slog.Warn("database not ready for migrations, retrying", "err", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// migrateURL swaps postgres:// for pgx5://, the scheme golang-migrate's pgx driver uses.
func migrateURL(url string) string {
	for _, prefix := range []string{"postgres://", "postgresql://"} {
		if rest, ok := strings.CutPrefix(url, prefix); ok {
			return "pgx5://" + rest
		}
	}
	return url
}
