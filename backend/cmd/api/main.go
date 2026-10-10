// Command api is the Relay backend.
//
//	api              apply migrations, then start the HTTP server
//	api migrate      apply migrations and exit
//	api healthcheck  exit 0 if the local server's /livez answers 200, else 1
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"relay/internal/app"
	"relay/internal/config"
	"relay/internal/db"
	"relay/internal/health"
)

// version is replaced at build time: go build -ldflags "-X main.version=<git sha>".
var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		if err := migrateOnly(); err != nil {
			slog.Error("migrate failed", "err", err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		slog.Error("api stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}

	// ctx is cancelled on Ctrl+C (SIGINT) or `docker stop` (SIGTERM).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// The schema must be in place before we serve anything. In full mode the api
	// can start before Postgres accepts connections, so keep trying for a while.
	if err := db.MigrateWithRetry(ctx, cfg.DatabaseURL, 30*time.Second); err != nil {
		return err
	}

	// After startup, a database outage no longer stops the api:
	// /readyz reports 503 until the database is reachable again.
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	a := app.New(cfg, pool, version)

	// Go's zero-value http.Server has no timeouts, so a slow client could hold
	// a connection open forever (slowloris). Set them explicitly.
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           a.Handler,
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
	a.Hub.Shutdown() // WebSockets are hijacked: srv.Shutdown doesn't close them
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	slog.Info("stopped cleanly")
	return nil
}

func migrateOnly() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	if err := db.Migrate(cfg.DatabaseURL); err != nil {
		return err
	}
	slog.Info("migrations applied")
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
