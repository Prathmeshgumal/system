// Package config reads the environment once, at startup, into a typed Config.
// Anything missing or invalid stops the server before it serves a single request.
package config

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type Config struct {
	Port         string
	DatabaseURL  string
	JWTSecret    []byte        // signs access tokens and WebSocket tickets
	AccessTTL    time.Duration // how long an access token lives
	RefreshTTL   time.Duration // how long a login lasts without activity
	CookieSecure bool          // send the refresh cookie over HTTPS only (Phase 2+)
	BcryptCost   int           // 12 in production; tests use 4 (bcrypt.MinCost) to stay fast
	PprofAddr    string        // private profiling listener, e.g. 127.0.0.1:6060; empty = off
}

// Load builds a Config from getenv (os.Getenv in main, a map in tests).
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		Port:        or(getenv("PORT"), "8080"),
		DatabaseURL: getenv("DATABASE_URL"),
		JWTSecret:   []byte(getenv("JWT_SECRET")),
		AccessTTL:   15 * time.Minute,
		RefreshTTL:  30 * 24 * time.Hour,
		PprofAddr:   getenv("PPROF_ADDR"),
	}
	var errs []error

	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is not set (copy .env.example to .env)"))
	}
	// HS256 is only as strong as its key: 32 bytes = 256 bits.
	if len(cfg.JWTSecret) < 32 {
		errs = append(errs, errors.New("JWT_SECRET must be at least 32 bytes (openssl rand -base64 32)"))
	}

	cost, err := strconv.Atoi(or(getenv("BCRYPT_COST"), "12"))
	if err != nil || cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		errs = append(errs, fmt.Errorf("BCRYPT_COST must be %d–%d", bcrypt.MinCost, bcrypt.MaxCost))
	}
	cfg.BcryptCost = cost

	secure, err := strconv.ParseBool(or(getenv("COOKIE_SECURE"), "false"))
	if err != nil {
		errs = append(errs, errors.New("COOKIE_SECURE must be true or false"))
	}
	cfg.CookieSecure = secure

	return cfg, errors.Join(errs...)
}

func or(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
