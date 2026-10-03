package database

import (
	"context"
	"errors"
	"net"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// IsUnavailable reports whether err means "the database can't be reached
// right now" — worth a 503 and a retry later — rather than a bug in our query.
//
//   - can't open a connection, or a network/timeout error on the way
//   - Postgres class 08 "connection exception"
//   - 57P01 / 57P02 / 57P03: the server is shutting down, crashed, or not
//     accepting connections yet. Stopping Postgres kills pooled idle
//     connections with 57P01, so the next query on one of them fails this way.
func IsUnavailable(err error) bool {
	if err == nil {
		return false
	}
	var connErr *pgconn.ConnectError
	var netErr net.Error
	if errors.As(err, &connErr) || errors.As(err, &netErr) ||
		errors.Is(err, context.DeadlineExceeded) || pgconn.Timeout(err) {
		return true
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "57P01", "57P02", "57P03":
			return true
		}
		return strings.HasPrefix(pgErr.Code, "08")
	}
	return false
}
