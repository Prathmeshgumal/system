// Command api is the Relay backend.
//
//	api                      start the HTTP server (same as `api serve`)
//	api migrate [up]         apply new migrations, then exit
//	api migrate down         undo the last migration
//	api migrate status       list migrations and whether they've run
//	api migrate create NAME  write a new empty migration file (dev only)
//	api healthcheck          exit 0 if the local server's /livez answers 200, else 1
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"go.uber.org/zap"

	"relay/internal/health"
	"relay/internal/notes"
	"relay/internal/platform/config"
	"relay/internal/platform/database"
	"relay/internal/platform/httpserver"
	"relay/internal/platform/logging"
)

// version is replaced at build time: go build -ldflags "-X main.version=<git sha>".
var version = "dev"

const usage = `usage: api [serve | migrate [up|down|status|create NAME] | healthcheck]`

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	cmd := "serve"
	if len(args) > 0 {
		cmd = args[0]
	}
	var err error
	switch cmd {
	case "serve":
		err = serve()
	case "migrate":
		err = migrate(args[1:])
	case "healthcheck":
		err = healthcheck()
	default:
		fmt.Fprintln(os.Stderr, usage)
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

func serve() error {
	cfg, log, err := setup()
	if err != nil {
		return err
	}
	defer func() { _ = log.Sync() }() // step 5 of shutdown: flush buffered log lines

	// ctx is cancelled by the first Ctrl+C (SIGINT) or `docker stop` (SIGTERM).
	// After that we restore default signal handling, so a second Ctrl+C exits
	// immediately instead of waiting for the graceful sequence.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() { <-ctx.Done(); stop() }()

	pool, err := database.Open(context.Background(), cfg)
	if err != nil {
		return err
	}
	defer func() { pool.Close(); log.Info("database pool closed") }() // step 4

	var srv *httpserver.Server // assigned below; readyz asks it whether we're shutting down
	mux := httpserver.NewMux()
	health.NewHandler(health.NewChecker(pool, time.Second), health.Options{
		Version:      version,
		StartedAt:    time.Now(),
		PoolStats:    func() database.PoolStats { return database.Stats(pool) },
		ShuttingDown: func() bool { return srv.ShuttingDown() },
	}).Register(mux)
	notes.NewHandler(notes.NewService(notes.NewStore(pool))).Register(mux)

	// Outermost first: recover → request ID → request log → router (note 7).
	handler := httpserver.Chain(mux,
		httpserver.Recover(log),
		httpserver.RequestID(log),
		httpserver.RequestLog(),
	)
	srv = httpserver.New(":"+strconv.Itoa(cfg.Port), handler, log, cfg.ShutdownDrainDelay, cfg.ShutdownTimeout)

	log.Info("starting", zap.String("version", version), zap.String("log_level", cfg.LogLevel))
	if err := srv.Run(ctx); err != nil { // steps 1–3 happen inside
		return err
	}
	log.Info("stopped cleanly")
	return nil
}

func migrate(args []string) error {
	sub := "up"
	if len(args) > 0 {
		sub = args[0]
	}
	if sub == "create" {
		if len(args) < 2 || args[1] == "" {
			return errors.New("usage: api migrate create NAME")
		}
		return database.CreateMigration(database.MigrationsDir, args[1])
	}

	cfg, log, err := setup()
	if err != nil {
		return err
	}
	defer func() { _ = log.Sync() }()

	pool, err := database.Open(context.Background(), cfg)
	if err != nil {
		return err
	}
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	switch sub {
	case "up":
		return database.Migrate(ctx, pool, log)
	case "down":
		return database.MigrateDown(ctx, pool, log)
	case "status":
		return database.MigrationStatus(ctx, pool, log)
	default:
		return fmt.Errorf("unknown migrate command %q\n%s", sub, usage)
	}
}

// setup loads config and builds the logger — the first two startup steps.
// Errors here are printed plainly: there's no logger yet.
func setup() (config.Config, *zap.Logger, error) {
	cfg, err := config.Load()
	if err != nil {
		return config.Config{}, nil, err
	}
	log, err := logging.New(cfg.LogLevel, cfg.LogFormat)
	if err != nil {
		return config.Config{}, nil, err
	}
	zap.ReplaceGlobals(log) // fallback for code without a request logger
	return cfg, log, nil
}

// healthcheck needs only PORT, so it works even if other config is broken.
func healthcheck() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	return health.Probe(ctx, "http://localhost:"+port+"/livez")
}
