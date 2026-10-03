package database_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"relay/internal/platform/database"
	"relay/internal/platform/database/dbtest"
)

func TestMain(m *testing.M) { dbtest.Main(m) }

// Integration tests: a real Postgres in a throwaway container (note 10).

func TestMigrate_CreatesTheNotesTable(t *testing.T) {
	pool := dbtest.Pool(t) // migrations already applied by the helper

	var exists bool
	err := pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'notes')`).Scan(&exists)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("table notes does not exist after migrating")
	}
}

func TestMigrate_RunningTwiceIsANoOp(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()

	before := currentVersion(t, pool)
	if err := database.Migrate(ctx, pool, zap.NewNop()); err != nil {
		t.Fatalf("second Migrate() error = %v", err)
	}
	if after := currentVersion(t, pool); after != before {
		t.Fatalf("version changed from %d to %d on a no-op run", before, after)
	}
}

func TestStats_ReflectsConnectionsInUse(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()

	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	inUse := database.Stats(pool).InUse
	conn.Release()

	if inUse < 1 {
		t.Fatalf("InUse = %d while holding a connection, want >= 1", inUse)
	}
}

// currentVersion reads goose's own bookkeeping table (note 9).
func currentVersion(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	var v int64
	err := pool.QueryRow(context.Background(),
		`SELECT max(version_id) FROM goose_db_version WHERE is_applied`).Scan(&v)
	if err != nil {
		t.Fatalf("read goose_db_version: %v", err)
	}
	if v < 1 {
		t.Fatalf("version = %d, want at least 1 after migrating", v)
	}
	return v
}
