package backup

import (
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"private-workspace/internal/db"
)

func TestRunCreatesRestorableGzip(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "private-workspace.db")

	store, err := db.Open(ctx, db.Config{
		Path:          dbPath,
		MigrationsDir: filepath.Join("..", "..", "migrations"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	report, err := Run(ctx, Options{
		SQLitePath: dbPath,
		BackupDir:  filepath.Join(tmp, "backups"),
		Tier:       "hourly",
		Now:        time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.GzipPath == "" {
		t.Fatal("expected gzip path in report")
	}
	if _, err := os.Stat(report.GzipPath); err != nil {
		t.Fatal(err)
	}

	restoredPath := filepath.Join(tmp, "restored.db")
	if err := gunzipFile(report.GzipPath, restoredPath); err != nil {
		t.Fatal(err)
	}
	restoredDB, err := openSQLite(ctx, restoredPath)
	if err != nil {
		t.Fatal(err)
	}
	defer restoredDB.Close()
	integrity, err := integrityCheck(ctx, restoredDB)
	if err != nil {
		t.Fatal(err)
	}
	if integrity != "ok" {
		t.Fatalf("expected restored integrity ok, got %q", integrity)
	}
}

func gunzipFile(sourcePath, targetPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	reader, err := gzip.NewReader(source)
	if err != nil {
		return err
	}
	defer reader.Close()

	target, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer target.Close()

	_, err = io.Copy(target, reader)
	return err
}
