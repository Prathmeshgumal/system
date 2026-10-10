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
	MetricsAddr  string        // private Prometheus listener, e.g. :9090; empty = off (Part 2.4)
	LogFormat    string        // "text" (dev, readable) or "json" (production, for Loki)

	S3 S3Config // object storage for profile pictures (Part 2.2); empty Endpoint = off
}

// S3Config points at any S3-compatible storage: AWS S3 in production,
// RustFS locally. With empty keys, the AWS SDK credential chain is used —
// on EC2 that's the instance's IAM role, so no keys exist at all.
type S3Config struct {
	Endpoint     string // host[:port], e.g. localhost:9000 or s3.eu-central-1.amazonaws.com
	Region       string
	Bucket       string
	AccessKey    string
	SecretKey    string
	UseSSL       bool
	PublicURL    string // what browsers upload to; e.g. /storage (proxied) — empty = the endpoint itself
	CreateBucket bool   // dev convenience: create the bucket at startup if missing
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
		MetricsAddr: getenv("METRICS_ADDR"),
		LogFormat:   or(getenv("LOG_FORMAT"), "text"),
		S3: S3Config{
			Endpoint:  getenv("S3_ENDPOINT"),
			Region:    or(getenv("S3_REGION"), "us-east-1"),
			Bucket:    or(getenv("S3_BUCKET"), "relay-media"),
			AccessKey: getenv("S3_ACCESS_KEY"),
			SecretKey: getenv("S3_SECRET_KEY"),
			PublicURL: getenv("S3_PUBLIC_URL"),
		},
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

	for name, dst := range map[string]*bool{"S3_USE_SSL": &cfg.S3.UseSSL, "S3_CREATE_BUCKET": &cfg.S3.CreateBucket} {
		v, err := strconv.ParseBool(or(getenv(name), "false"))
		if err != nil {
			errs = append(errs, fmt.Errorf("%s must be true or false", name))
		}
		*dst = v
	}
	if cfg.LogFormat != "text" && cfg.LogFormat != "json" {
		errs = append(errs, errors.New(`LOG_FORMAT must be "text" or "json"`))
	}
	if (cfg.S3.AccessKey == "") != (cfg.S3.SecretKey == "") {
		errs = append(errs, errors.New("set both S3_ACCESS_KEY and S3_SECRET_KEY, or neither (IAM role)"))
	}

	return cfg, errors.Join(errs...)
}

func or(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
