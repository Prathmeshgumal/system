package notes

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// Unit test (no database): which errors count as "database unavailable" → 503,
// and which are bugs → 500.
func TestWrap_ClassifiesUnavailableErrors(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		unavailable bool
	}{
		// Found in dev: stopping Postgres kills pooled connections with 57P01.
		{"admin shutdown 57P01", &pgconn.PgError{Code: "57P01"}, true},
		{"crash shutdown 57P02", &pgconn.PgError{Code: "57P02"}, true},
		{"cannot connect now 57P03", &pgconn.PgError{Code: "57P03"}, true},
		{"connection failure 08006", &pgconn.PgError{Code: "08006"}, true},
		{"network error", &net.OpError{Op: "dial", Err: errors.New("connection refused")}, true},
		{"timeout", context.DeadlineExceeded, true},
		{"wrapped admin shutdown", fmt.Errorf("query: %w", &pgconn.PgError{Code: "57P01"}), true},

		{"check constraint 23514", &pgconn.PgError{Code: "23514"}, false},
		{"syntax error 42601", &pgconn.PgError{Code: "42601"}, false},
		{"plain error", errors.New("boom"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := errors.Is(wrap("op", tt.err), ErrUnavailable)
			if got != tt.unavailable {
				t.Fatalf("unavailable = %v, want %v", got, tt.unavailable)
			}
		})
	}
}
