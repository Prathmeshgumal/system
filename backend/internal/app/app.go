// Package app wires the modules into one http.Handler. main.go and the
// end-to-end API tests both build the server through New, so tests exercise
// exactly the routes and middleware that production runs.
package app

import (
	"net/http"
	"runtime"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"relay/internal/auth"
	"relay/internal/chat"
	"relay/internal/config"
	"relay/internal/health"
	"relay/internal/httpx"
	"relay/internal/realtime"
	"relay/internal/users"
)

type App struct {
	Handler http.Handler
	Hub     *realtime.Hub // main calls Hub.Shutdown to close WebSockets on exit
}

func New(cfg config.Config, pool *pgxpool.Pool, version string) *App {
	userStore := users.NewStore(pool)
	tokens := auth.NewTokens(cfg.JWTSecret, cfg.AccessTTL)
	hub := realtime.NewHub()
	ws := realtime.NewHandler(hub, tokens)
	chats := chat.NewService(chat.NewStore(pool), userStore, hub)

	mux := http.NewServeMux()

	// Public: health, logging in, and the WebSocket (it checks its own ticket).
	health.NewHandler(health.NewChecker(pool, time.Second), version, time.Now()).Register(mux)
	auth.NewHandler(userStore, auth.NewSessions(pool, cfg.RefreshTTL), tokens,
		auth.NewHasher(cfg.BcryptCost, hashSlots()), cfg.CookieSecure).Register(mux)
	ws.RegisterPublic(mux)

	// Everything else under /api needs an access token. Go's mux picks the most
	// specific pattern, so /api/auth/… and /api/status above still win.
	private := http.NewServeMux()
	users.NewHandler(userStore).Register(private)
	chat.NewHandler(chats).Register(private)
	ws.RegisterPrivate(private)
	mux.Handle("/api/", tokens.RequireUser(private))

	return &App{Handler: httpx.Recover(httpx.Log(mux)), Hub: hub}
}

// hashSlots: how many bcrypt hashes may run at once. One core stays free for
// everything else, so a burst of logins can't stall chat (Part 1.4).
// GOMAXPROCS, not NumCPU: it respects container CPU limits.
func hashSlots() int {
	return max(1, runtime.GOMAXPROCS(0)-1)
}
