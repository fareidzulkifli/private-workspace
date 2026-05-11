package phase1

import (
	"context"
	"testing"
)

func TestApplyMigrationsAndValidateEmptyDatabase(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/private-workspace.db"

	db, err := OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	applied, err := ApplyMigrations(ctx, db, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) == 0 {
		t.Fatal("expected at least one applied migration")
	}

	for _, table := range []string{
		"schema_migrations",
		"admin_users",
		"sessions",
		"organizations",
		"projects",
		"tasks",
		"dashboard_events",
		"prompt_templates",
		"context_packs",
		"context_pack_items",
		"task_attachments",
	} {
		if _, err := CountSQLiteRows(ctx, db, table); err != nil {
			t.Fatalf("expected table %s to exist: %v", table, err)
		}
	}

	checks, err := ValidateSQLite(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if !ChecksPassed(checks) {
		t.Fatalf("expected validation checks to pass: %#v", checks)
	}
}

func TestTableSpecsIncludeOrganizationSlug(t *testing.T) {
	for _, spec := range TableSpecs() {
		if spec.Name != "organizations" {
			continue
		}
		for _, column := range spec.Columns {
			if column.Name == "slug" {
				return
			}
		}
	}
	t.Fatal("expected organizations.slug in migration table specs")
}
