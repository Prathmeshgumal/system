// Package database owns the Postgres connection pool and the schema migrations.
// See knowledge-base/notes/phase-0-notes.md, notes 9 and 11.
package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"relay/internal/platform/config"
)

// Open creates the connection pool with the settings from config. Like
// pgxpool.New it connects lazily, so it succeeds even if Postgres is down —
// /readyz reports that until the database is reachable.
func Open(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	pc, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	pc.MaxConns = cfg.DBMaxConns
	pc.MinConns = cfg.DBMinConns
	pc.MaxConnLifetime = cfg.DBMaxConnLifetime
	pc.MaxConnIdleTime = cfg.DBMaxConnIdleTime

	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	return pool, nil
}

// PoolStats is a snapshot of the pool, shown on the status page.
type PoolStats struct {
	InUse  int32 `json:"in_use"` // borrowed by a request right now
	Idle   int32 `json:"idle"`   // open and waiting to be borrowed
	Total  int32 `json:"total"`  // in use + idle (+ being opened)
	Max    int32 `json:"max"`    // DB_MAX_CONNS
	Waited int64 `json:"waited"` // borrows that had to wait because every connection was busy (since start)
}

func Stats(pool *pgxpool.Pool) PoolStats {
	s := pool.Stat()
	return PoolStats{
		InUse:  s.AcquiredConns(),
		Idle:   s.IdleConns(),
		Total:  s.TotalConns(),
		Max:    s.MaxConns(),
		Waited: s.EmptyAcquireCount(),
	}
}
