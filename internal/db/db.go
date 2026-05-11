package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const sqliteDriver = "sqlite"

type Config struct {
	Path          string
	MigrationsDir string
	AppEnv        string
}

type DB struct {
	sql           *sql.DB
	migrationsDir string
}

func Open(ctx context.Context, cfg Config) (*DB, error) {
	if strings.TrimSpace(cfg.Path) == "" {
		return nil, errors.New("sqlite path is required")
	}
	if strings.TrimSpace(cfg.MigrationsDir) == "" {
		return nil, errors.New("migrations directory is required")
	}

	if cfg.Path != ":memory:" {
		dir := filepath.Dir(cfg.Path)
		if dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0o750); err != nil {
				return nil, fmt.Errorf("create sqlite directory: %w", err)
			}
		}
	}

	sqlDB, err := sql.Open(sqliteDriver, cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)

	store := &DB{sql: sqlDB, migrationsDir: cfg.MigrationsDir}
	if err := store.configure(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	if _, err := store.applyMigrations(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return store, nil
}

func (d *DB) Close() error {
	if d == nil || d.sql == nil {
		return nil
	}
	return d.sql.Close()
}

func (d *DB) Ping(ctx context.Context) error {
	return d.sql.PingContext(ctx)
}

func (d *DB) Ready(ctx context.Context) error {
	if err := d.Ping(ctx); err != nil {
		return fmt.Errorf("sqlite ping: %w", err)
	}

	var tableName string
	if err := d.sql.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'").Scan(&tableName); err != nil {
		return fmt.Errorf("schema_migrations table is missing: %w", err)
	}

	latest, err := latestMigration(d.migrationsDir)
	if err != nil {
		return err
	}
	var count int
	if err := d.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version = ?", latest).Scan(&count); err != nil {
		return fmt.Errorf("check latest migration: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("latest migration %s is not applied", latest)
	}

	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin readiness write probe: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "CREATE TEMP TABLE IF NOT EXISTS __pw_ready_probe (id INTEGER)"); err != nil {
		return fmt.Errorf("create readiness probe table: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO __pw_ready_probe (id) VALUES (1)"); err != nil {
		return fmt.Errorf("run readiness write probe: %w", err)
	}
	return nil
}

func (d *DB) Tx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (d *DB) SQL() *sql.DB {
	return d.sql
}

func (d *DB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return d.sql.ExecContext(ctx, query, args...)
}

func (d *DB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return d.sql.QueryContext(ctx, query, args...)
}

func (d *DB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return d.sql.QueryRowContext(ctx, query, args...)
}

func (d *DB) configure(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA wal_autocheckpoint = 1000",
	}
	for _, stmt := range pragmas {
		if _, err := d.sql.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("set %s: %w", stmt, err)
		}
	}
	return nil
}

func (d *DB) applyMigrations(ctx context.Context) ([]string, error) {
	files, err := migrationFiles(d.migrationsDir)
	if err != nil {
		return nil, err
	}

	if _, err := d.sql.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return nil, fmt.Errorf("ensure schema_migrations: %w", err)
	}

	var applied []string
	for _, file := range files {
		version := strings.TrimSuffix(file, ".sql")
		var count int
		if err := d.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version = ?", version).Scan(&count); err != nil {
			return applied, fmt.Errorf("check migration %s: %w", version, err)
		}
		if count > 0 {
			continue
		}

		sqlBytes, err := os.ReadFile(filepath.Join(d.migrationsDir, file))
		if err != nil {
			return applied, fmt.Errorf("read migration %s: %w", file, err)
		}

		tx, err := d.sql.BeginTx(ctx, nil)
		if err != nil {
			return applied, fmt.Errorf("begin migration %s: %w", version, err)
		}
		if _, err := tx.ExecContext(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback()
			return applied, fmt.Errorf("apply migration %s: %w", version, err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)", version, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			_ = tx.Rollback()
			return applied, fmt.Errorf("record migration %s: %w", version, err)
		}
		if err := tx.Commit(); err != nil {
			return applied, fmt.Errorf("commit migration %s: %w", version, err)
		}
		applied = append(applied, version)
	}
	return applied, nil
}

func latestMigration(migrationsDir string) (string, error) {
	files, err := migrationFiles(migrationsDir)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(files[len(files)-1], ".sql"), nil
}

func migrationFiles(migrationsDir string) ([]string, error) {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("read migrations directory: %w", err)
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("no .sql migrations found in %s", migrationsDir)
	}
	return files, nil
}
