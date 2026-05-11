package phase1

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
)

type MigrationOptions struct {
	SourceDBURL   string
	SQLitePath    string
	MigrationsDir string
	DryRun        bool
}

func RunMigration(ctx context.Context, opts MigrationOptions) (*MigrationReport, error) {
	if opts.SourceDBURL == "" {
		return nil, errors.New("source database URL is required")
	}
	if opts.SQLitePath == "" && !opts.DryRun {
		return nil, errors.New("sqlite path is required")
	}

	startedAt := time.Now().UTC()
	sqlitePath := opts.SQLitePath
	if opts.DryRun {
		tmpDir, err := os.MkdirTemp("", "private-workspace-migration-*")
		if err != nil {
			return nil, fmt.Errorf("create dry-run directory: %w", err)
		}
		sqlitePath = filepath.Join(tmpDir, "dry-run.db")
	} else if info, err := os.Stat(sqlitePath); err == nil && info.Size() > 0 {
		return nil, fmt.Errorf("target sqlite file already exists and is not empty: %s", sqlitePath)
	}

	db, err := OpenSQLite(ctx, sqlitePath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	applied, err := ApplyMigrations(ctx, db, opts.MigrationsDir)
	if err != nil {
		return nil, err
	}

	source, err := pgx.Connect(ctx, opts.SourceDBURL)
	if err != nil {
		return nil, fmt.Errorf("connect to supabase/postgres: %w", err)
	}
	defer source.Close(ctx)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin sqlite import transaction: %w", err)
	}

	report := &MigrationReport{
		StartedAt:  startedAt,
		Source:     opts.SourceDBURL,
		SQLitePath: sqlitePath,
		DryRun:     opts.DryRun,
		Applied:    applied,
	}

	for _, spec := range TableSpecs() {
		sourceCount, err := CountPostgresRows(ctx, source, spec.Name)
		if err != nil {
			_ = tx.Rollback()
			report.CompletedAt = time.Now().UTC()
			return report, err
		}
		imported, err := ImportTable(ctx, source, tx, spec)
		if err != nil {
			_ = tx.Rollback()
			report.CompletedAt = time.Now().UTC()
			return report, err
		}
		report.Counts = append(report.Counts, TableCount{
			Table:    spec.Name,
			Source:   sourceCount,
			Imported: imported,
		})
	}

	if err := tx.Commit(); err != nil {
		report.CompletedAt = time.Now().UTC()
		return report, fmt.Errorf("commit sqlite import transaction: %w", err)
	}

	for i := range report.Counts {
		targetCount, err := CountSQLiteRows(ctx, db, report.Counts[i].Table)
		if err != nil {
			report.CompletedAt = time.Now().UTC()
			return report, err
		}
		report.Counts[i].Target = targetCount
		report.Counts[i].Passed = report.Counts[i].Source == report.Counts[i].Imported && report.Counts[i].Source == targetCount
	}

	checks, err := ValidateSQLite(ctx, db)
	if err != nil {
		report.CompletedAt = time.Now().UTC()
		return report, err
	}
	report.Checks = checks
	report.CompletedAt = time.Now().UTC()

	if !report.Passed() {
		return report, errors.New("migration validation failed")
	}
	return report, nil
}
