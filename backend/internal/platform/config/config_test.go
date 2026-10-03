package config

import (
	"strings"
	"testing"
	"time"
)

// validEnv is the smallest environment that loads successfully.
func validEnv() map[string]string {
	return map[string]string{"DATABASE_URL": "postgres://u:p@localhost:5432/db"}
}

func TestLoad_OnlyRequired_AppliesDefaults(t *testing.T) {
	cfg, err := load(validEnv())
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}

	checks := []struct {
		name      string
		got, want any
	}{
		{"Port", cfg.Port, 8080},
		{"LogLevel", cfg.LogLevel, "info"},
		{"LogFormat", cfg.LogFormat, "json"},
		{"DBMaxConns", cfg.DBMaxConns, int32(10)},
		{"DBMinConns", cfg.DBMinConns, int32(2)},
		{"DBMaxConnLifetime", cfg.DBMaxConnLifetime, time.Hour},
		{"DBMaxConnIdleTime", cfg.DBMaxConnIdleTime, 30 * time.Minute},
		{"ShutdownDrainDelay", cfg.ShutdownDrainDelay, 5 * time.Second},
		{"ShutdownTimeout", cfg.ShutdownTimeout, 10 * time.Second},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestLoad_OverridesFromEnv(t *testing.T) {
	env := validEnv()
	env["PORT"] = "9000"
	env["SHUTDOWN_DRAIN_DELAY"] = "0s"
	env["LOG_FORMAT"] = "console"

	cfg, err := load(env)
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}
	if cfg.Port != 9000 || cfg.ShutdownDrainDelay != 0 || cfg.LogFormat != "console" {
		t.Fatalf("overrides not applied: %+v", cfg)
	}
}

func TestLoad_Errors(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string // applied on top of validEnv()
		drop    string            // key removed from validEnv()
		wantMsg string            // the error must mention this, so the user knows what to fix
	}{
		{name: "missing DATABASE_URL", drop: "DATABASE_URL", wantMsg: "DATABASE_URL"},
		{name: "bad duration", env: map[string]string{"SHUTDOWN_TIMEOUT": "abc"}, wantMsg: "SHUTDOWN_TIMEOUT"},
		{name: "port not a number", env: map[string]string{"PORT": "http"}, wantMsg: "PORT"},
		{name: "port out of range", env: map[string]string{"PORT": "70000"}, wantMsg: "PORT"},
		{name: "min conns above max", env: map[string]string{"DB_MIN_CONNS": "20"}, wantMsg: "DB_MIN_CONNS"},
		{name: "max conns zero", env: map[string]string{"DB_MAX_CONNS": "0"}, wantMsg: "DB_MAX_CONNS"},
		{name: "unknown log level", env: map[string]string{"LOG_LEVEL": "loud"}, wantMsg: "LOG_LEVEL"},
		{name: "unknown log format", env: map[string]string{"LOG_FORMAT": "xml"}, wantMsg: "LOG_FORMAT"},
		{name: "negative drain delay", env: map[string]string{"SHUTDOWN_DRAIN_DELAY": "-1s"}, wantMsg: "SHUTDOWN_DRAIN_DELAY"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := validEnv()
			delete(env, tt.drop)
			for k, v := range tt.env {
				env[k] = v
			}

			_, err := load(env)

			if err == nil {
				t.Fatal("load() error = nil, want an error")
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Fatalf("error %q should mention %q", err, tt.wantMsg)
			}
		})
	}
}
