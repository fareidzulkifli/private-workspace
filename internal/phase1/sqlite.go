package phase1

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

func OpenSQLite(ctx context.Context, path string) (*sql.DB, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("sqlite path is required")
	}

	if path != ":memory:" {
		dir := filepath.Dir(path)
		if dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0o750); err != nil {
				return nil, fmt.Errorf("create sqlite directory: %w", err)
			}
		}
	}

	db, err := sql.Open(sqliteDriver, path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	if err := configureSQLite(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func configureSQLite(ctx context.Context, db *sql.DB) error {
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
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("set %s: %w", stmt, err)
		}
	}
	return nil
}

func FindMigrationsDir(start string) (string, error) {
	if start == "" {
		start = "."
	}
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(abs, "migrations")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, nil
		}
		next := filepath.Dir(abs)
		if next == abs {
			return "", errors.New("migrations directory not found")
		}
		abs = next
	}
}

func ApplyMigrations(ctx context.Context, db *sql.DB, migrationsDir string) ([]string, error) {
	if migrationsDir == "" {
		found, err := FindMigrationsDir(".")
		if err != nil {
			return nil, err
		}
		migrationsDir = found
	}

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

	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return nil, fmt.Errorf("ensure schema_migrations: %w", err)
	}

	var applied []string
	for _, file := range files {
		version := strings.TrimSuffix(file, ".sql")
		var count int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version = ?", version).Scan(&count); err != nil {
			return applied, fmt.Errorf("check migration %s: %w", version, err)
		}
		if count > 0 {
			continue
		}

		sqlBytes, err := os.ReadFile(filepath.Join(migrationsDir, file))
		if err != nil {
			return applied, fmt.Errorf("read migration %s: %w", file, err)
		}

		tx, err := db.BeginTx(ctx, nil)
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

func QuoteSQLiteString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
