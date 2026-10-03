package notes

import (
	"context"
	"strings"
	"testing"

	"relay/internal/platform/database/dbtest"
)

func TestMain(m *testing.M) { dbtest.Main(m) }

// Integration tests against a real Postgres (note 10).

func TestStore_CreateThenListNewestFirst(t *testing.T) {
	pool := dbtest.Pool(t)
	dbtest.Truncate(t, pool, "notes")
	store := NewStore(pool)
	ctx := context.Background()

	for _, b := range []string{"first", "second", "third"} {
		if _, err := store.Create(ctx, b); err != nil {
			t.Fatalf("Create(%q): %v", b, err)
		}
	}

	got, err := store.ListLatest(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Body != "third" || got[2].Body != "first" {
		t.Fatalf("want newest first [third second first], got %+v", got)
	}
	if got[0].ID == 0 || got[0].CreatedAt.IsZero() {
		t.Fatalf("database should fill id and created_at: %+v", got[0])
	}
}

func TestStore_ListRespectsLimit(t *testing.T) {
	pool := dbtest.Pool(t)
	dbtest.Truncate(t, pool, "notes")
	store := NewStore(pool)
	ctx := context.Background()
	for range 5 {
		if _, err := store.Create(ctx, "x"); err != nil {
			t.Fatal(err)
		}
	}

	got, err := store.ListLatest(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
}

func TestStore_EmptyTable_ReturnsEmptyNotNil(t *testing.T) {
	pool := dbtest.Pool(t)
	dbtest.Truncate(t, pool, "notes")

	got, err := NewStore(pool).ListLatest(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("want an empty slice, got nil")
	}
}

// The database is the last line of defence: even if a bug skipped the
// service's validation, the CHECK constraint refuses the row.
func TestStore_DatabaseRejectsTooLongBody(t *testing.T) {
	pool := dbtest.Pool(t)
	dbtest.Truncate(t, pool, "notes")

	_, err := NewStore(pool).Create(context.Background(), strings.Repeat("a", 1001))
	if err == nil {
		t.Fatal("Create() with 1001 chars succeeded; the CHECK constraint should reject it")
	}
}
