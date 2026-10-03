package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
	"go.uber.org/zap"
)

// MigrationsDir is where migration files live, relative to backend/.
// `api migrate create` writes new files here.
const MigrationsDir = "internal/platform/database/migrations"

// The .sql files are compiled into the binary, so the production image needs
// no extra files to run `api migrate`.
//
//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migrate applies every migration that hasn't run yet (goose up).
func Migrate(ctx context.Context, pool *pgxpool.Pool, log *zap.Logger) error {
	return withProvider(pool, func(p *goose.Provider) error {
		results, err := p.Up(ctx)
		for _, r := range results {
			log.Info("migration applied", zap.String("file", r.Source.Path), zap.Duration("took", r.Duration))
		}
		if err != nil {
			return fmt.Errorf("migrate up: %w", err)
		}
		if len(results) == 0 {
			log.Info("database already up to date")
		}
		return nil
	})
}

// MigrateDown undoes the most recently applied migration (goose down).
func MigrateDown(ctx context.Context, pool *pgxpool.Pool, log *zap.Logger) error {
	return withProvider(pool, func(p *goose.Provider) error {
		r, err := p.Down(ctx)
		if err != nil {
			return fmt.Errorf("migrate down: %w", err)
		}
		log.Info("migration undone", zap.String("file", r.Source.Path))
		return nil
	})
}

// MigrationStatus logs every migration and whether it has been applied.
func MigrationStatus(ctx context.Context, pool *pgxpool.Pool, log *zap.Logger) error {
	return withProvider(pool, func(p *goose.Provider) error {
		statuses, err := p.Status(ctx)
		if err != nil {
			return fmt.Errorf("migrate status: %w", err)
		}
		for _, s := range statuses {
			log.Info("migration", zap.String("file", s.Source.Path), zap.String("state", string(s.State)))
		}
		return nil
	})
}

// CreateMigration writes a new, numbered, empty .sql migration into dir.
func CreateMigration(dir, name string) error {
	goose.SetSequential(true) // 00002_x.sql rather than a timestamp
	return goose.Create(nil, dir, name, "sql")
}

func withProvider(pool *pgxpool.Pool, fn func(*goose.Provider) error) error {
	// goose speaks database/sql; this wraps our pgx pool without a second pool.
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	p, err := newProvider(db)
	if err != nil {
		return err
	}
	return fn(p)
}

func newProvider(db *sql.DB) (*goose.Provider, error) {
	fsys, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		return nil, err
	}
	// A Postgres advisory lock: if two `api migrate` runs start at once, the
	// second waits instead of applying the same migration twice.
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return nil, err
	}
	p, err := goose.NewProvider(goose.DialectPostgres, db, fsys, goose.WithSessionLocker(locker))
	if err != nil {
		return nil, fmt.Errorf("load migrations: %w", err)
	}
	return p, nil
}
