package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"private-workspace/internal/backup"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	sqlitePath, err := backup.RequiredEnv("SQLITE_PATH")
	if err != nil {
		fatal(err)
	}

	report, err := backup.Run(ctx, backup.Options{
		SQLitePath: sqlitePath,
		BackupDir:  backup.Env("BACKUP_DIR", "backups"),
		Tier:       backup.Env("BACKUP_TIER", "hourly"),
		R2:         backup.R2OptionsFromEnv(),
	})
	if report != nil {
		markdown := report.Markdown()
		reportPath := backup.Env("BACKUP_REPORT_PATH", defaultReportPath("backup-report", report.CompletedAt))
		if err := os.MkdirAll(filepath.Dir(reportPath), 0o750); err != nil {
			fatal(err)
		}
		if writeErr := os.WriteFile(reportPath, []byte(markdown), 0o640); writeErr != nil {
			fatal(writeErr)
		}
		fmt.Print(markdown)
		fmt.Printf("\nwrote backup report to %s\n", reportPath)
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
