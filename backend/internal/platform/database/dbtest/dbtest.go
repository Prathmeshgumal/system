// Package dbtest gives integration tests a real, throwaway Postgres
// (Testcontainers). See knowledge-base/notes/phase-0-notes.md, note 10.
//
// Usage in a test package:
//
//	func TestMain(m *testing.M) { dbtest.Main(m) }
//
//	func TestSomething(t *testing.T) {
//	    pool := dbtest.Pool(t) // skipped under `go test -short`
//	    ...
//	}
package dbtest

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"go.uber.org/zap"

	"relay/internal/platform/database"
)

// Same major version as compose.yaml, so tests see what production sees.
const image = "postgres:18"

var (
	once      sync.Once
	container *tcpostgres.PostgresContainer
	pool      *pgxpool.Pool
	startErr  error
)

// Main runs the package's tests, then removes the container if one was started.
func Main(m *testing.M) {
	code := m.Run()
	if pool != nil {
		pool.Close()
	}
	if container != nil {
		_ = testcontainers.TerminateContainer(container)
	}
	os.Exit(code)
}

// Pool returns a pool connected to a migrated test database. The container is
// started on first use and shared by every test in the package; each test must
// clean up the rows it creates (see Truncate). Under -short it skips the test,
// so `make test-unit` runs without Docker.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: needs Docker (skipped with -short)")
	}
	once.Do(func() { startErr = start() })
	if startErr != nil {
		t.Fatalf("start test Postgres: %v", startErr)
	}
	return pool
}

// Truncate empties tables and registers the same for when the test ends,
// so each test starts and finishes with clean tables.
func Truncate(t *testing.T, p *pgxpool.Pool, tables ...string) {
	t.Helper()
	run := func() {
		for _, tbl := range tables {
			if _, err := p.Exec(context.Background(), "TRUNCATE "+tbl+" RESTART IDENTITY CASCADE"); err != nil {
				t.Fatalf("truncate %s: %v", tbl, err)
			}
		}
	}
	run()
	t.Cleanup(run)
}

func start() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	c, err := tcpostgres.Run(ctx, image,
		tcpostgres.WithDatabase("relay_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		return fmt.Errorf("run container: %w", err)
	}
	container = c

	url, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return fmt.Errorf("connection string: %w", err)
	}
	p, err := pgxpool.New(ctx, url)
	if err != nil {
		return fmt.Errorf("pool: %w", err)
	}
	pool = p

	if err := database.Migrate(ctx, p, zap.NewNop()); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}
