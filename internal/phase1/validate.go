package phase1

import (
	"context"
	"database/sql"
	"fmt"
)

type CheckResult struct {
	Name   string
	Passed bool
	Detail string
}

func ValidateSQLite(ctx context.Context, db *sql.DB) ([]CheckResult, error) {
	var checks []CheckResult
	addCountCheck := func(name, query string) error {
		var count int64
		if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		checks = append(checks, CheckResult{
			Name:   name,
			Passed: count == 0,
			Detail: fmt.Sprintf("%d invalid rows", count),
		})
		return nil
	}

	if err := addCountCheck("organizations.slug non-empty", "SELECT COUNT(*) FROM organizations WHERE trim(slug) = ''"); err != nil {
		return checks, err
	}
	if err := addCountCheck("tasks.status valid", "SELECT COUNT(*) FROM tasks WHERE status NOT IN ('In Progress', 'Done', 'KIV')"); err != nil {
		return checks, err
	}
	if err := addCountCheck("projects.project_type valid", "SELECT COUNT(*) FROM projects WHERE project_type NOT IN ('Work', 'Personal', 'Learning', 'Creative', 'Admin')"); err != nil {
		return checks, err
	}
	if err := addCountCheck("prompt_templates.tags valid JSON", "SELECT COUNT(*) FROM prompt_templates WHERE NOT json_valid(tags)"); err != nil {
		return checks, err
	}
	if err := addCountCheck("context_packs.tags valid JSON", "SELECT COUNT(*) FROM context_packs WHERE NOT json_valid(tags)"); err != nil {
		return checks, err
	}
	if err := addCountCheck("task_attachments.r2_key non-empty", "SELECT COUNT(*) FROM task_attachments WHERE trim(r2_key) = ''"); err != nil {
		return checks, err
	}

	fkRows, err := countRows(ctx, db, "PRAGMA foreign_key_check")
	if err != nil {
		return checks, err
	}
	checks = append(checks, CheckResult{
		Name:   "PRAGMA foreign_key_check",
		Passed: fkRows == 0,
		Detail: fmt.Sprintf("%d rows", fkRows),
	})

	var integrity string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		return checks, fmt.Errorf("PRAGMA integrity_check: %w", err)
	}
	checks = append(checks, CheckResult{
		Name:   "PRAGMA integrity_check",
		Passed: integrity == "ok",
		Detail: integrity,
	})

	return checks, nil
}

func countRows(ctx context.Context, db *sql.DB, query string) (int64, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("count rows for %q: %w", query, err)
	}
	defer rows.Close()

	var count int64
	for rows.Next() {
		count++
	}
	if err := rows.Err(); err != nil {
		return count, err
	}
	return count, nil
}

func ChecksPassed(checks []CheckResult) bool {
	for _, check := range checks {
		if !check.Passed {
			return false
		}
	}
	return true
}
