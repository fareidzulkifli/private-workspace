package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"private-workspace/internal/phase1"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	sourceURL, err := phase1.RequiredEnv("SUPABASE_DB_URL")
	if err != nil {
		fatal(err)
	}
	sqlitePath := phase1.Env("SQLITE_PATH", "")
	dryRun, err := phase1.EnvBool("MIGRATION_DRY_RUN", false)
	if err != nil {
		fatal(err)
	}

	report, err := phase1.RunMigration(ctx, phase1.MigrationOptions{
		SourceDBURL:   sourceURL,
		SQLitePath:    sqlitePath,
		MigrationsDir: phase1.Env("MIGRATIONS_DIR", ""),
		DryRun:        dryRun,
	})
	if report != nil {
		markdown := report.Markdown()
		reportPath := phase1.Env("MIGRATION_REPORT_PATH", defaultReportPath("migration-report", report.CompletedAt))
		if err := os.MkdirAll(filepath.Dir(reportPath), 0o750); err != nil {
			fatal(err)
		}
		if writeErr := os.WriteFile(reportPath, []byte(markdown), 0o640); writeErr != nil {
			fatal(writeErr)
		}
		fmt.Print(markdown)
		fmt.Printf("\nwrote migration report to %s\n", reportPath)
	}
	if err != nil {
		fatal(err)
	}
}

func defaultReportPath(prefix string, completedAt time.Time) string {
	if completedAt.IsZero() {
		completedAt = time.Now().UTC()
	}
	return filepath.Join(".docs", prefix+"-"+completedAt.UTC().Format("20060102T150405Z")+".md")
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
