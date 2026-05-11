package phase1

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

type ColumnSpec struct {
	Name     string
	Kind     ColumnKind
	Fallback any
}

type TableSpec struct {
	Name    string
	Columns []ColumnSpec
	OrderBy string
}

func TableSpecs() []TableSpec {
	return []TableSpec{
		{
			Name: "organizations",
			Columns: []ColumnSpec{
				{"id", KindText, nil},
				{"name", KindText, nil},
				{"slug", KindText, nil},
				{"order_index", KindReal, float64(0)},
				{"created_at", KindTimestamp, nil},
			},
			OrderBy: "order_index, name, id",
		},
		{
			Name: "projects",
			Columns: []ColumnSpec{
				{"id", KindText, nil},
				{"org_id", KindText, nil},
				{"name", KindText, nil},
				{"order_index", KindReal, float64(0)},
				{"created_at", KindTimestamp, nil},
				{"description_markdown", KindText, nil},
				{"goal", KindText, nil},
				{"context_markdown", KindText, nil},
				{"project_type", KindText, "Work"},
				{"ai_instructions", KindText, nil},
				{"current_focus", KindText, nil},
				{"target_date", KindDate, nil},
				{"archived_at", KindTimestamp, nil},
			},
			OrderBy: "org_id, archived_at NULLS FIRST, order_index, name, id",
		},
		{
			Name: "tasks",
			Columns: []ColumnSpec{
				{"id", KindText, nil},
				{"project_id", KindText, nil},
				{"summary", KindText, nil},
				{"notes_markdown", KindText, nil},
				{"status", KindText, "In Progress"},
				{"urgent", KindBool, 0},
				{"important", KindBool, 0},
				{"created_at", KindTimestamp, nil},
				{"updated_at", KindTimestamp, nil},
				{"due_date", KindDate, nil},
				{"order_index", KindReal, float64(0)},
				{"completed_at", KindTimestamp, nil},
			},
			OrderBy: "project_id, status, order_index, created_at, id",
		},
		{
			Name: "dashboard_events",
			Columns: []ColumnSpec{
				{"id", KindText, nil},
				{"title", KindText, nil},
				{"event_date", KindDate, nil},
				{"notes", KindText, nil},
				{"color", KindText, "blue"},
				{"created_at", KindTimestamp, nil},
				{"updated_at", KindTimestamp, nil},
			},
			OrderBy: "event_date, created_at, id",
		},
		{
			Name: "prompt_templates",
			Columns: []ColumnSpec{
				{"id", KindText, nil},
				{"org_id", KindText, nil},
				{"title", KindText, nil},
				{"description", KindText, nil},
				{"category", KindText, "General"},
				{"tags", KindJSON, "[]"},
				{"body", KindText, nil},
				{"is_favorite", KindBool, 0},
				{"archived_at", KindTimestamp, nil},
				{"created_at", KindTimestamp, nil},
				{"updated_at", KindTimestamp, nil},
			},
			OrderBy: "archived_at NULLS FIRST, updated_at DESC, title, id",
		},
		{
			Name: "context_packs",
			Columns: []ColumnSpec{
				{"id", KindText, nil},
				{"org_id", KindText, nil},
				{"title", KindText, nil},
				{"description", KindText, nil},
				{"tags", KindJSON, "[]"},
				{"archived_at", KindTimestamp, nil},
				{"created_at", KindTimestamp, nil},
				{"updated_at", KindTimestamp, nil},
			},
			OrderBy: "archived_at NULLS FIRST, updated_at DESC, title, id",
		},
		{
			Name: "context_pack_items",
			Columns: []ColumnSpec{
				{"id", KindText, nil},
				{"context_pack_id", KindText, nil},
				{"title", KindText, nil},
				{"body", KindText, nil},
				{"sort_order", KindReal, float64(0)},
				{"enabled_by_default", KindBool, 1},
				{"created_at", KindTimestamp, nil},
				{"updated_at", KindTimestamp, nil},
			},
			OrderBy: "context_pack_id, sort_order, created_at, id",
		},
		{
			Name: "task_attachments",
			Columns: []ColumnSpec{
				{"id", KindText, nil},
				{"task_id", KindText, nil},
				{"filename", KindText, nil},
				{"r2_key", KindText, nil},
				{"mime_type", KindText, nil},
				{"size_bytes", KindInteger, int64(0)},
				{"uploaded_at", KindTimestamp, nil},
			},
			OrderBy: "task_id, uploaded_at, id",
		},
	}
}

func ImportTable(ctx context.Context, source PGQuerier, target SQLExecer, spec TableSpec) (int64, error) {
	rows, err := source.Query(ctx, spec.SelectSQL())
	if err != nil {
		return 0, fmt.Errorf("query source %s: %w", spec.Name, err)
	}
	defer rows.Close()

	insertSQL := spec.InsertSQL()
	var imported int64
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return imported, fmt.Errorf("read source row %s: %w", spec.Name, err)
		}
		if len(values) != len(spec.Columns) {
			return imported, fmt.Errorf("%s returned %d values for %d columns", spec.Name, len(values), len(spec.Columns))
		}

		args := make([]any, len(values))
		for i, value := range values {
			normalized, err := NormalizeValue(value, spec.Columns[i].Kind, spec.Columns[i].Fallback)
			if err != nil {
				return imported, fmt.Errorf("normalize %s.%s: %w", spec.Name, spec.Columns[i].Name, err)
			}
			args[i] = normalized
		}

		if _, err := target.ExecContext(ctx, insertSQL, args...); err != nil {
			return imported, fmt.Errorf("insert %s row %d: %w", spec.Name, imported+1, err)
		}
		imported++
	}
	if err := rows.Err(); err != nil {
		return imported, fmt.Errorf("iterate source %s: %w", spec.Name, err)
	}

	return imported, nil
}

func CountPostgresRows(ctx context.Context, source PGQuerier, table string) (int64, error) {
	var count int64
	if err := source.QueryRow(ctx, "SELECT COUNT(*) FROM "+quotePGIdent(table)).Scan(&count); err != nil {
		return 0, fmt.Errorf("count postgres %s: %w", table, err)
	}
	return count, nil
}

func CountSQLiteRows(ctx context.Context, db *sql.DB, table string) (int64, error) {
	var count int64
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+quoteSQLiteIdent(table)).Scan(&count); err != nil {
		return 0, fmt.Errorf("count sqlite %s: %w", table, err)
	}
	return count, nil
}

func (t TableSpec) SelectSQL() string {
	columns := make([]string, len(t.Columns))
	for i, column := range t.Columns {
		columns[i] = quotePGIdent(column.Name)
	}
	sql := "SELECT " + strings.Join(columns, ", ") + " FROM " + quotePGIdent(t.Name)
	if t.OrderBy != "" {
		sql += " ORDER BY " + t.OrderBy
	}
	return sql
}

func (t TableSpec) InsertSQL() string {
	columns := make([]string, len(t.Columns))
	placeholders := make([]string, len(t.Columns))
	for i, column := range t.Columns {
		columns[i] = quoteSQLiteIdent(column.Name)
		placeholders[i] = "?"
	}
	return "INSERT INTO " + quoteSQLiteIdent(t.Name) +
		" (" + strings.Join(columns, ", ") + ") VALUES (" + strings.Join(placeholders, ", ") + ")"
}

func quotePGIdent(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func quoteSQLiteIdent(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

type PGQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type SQLExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}
