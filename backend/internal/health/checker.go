// Package health answers "is this server OK?" for two audiences:
// infrastructure (Docker, load balancers, Kubernetes) via /livez and /readyz,
// and humans via /api/status. See knowledge-base/notes/phase-0-notes.md, note 2.
package health

import (
	"context"
	"math"
	"time"
)

const (
	StatusOK   = "ok"
	StatusDown = "down"
)

// Pinger is the one thing the checker needs from the database.
// *pgxpool.Pool satisfies it, and tests use a fake.
type Pinger interface {
	Ping(ctx context.Context) error
}

// DependencyStatus is the result of checking one dependency.
type DependencyStatus struct {
	Status    string  `json:"status"`
	LatencyMS float64 `json:"latency_ms"`
}

// Checker runs dependency checks, each bounded by a timeout so a hung
// dependency can't make the health check itself hang.
type Checker struct {
	db      Pinger
	timeout time.Duration
}

func NewChecker(db Pinger, timeout time.Duration) *Checker {
	return &Checker{db: db, timeout: timeout}
}

// CheckPostgres pings the database and reports whether it answered in time.
func (c *Checker) CheckPostgres(ctx context.Context) DependencyStatus {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	start := time.Now()
	err := c.db.Ping(ctx)
	latency := roundMS(time.Since(start))

	if err != nil {
		return DependencyStatus{Status: StatusDown, LatencyMS: latency}
	}
	return DependencyStatus{Status: StatusOK, LatencyMS: latency}
}

// roundMS converts a duration to milliseconds with two decimals (e.g. 0.42).
func roundMS(d time.Duration) float64 {
	return math.Round(float64(d.Microseconds())/10) / 100
}
