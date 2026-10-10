package db_test

import (
	"context"
	"net/url"
	"testing"

	"relay/internal/db"
	"relay/internal/testdb"
)

func TestMigrate_SecondRunIsNoOp(t *testing.T) {
	pool := testdb.New(t) // already migrated once

	u, _ := url.Parse(pool.Config().ConnString())
	if err := db.Migrate(u.String()); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	var n int
	err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM information_schema.tables WHERE table_name IN ('users','sessions')").Scan(&n)
	if err != nil || n != 2 {
		t.Fatalf("tables found = %d, err = %v; want 2", n, err)
	}
}
