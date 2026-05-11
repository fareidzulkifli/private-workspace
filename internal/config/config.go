package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	EnvDevelopment = "development"
	EnvProduction  = "production"
)

type Config struct {
	AppEnv            string
	AppBaseURL        string
	HTTPAddr          string
	SQLitePath        string
	MigrationsDir     string
	AppSecret         string
	SessionCookie     string
	CSRFHeader        string
	SessionTTL        time.Duration
	CookieSecure      bool
	AdminEmail        string
	AdminPasswordHash string
	R2AccessKeyID     string
	R2SecretAccessKey string
	R2Endpoint        string
	R2BucketName      string
	R2Region          string
	GitHubPAT         string
	GitHubBranch      string
	GitNoteOwner      string
	GitNoteRepo       string
	GrokAPIKey        string
	MalaysiaState     string
}

type lookupFunc func(string) (string, bool)

func Load() (Config, error) {
	return LoadFromLookup(os.LookupEnv)
}

func LoadFromLookup(lookup lookupFunc) (Config, error) {
	cfg := Config{
		AppEnv:        EnvDevelopment,
		AppBaseURL:    "http://localhost:4000",
		HTTPAddr:      "127.0.0.1:4000",
		MigrationsDir: "./migrations",
		SessionCookie: "pw_session",
		CSRFHeader:    "X-CSRF-Token",
		SessionTTL:    168 * time.Hour,
		CookieSecure:  false,
		R2Region:      "auto",
		GitHubBranch:  "main",
		GitNoteOwner:  "fareidzulkifli",
		GitNoteRepo:   "BA-notes",
		MalaysiaState: "kuala-lumpur",
	}

	if value, ok := env(lookup, "APP_ENV"); ok {
		cfg.AppEnv = strings.ToLower(value)
	}
	if cfg.AppEnv != EnvDevelopment && cfg.AppEnv != EnvProduction {
		return Config{}, errors.New("APP_ENV must be development or production")
	}

	if value, ok := env(lookup, "APP_BASE_URL"); ok {
		cfg.AppBaseURL = value
	}
	parsedBaseURL, err := url.Parse(cfg.AppBaseURL)
	if err != nil {
		return Config{}, fmt.Errorf("APP_BASE_URL must be a valid URL: %w", err)
	}
	if parsedBaseURL.Scheme == "" || parsedBaseURL.Host == "" {
		return Config{}, errors.New("APP_BASE_URL must be an absolute URL")
	}

	if value, ok := env(lookup, "HTTP_ADDR"); ok {
		cfg.HTTPAddr = value
	}
	if cfg.HTTPAddr == "" {
		return Config{}, errors.New("HTTP_ADDR is required")
	}

	if value, ok := env(lookup, "SQLITE_PATH"); ok {
		cfg.SQLitePath = value
	}
	if cfg.SQLitePath == "" {
		return Config{}, errors.New("SQLITE_PATH is required")
	}

	if value, ok := env(lookup, "MIGRATIONS_DIR"); ok {
		cfg.MigrationsDir = value
	}
	if cfg.MigrationsDir == "" {
		return Config{}, errors.New("MIGRATIONS_DIR is required")
	}

	if value, ok := env(lookup, "APP_SECRET"); ok {
		cfg.AppSecret = value
	}
	if cfg.AppSecret == "" {
		return Config{}, errors.New("APP_SECRET is required")
	}

	if value, ok := env(lookup, "SESSION_COOKIE_NAME"); ok {
		cfg.SessionCookie = value
	}
	if cfg.SessionCookie == "" {
		return Config{}, errors.New("SESSION_COOKIE_NAME is required")
	}

	if value, ok := env(lookup, "CSRF_HEADER_NAME"); ok {
		cfg.CSRFHeader = value
	}
	if cfg.CSRFHeader == "" {
		return Config{}, errors.New("CSRF_HEADER_NAME is required")
	}

	if value, ok := env(lookup, "SESSION_TTL_HOURS"); ok {
		hours, err := strconv.Atoi(value)
		if err != nil || hours <= 0 {
			return Config{}, errors.New("SESSION_TTL_HOURS must be a positive integer")
		}
		cfg.SessionTTL = time.Duration(hours) * time.Hour
	}

	if value, ok := env(lookup, "COOKIE_SECURE"); ok {
		secure, err := strconv.ParseBool(value)
		if err != nil {
			return Config{}, errors.New("COOKIE_SECURE must be true or false")
		}
		cfg.CookieSecure = secure
	}
	if cfg.AppEnv == EnvProduction && !cfg.CookieSecure {
		return Config{}, errors.New("COOKIE_SECURE must be true when APP_ENV=production")
	}

	if value, ok := env(lookup, "ADMIN_EMAIL"); ok {
		cfg.AdminEmail = NormalizeEmail(value)
	}
	if cfg.AdminEmail == "" {
		return Config{}, errors.New("ADMIN_EMAIL is required")
	}
	if !strings.Contains(cfg.AdminEmail, "@") {
		return Config{}, errors.New("ADMIN_EMAIL must be a valid email address")
	}

	if value, ok := env(lookup, "ADMIN_PASSWORD_HASH"); ok {
		cfg.AdminPasswordHash = value
	}
	if cfg.AdminPasswordHash == "" {
		return Config{}, errors.New("ADMIN_PASSWORD_HASH is required")
	}

	if value, ok := env(lookup, "R2_ACCESS_KEY_ID"); ok {
		cfg.R2AccessKeyID = value
	}
	if value, ok := env(lookup, "R2_SECRET_ACCESS_KEY"); ok {
		cfg.R2SecretAccessKey = value
	}
	if value, ok := env(lookup, "R2_ENDPOINT"); ok {
		cfg.R2Endpoint = value
	}
	if value, ok := env(lookup, "R2_BUCKET_NAME"); ok {
		cfg.R2BucketName = value
	}
	if value, ok := env(lookup, "R2_REGION"); ok {
		cfg.R2Region = value
	}
	if value, ok := env(lookup, "GITHUB_PAT"); ok {
		cfg.GitHubPAT = value
	}
	if value, ok := env(lookup, "GITHUB_BRANCH"); ok {
		cfg.GitHubBranch = value
	}
	if value, ok := env(lookup, "GITNOTE_OWNER"); ok {
		cfg.GitNoteOwner = value
	}
	if value, ok := env(lookup, "GITNOTE_REPO"); ok {
		cfg.GitNoteRepo = value
	}
	if value, ok := env(lookup, "GROK_API_KEY"); ok {
		cfg.GrokAPIKey = value
	}
	if value, ok := env(lookup, "MALAYSIA_HOLIDAY_STATE"); ok {
		cfg.MalaysiaState = value
	}

	return cfg, nil
}

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func env(lookup lookupFunc, key string) (string, bool) {
	value, ok := lookup(key)
	if !ok {
		return "", false
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	return value, true
}
