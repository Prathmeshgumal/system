package health

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakePinger stands in for the Postgres pool so tests don't need a real database.
type fakePinger struct {
	err   error
	block bool // if true, Ping waits until the context is cancelled (simulates a hung DB)
}

func (f fakePinger) Ping(ctx context.Context) error {
	if f.block {
		<-ctx.Done()
		return ctx.Err()
	}
	return f.err
}

func TestCheckPostgres_PingSucceeds_ReportsOK(t *testing.T) {
	c := NewChecker(fakePinger{}, time.Second)

	got := c.CheckPostgres(context.Background())

	if got.Status != StatusOK {
		t.Fatalf("status = %q, want %q", got.Status, StatusOK)
	}
	if got.LatencyMS < 0 {
		t.Fatalf("latency_ms = %v, want >= 0", got.LatencyMS)
	}
}

func TestCheckPostgres_PingFails_ReportsDown(t *testing.T) {
	c := NewChecker(fakePinger{err: errors.New("connection refused")}, time.Second)

	got := c.CheckPostgres(context.Background())

	if got.Status != StatusDown {
		t.Fatalf("status = %q, want %q", got.Status, StatusDown)
	}
}

func TestCheckPostgres_PingHangs_GivesUpAtTimeout(t *testing.T) {
	timeout := 50 * time.Millisecond
	c := NewChecker(fakePinger{block: true}, timeout)

	start := time.Now()
	got := c.CheckPostgres(context.Background())
	elapsed := time.Since(start)

	if got.Status != StatusDown {
		t.Fatalf("status = %q, want %q", got.Status, StatusDown)
	}
	if elapsed > 10*timeout {
		t.Fatalf("check took %v, want about %v — the timeout isn't being applied", elapsed, timeout)
	}
}
