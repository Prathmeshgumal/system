// Package config loads every setting the backend needs from environment
// variables, once, at startup — and refuses to start if anything is missing or
// invalid. See knowledge-base/notes/phase-0-notes.md, note 5.
package config

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

// Config holds every setting. Defaults are the production-safe values;
// development overrides them in .env.
type Config struct {
	Port        int    `env:"PORT"         envDefault:"8080"`
	DatabaseURL string `env:"DATABASE_URL,required"`

	LogLevel  string `env:"LOG_LEVEL"  envDefault:"info"` // debug | info | warn | error
	LogFormat string `env:"LOG_FORMAT" envDefault:"json"` // json | console

	DBMaxConns        int32         `env:"DB_MAX_CONNS"          envDefault:"10"`
	DBMinConns        int32         `env:"DB_MIN_CONNS"          envDefault:"2"`
	DBMaxConnLifetime time.Duration `env:"DB_MAX_CONN_LIFETIME"  envDefault:"1h"`
	DBMaxConnIdleTime time.Duration `env:"DB_MAX_CONN_IDLE_TIME" envDefault:"30m"`

	ShutdownDrainDelay time.Duration `env:"SHUTDOWN_DRAIN_DELAY" envDefault:"5s"`
	ShutdownTimeout    time.Duration `env:"SHUTDOWN_TIMEOUT"     envDefault:"10s"`
}

// Load reads the process environment.
func Load() (Config, error) {
	return load(environ())
}

// load reads from the given map — tests pass their own instead of touching
// the real environment.
func load(vars map[string]string) (Config, error) {
	var cfg Config
	if err := env.ParseWithOptions(&cfg, env.Options{Environment: vars}); err != nil {
		return Config{}, fmt.Errorf("config: %w", friendly(err))
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	return cfg, nil
}

// Validate checks rules that struct tags can't express. It reports every
// problem at once, so you can fix them all in one go.
func (c Config) Validate() error {
	var errs []error
	if c.Port < 1 || c.Port > 65535 {
		errs = append(errs, fmt.Errorf("PORT must be 1-65535, got %d", c.Port))
	}
	if c.DBMaxConns < 1 {
		errs = append(errs, fmt.Errorf("DB_MAX_CONNS must be at least 1, got %d", c.DBMaxConns))
	}
	if c.DBMinConns < 0 || c.DBMinConns > c.DBMaxConns {
		errs = append(errs, fmt.Errorf("DB_MIN_CONNS must be between 0 and DB_MAX_CONNS (%d), got %d", c.DBMaxConns, c.DBMinConns))
	}
	if !oneOf(c.LogLevel, "debug", "info", "warn", "error") {
		errs = append(errs, fmt.Errorf("LOG_LEVEL must be debug, info, warn or error, got %q", c.LogLevel))
	}
	if !oneOf(c.LogFormat, "json", "console") {
		errs = append(errs, fmt.Errorf("LOG_FORMAT must be json or console, got %q", c.LogFormat))
	}
	if c.ShutdownDrainDelay < 0 {
		errs = append(errs, fmt.Errorf("SHUTDOWN_DRAIN_DELAY must not be negative, got %s", c.ShutdownDrainDelay))
	}
	if c.ShutdownTimeout <= 0 {
		errs = append(errs, fmt.Errorf("SHUTDOWN_TIMEOUT must be positive, got %s", c.ShutdownTimeout))
	}
	return errors.Join(errs...)
}

// friendly rewrites the library's parse errors, which name the Go field
// ("ShutdownTimeout"), to name the env var you actually edit ("SHUTDOWN_TIMEOUT").
func friendly(err error) error {
	var agg env.AggregateError
	if !errors.As(err, &agg) {
		return err
	}
	out := make([]error, 0, len(agg.Errors))
	for _, e := range agg.Errors {
		var pe env.ParseError
		if errors.As(e, &pe) {
			out = append(out, fmt.Errorf("%s has an invalid value: %w", envName(pe.Name), pe.Err))
			continue
		}
		out = append(out, e) // e.g. "required environment variable \"DATABASE_URL\" is not set"
	}
	return errors.Join(out...)
}

// envName returns the env var for a Config field, read from its `env` tag.
func envName(field string) string {
	f, ok := reflect.TypeFor[Config]().FieldByName(field)
	if !ok {
		return field
	}
	name, _, _ := strings.Cut(f.Tag.Get("env"), ",")
	return name
}

func oneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}

func environ() map[string]string {
	vars := make(map[string]string)
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			vars[k] = v
		}
	}
	return vars
}
