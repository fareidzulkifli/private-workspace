package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadFromLookupDefaults(t *testing.T) {
	cfg, err := LoadFromLookup(mapLookup(map[string]string{
		"SQLITE_PATH":         "./data/test.db",
		"APP_SECRET":          "test-secret",
		"ADMIN_EMAIL":         " Admin@Example.COM ",
		"ADMIN_PASSWORD_HASH": "argon2id$hash",
	}))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.AppEnv != EnvDevelopment {
		t.Fatalf("AppEnv = %q", cfg.AppEnv)
	}
	if cfg.AppBaseURL != "http://localhost:4000" {
		t.Fatalf("AppBaseURL = %q", cfg.AppBaseURL)
	}
	if cfg.HTTPAddr != "127.0.0.1:4000" {
		t.Fatalf("HTTPAddr = %q", cfg.HTTPAddr)
	}
	if cfg.MigrationsDir != "./migrations" {
		t.Fatalf("MigrationsDir = %q", cfg.MigrationsDir)
	}
	if cfg.SessionCookie != "pw_session" {
		t.Fatalf("SessionCookie = %q", cfg.SessionCookie)
	}
	if cfg.CSRFHeader != "X-CSRF-Token" {
		t.Fatalf("CSRFHeader = %q", cfg.CSRFHeader)
	}
	if cfg.SessionTTL != 168*time.Hour {
		t.Fatalf("SessionTTL = %s", cfg.SessionTTL)
	}
	if cfg.CookieSecure {
		t.Fatal("CookieSecure should default false in development")
	}
	if cfg.AdminEmail != "admin@example.com" {
		t.Fatalf("AdminEmail = %q", cfg.AdminEmail)
	}
}

func TestLoadFromLookupProductionRequiresSecureCookie(t *testing.T) {
	_, err := LoadFromLookup(mapLookup(map[string]string{
		"APP_ENV":             "production",
		"SQLITE_PATH":         "./data/test.db",
		"APP_SECRET":          "test-secret",
		"ADMIN_EMAIL":         "admin@example.com",
		"ADMIN_PASSWORD_HASH": "argon2id$hash",
		"COOKIE_SECURE":       "false",
	}))
	if err == nil {
		t.Fatal("expected production COOKIE_SECURE error")
	}
	if !strings.Contains(err.Error(), "COOKIE_SECURE") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadFromLookupRequiresSecretsWithoutLeakingValues(t *testing.T) {
	_, err := LoadFromLookup(mapLookup(map[string]string{
		"SQLITE_PATH":   "./data/test.db",
		"APP_SECRET":    "super-secret-value",
		"ADMIN_EMAIL":   "admin@example.com",
		"COOKIE_SECURE": "false",
	}))
	if err == nil {
		t.Fatal("expected missing ADMIN_PASSWORD_HASH error")
	}
	if strings.Contains(err.Error(), "super-secret-value") {
		t.Fatalf("error leaked secret value: %v", err)
	}
}

func TestLoadFromLookupRejectsInvalidTTL(t *testing.T) {
	_, err := LoadFromLookup(mapLookup(map[string]string{
		"SQLITE_PATH":         "./data/test.db",
		"APP_SECRET":          "test-secret",
		"ADMIN_EMAIL":         "admin@example.com",
		"ADMIN_PASSWORD_HASH": "argon2id$hash",
		"SESSION_TTL_HOURS":   "0",
	}))
	if err == nil {
		t.Fatal("expected ttl error")
	}
}

func mapLookup(values map[string]string) lookupFunc {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}
