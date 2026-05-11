package db

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"runtime"
	"testing"
)

func TestOpenAppliesMigrationsAndReady(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, Config{
		Path:          filepath.Join(t.TempDir(), "private-workspace.db"),
		MigrationsDir: migrationsDir(t),
		AppEnv:        "development",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.Ready(ctx); err != nil {
		t.Fatalf("Ready: %v", err)
	}

	var count int
	if err := store.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version = ?", "001_initial_sqlite_schema").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("migration count = %d", count)
	}
}

func TestTxRollsBackOnError(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, Config{
		Path:          filepath.Join(t.TempDir(), "private-workspace.db"),
		MigrationsDir: migrationsDir(t),
		AppEnv:        "development",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	rollbackErr := errors.New("rollback")
	err = store.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO organizations (id, name, slug, created_at) VALUES (?, ?, ?, ?)", "org_1", "Org", "org", "2026-01-01T00:00:00Z")
		if err != nil {
			return err
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("expected rollback error, got %v", err)
	}

	var count int
	if err := store.QueryRowContext(ctx, "SELECT COUNT(*) FROM organizations WHERE id = ?", "org_1").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("row should have rolled back, count=%d", count)
	}
}

func migrationsDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "migrations"))
}
