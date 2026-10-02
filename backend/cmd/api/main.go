// Command api is the Relay backend.
//
//	api              start the HTTP server
//	api healthcheck  exit 0 if the local server's /livez answers 200, else 1
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"relay/internal/health"
)

// version is replaced at build time: go build -ldflags "-X main.version=<git sha>".
var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}
	if err := run(); err != nil {
		slog.Error("api stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	port := getenv("PORT", "8080")
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return errors.New("DATABASE_URL is not set (copy .env.example to .env)")
	}

	// ctx is cancelled on Ctrl+C (SIGINT) or `docker stop` (SIGTERM).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// pgxpool connects lazily: this succeeds even if Postgres is down.
	// /readyz reports 503 until the database becomes reachable.
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	defer pool.Close()

	mux := http.NewServeMux()
	health.NewHandler(health.NewChecker(pool, time.Second), version, time.Now()).Register(mux)

	// Go's zero-value http.Server has no timeouts, so a slow client could hold
	// a connection open forever (slowloris). Set them explicitly.
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() {
		slog.Info("api listening", "addr", srv.Addr, "version", version)
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		return err // e.g. port already in use
	case <-ctx.Done():
	}

	// Graceful shutdown: stop accepting new connections, give in-flight
	// requests up to 10s to finish, then return (deferred pool.Close runs next).
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	slog.Info("stopped cleanly")
	return nil
}

func healthcheck() int {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	url := "http://localhost:" + getenv("PORT", "8080") + "/livez"
	if err := health.Probe(ctx, url); err != nil {
		fmt.Fprintln(os.Stderr, "unhealthy:", err)
		return 1
	}
	return 0
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
