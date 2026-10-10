package config

import (
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func valid() map[string]string {
	return map[string]string{
		"DATABASE_URL": "postgres://u:p@localhost:5432/db",
		"JWT_SECRET":   strings.Repeat("x", 32),
	}
}

func TestLoad_Defaults(t *testing.T) {
	cfg, err := Load(env(valid()))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want 8080", cfg.Port)
	}
	if cfg.BcryptCost != 12 {
		t.Errorf("BcryptCost = %d, want 12", cfg.BcryptCost)
	}
	if cfg.AccessTTL != 15*time.Minute || cfg.RefreshTTL != 30*24*time.Hour {
		t.Errorf("TTLs = %v / %v", cfg.AccessTTL, cfg.RefreshTTL)
	}
	if cfg.CookieSecure {
		t.Error("CookieSecure should default to false (http://localhost in dev)")
	}
	if cfg.PprofAddr != "" || cfg.MetricsAddr != "" {
		t.Error("pprof and metrics must be off by default")
	}
	if cfg.LogFormat != "text" || cfg.S3.Endpoint != "" || cfg.S3.Bucket != "relay-media" || cfg.S3.Region != "us-east-1" {
		t.Errorf("defaults: %+v", cfg)
	}
}

func TestLoad_Overrides(t *testing.T) {
	m := valid()
	m["PORT"] = "9000"
	m["BCRYPT_COST"] = "4"
	m["COOKIE_SECURE"] = "true"
	m["PPROF_ADDR"] = "127.0.0.1:6060"
	cfg, err := Load(env(m))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != "9000" || cfg.BcryptCost != 4 || !cfg.CookieSecure || cfg.PprofAddr != "127.0.0.1:6060" {
		t.Errorf("overrides not applied: %+v", cfg)
	}
}

func TestLoad_Errors(t *testing.T) {
	cases := map[string]func(map[string]string){
		"no DATABASE_URL":  func(m map[string]string) { delete(m, "DATABASE_URL") },
		"no JWT_SECRET":    func(m map[string]string) { delete(m, "JWT_SECRET") },
		"short JWT_SECRET": func(m map[string]string) { m["JWT_SECRET"] = "too-short" },
		"bad BCRYPT_COST":  func(m map[string]string) { m["BCRYPT_COST"] = "99" },
		"bad bool":         func(m map[string]string) { m["COOKIE_SECURE"] = "maybe" },
		"bad S3 bool":      func(m map[string]string) { m["S3_USE_SSL"] = "yep" },
		"bad log format":   func(m map[string]string) { m["LOG_FORMAT"] = "xml" },
		"half S3 keys":     func(m map[string]string) { m["S3_ACCESS_KEY"] = "only-this" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			m := valid()
			mutate(m)
			if _, err := Load(env(m)); err == nil {
				t.Error("want error, got nil")
			}
		})
	}
}
