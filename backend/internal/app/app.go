// Package app wires the modules into one http.Handler. main.go and the
// end-to-end API tests both build the server through New, so tests exercise
// exactly the routes and middleware that production runs.
package app

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"relay/internal/auth"
	"relay/internal/config"
	"relay/internal/health"
	"relay/internal/httpx"
	"relay/internal/users"
)

func New(cfg config.Config, pool *pgxpool.Pool, version string) http.Handler {
	userStore := users.NewStore(pool)
	tokens := auth.NewTokens(cfg.JWTSecret, cfg.AccessTTL)

	mux := http.NewServeMux()

	// Public: health and logging in.
	health.NewHandler(health.NewChecker(pool, time.Second), version, time.Now()).Register(mux)
	auth.NewHandler(userStore, auth.NewSessions(pool, cfg.RefreshTTL), tokens,
		auth.NewHasher(cfg.BcryptCost), cfg.CookieSecure).Register(mux)

	// Everything else under /api needs an access token. Go's mux picks the most
	// specific pattern, so /api/auth/… and /api/status above still win.
	private := http.NewServeMux()
	users.NewHandler(userStore).Register(private)
	mux.Handle("/api/", tokens.RequireUser(private))

	return httpx.Recover(httpx.Log(mux))
}
