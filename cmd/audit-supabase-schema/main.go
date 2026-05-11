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
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	sourceURL, err := phase1.RequiredEnv("SUPABASE_DB_URL")
	if err != nil {
		fatal(err)
	}
	outputPath := phase1.Env("AUDIT_OUTPUT_PATH", ".docs/live-supabase-schema-audit.md")
	checkedSchemaPath := phase1.Env("CHECKED_SCHEMA_PATH", "db/schema.sql")

	report, err := phase1.RunSupabaseAudit(ctx, sourceURL, checkedSchemaPath)
	if err != nil {
		fatal(err)
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o750); err != nil {
		fatal(err)
	}
	if err := os.WriteFile(outputPath, []byte(report.Markdown()), 0o640); err != nil {
		fatal(err)
	}

	fmt.Printf("wrote schema audit to %s\n", outputPath)
	if len(report.MissingExpected) > 0 {
		fmt.Printf("warning: %d expected columns are missing from the live schema\n", len(report.MissingExpected))
	}
	if len(report.MissingFromDBSchema) > 0 {
		fmt.Printf("warning: %d live columns appear missing from db/schema.sql\n", len(report.MissingFromDBSchema))
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
