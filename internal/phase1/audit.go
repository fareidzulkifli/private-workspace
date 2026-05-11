package phase1

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type ColumnInfo struct {
	Table      string
	Name       string
	DataType   string
	Nullable   string
	Default    string
	InDBSchema bool
}

type ConstraintInfo struct {
	Table        string
	Name         string
	Type         string
	Column       string
	ForeignTable string
	ForeignCol   string
}

type IndexInfo struct {
	Table string
	Name  string
	Def   string
}

type EnumInfo struct {
	Type  string
	Value string
}

type RowCountInfo struct {
	Table string
	Count int64
}

type AuditReport struct {
	GeneratedAt         time.Time
	Source              string
	Columns             []ColumnInfo
	Constraints         []ConstraintInfo
	Indexes             []IndexInfo
	Enums               []EnumInfo
	RowCounts           []RowCountInfo
	MissingExpected     []string
	MissingFromDBSchema []string
	CheckedDBSchemaPath string
}

func RunSupabaseAudit(ctx context.Context, sourceURL, checkedSchemaPath string) (*AuditReport, error) {
	if sourceURL == "" {
		return nil, fmt.Errorf("source database URL is required")
	}

	conn, err := pgx.Connect(ctx, sourceURL)
	if err != nil {
		return nil, fmt.Errorf("connect to supabase/postgres: %w", err)
	}
	defer conn.Close(ctx)

	report := &AuditReport{
		GeneratedAt:         time.Now().UTC(),
		Source:              sourceURL,
		CheckedDBSchemaPath: checkedSchemaPath,
	}

	report.Columns, err = auditColumns(ctx, conn)
	if err != nil {
		return nil, err
	}
	report.Constraints, err = auditConstraints(ctx, conn)
	if err != nil {
		return nil, err
	}
	report.Indexes, err = auditIndexes(ctx, conn)
	if err != nil {
		return nil, err
	}
	report.Enums, err = auditEnums(ctx, conn)
	if err != nil {
		return nil, err
	}
	report.RowCounts, err = auditRowCounts(ctx, conn)
	if err != nil {
		return nil, err
	}

	report.MissingExpected = missingExpectedColumns(report.Columns)
	if checkedSchemaPath != "" {
		if schemaColumns, err := parseCheckedSchemaColumns(checkedSchemaPath); err == nil {
			for i := range report.Columns {
				if schemaColumns[report.Columns[i].Table][report.Columns[i].Name] {
					report.Columns[i].InDBSchema = true
				} else {
					report.MissingFromDBSchema = append(report.MissingFromDBSchema, report.Columns[i].Table+"."+report.Columns[i].Name)
				}
			}
			sort.Strings(report.MissingFromDBSchema)
		}
	}

	return report, nil
}

func (r AuditReport) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Live Supabase Schema Audit\n\n")
	fmt.Fprintf(&b, "- Generated at: %s\n", r.GeneratedAt.UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "- Source database: `%s`\n", RedactDSN(r.Source))
	if r.CheckedDBSchemaPath != "" {
		fmt.Fprintf(&b, "- Checked schema file: `%s`\n", r.CheckedDBSchemaPath)
	}
	fmt.Fprintf(&b, "\n")

	fmt.Fprintf(&b, "## Columns\n\n")
	fmt.Fprintf(&b, "| Table | Column | Type | Nullable | Default | In db/schema.sql |\n")
	fmt.Fprintf(&b, "| --- | --- | --- | --- | --- | --- |\n")
	for _, column := range r.Columns {
		fmt.Fprintf(&b, "| `%s` | `%s` | `%s` | `%s` | `%s` | `%t` |\n",
			column.Table,
			column.Name,
			column.DataType,
			column.Nullable,
			escapeMarkdownTable(column.Default),
			column.InDBSchema,
		)
	}

	fmt.Fprintf(&b, "\n## Constraints\n\n")
	fmt.Fprintf(&b, "| Table | Constraint | Type | Column | Foreign Table | Foreign Column |\n")
	fmt.Fprintf(&b, "| --- | --- | --- | --- | --- | --- |\n")
	for _, constraint := range r.Constraints {
		fmt.Fprintf(&b, "| `%s` | `%s` | `%s` | `%s` | `%s` | `%s` |\n",
			constraint.Table,
			constraint.Name,
			constraint.Type,
			constraint.Column,
			constraint.ForeignTable,
			constraint.ForeignCol,
		)
	}

	fmt.Fprintf(&b, "\n## Indexes\n\n")
	fmt.Fprintf(&b, "| Table | Index | Definition |\n")
	fmt.Fprintf(&b, "| --- | --- | --- |\n")
	for _, index := range r.Indexes {
		fmt.Fprintf(&b, "| `%s` | `%s` | `%s` |\n", index.Table, index.Name, escapeMarkdownTable(index.Def))
	}

	fmt.Fprintf(&b, "\n## Enums\n\n")
	fmt.Fprintf(&b, "| Type | Value |\n")
	fmt.Fprintf(&b, "| --- | --- |\n")
	for _, enum := range r.Enums {
		fmt.Fprintf(&b, "| `%s` | `%s` |\n", enum.Type, enum.Value)
	}

	fmt.Fprintf(&b, "\n## Row Counts\n\n")
	fmt.Fprintf(&b, "| Table | Count |\n")
	fmt.Fprintf(&b, "| --- | ---: |\n")
	for _, count := range r.RowCounts {
		fmt.Fprintf(&b, "| `%s` | %d |\n", count.Table, count.Count)
	}

	fmt.Fprintf(&b, "\n## Discrepancies\n\n")
	if len(r.MissingExpected) == 0 {
		fmt.Fprintf(&b, "- No expected application columns are missing from the live schema.\n")
	} else {
		for _, missing := range r.MissingExpected {
			fmt.Fprintf(&b, "- Expected application column missing from live schema: `%s`\n", missing)
		}
	}
	if len(r.MissingFromDBSchema) == 0 {
		fmt.Fprintf(&b, "- No live columns were detected as missing from `db/schema.sql` by the lightweight parser.\n")
	} else {
		for _, missing := range r.MissingFromDBSchema {
			fmt.Fprintf(&b, "- Live column appears missing from `db/schema.sql`: `%s`\n", missing)
		}
	}

	return b.String()
}

func auditColumns(ctx context.Context, conn *pgx.Conn) ([]ColumnInfo, error) {
	rows, err := conn.Query(ctx, `
		SELECT table_name, column_name, data_type, is_nullable, column_default
		FROM information_schema.columns
		WHERE table_schema = 'public'
		ORDER BY table_name, ordinal_position
	`)
	if err != nil {
		return nil, fmt.Errorf("audit columns: %w", err)
	}
	defer rows.Close()

	var columns []ColumnInfo
	for rows.Next() {
		var col ColumnInfo
		var def sql.NullString
		if err := rows.Scan(&col.Table, &col.Name, &col.DataType, &col.Nullable, &def); err != nil {
			return nil, err
		}
		if def.Valid {
			col.Default = def.String
		}
		columns = append(columns, col)
	}
	return columns, rows.Err()
}

func auditConstraints(ctx context.Context, conn *pgx.Conn) ([]ConstraintInfo, error) {
	rows, err := conn.Query(ctx, `
		SELECT
		  tc.table_name,
		  tc.constraint_name,
		  tc.constraint_type,
		  kcu.column_name,
		  ccu.table_name AS foreign_table_name,
		  ccu.column_name AS foreign_column_name
		FROM information_schema.table_constraints tc
		LEFT JOIN information_schema.key_column_usage kcu
		  ON tc.constraint_name = kcu.constraint_name
		 AND tc.table_schema = kcu.table_schema
		LEFT JOIN information_schema.constraint_column_usage ccu
		  ON ccu.constraint_name = tc.constraint_name
		 AND ccu.table_schema = tc.table_schema
		WHERE tc.table_schema = 'public'
		ORDER BY tc.table_name, tc.constraint_name, kcu.ordinal_position
	`)
	if err != nil {
		return nil, fmt.Errorf("audit constraints: %w", err)
	}
	defer rows.Close()

	var constraints []ConstraintInfo
	for rows.Next() {
		var item ConstraintInfo
		var column, foreignTable, foreignColumn sql.NullString
		if err := rows.Scan(&item.Table, &item.Name, &item.Type, &column, &foreignTable, &foreignColumn); err != nil {
			return nil, err
		}
		item.Column = column.String
		item.ForeignTable = foreignTable.String
		item.ForeignCol = foreignColumn.String
		constraints = append(constraints, item)
	}
	return constraints, rows.Err()
}

func auditIndexes(ctx context.Context, conn *pgx.Conn) ([]IndexInfo, error) {
	rows, err := conn.Query(ctx, `
		SELECT tablename, indexname, indexdef
		FROM pg_indexes
		WHERE schemaname = 'public'
		ORDER BY tablename, indexname
	`)
	if err != nil {
		return nil, fmt.Errorf("audit indexes: %w", err)
	}
	defer rows.Close()

	var indexes []IndexInfo
	for rows.Next() {
		var item IndexInfo
		if err := rows.Scan(&item.Table, &item.Name, &item.Def); err != nil {
			return nil, err
		}
		indexes = append(indexes, item)
	}
	return indexes, rows.Err()
}

func auditEnums(ctx context.Context, conn *pgx.Conn) ([]EnumInfo, error) {
	rows, err := conn.Query(ctx, `
		SELECT t.typname, e.enumlabel
		FROM pg_type t
		JOIN pg_enum e ON t.oid = e.enumtypid
		JOIN pg_namespace n ON n.oid = t.typnamespace
		WHERE n.nspname = 'public'
		ORDER BY t.typname, e.enumsortorder
	`)
	if err != nil {
		return nil, fmt.Errorf("audit enums: %w", err)
	}
	defer rows.Close()

	var enums []EnumInfo
	for rows.Next() {
		var item EnumInfo
		if err := rows.Scan(&item.Type, &item.Value); err != nil {
			return nil, err
		}
		enums = append(enums, item)
	}
	return enums, rows.Err()
}

func auditRowCounts(ctx context.Context, conn *pgx.Conn) ([]RowCountInfo, error) {
	rows, err := conn.Query(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_type = 'BASE TABLE'
		ORDER BY table_name
	`)
	if err != nil {
		return nil, fmt.Errorf("list tables for counts: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var counts []RowCountInfo
	for _, table := range tables {
		var count int64
		if err := conn.QueryRow(ctx, "SELECT COUNT(*) FROM "+quotePGIdent(table)).Scan(&count); err != nil {
			return nil, fmt.Errorf("count %s: %w", table, err)
		}
		counts = append(counts, RowCountInfo{Table: table, Count: count})
	}
	return counts, nil
}

func missingExpectedColumns(columns []ColumnInfo) []string {
	live := map[string]map[string]bool{}
	for _, column := range columns {
		if live[column.Table] == nil {
			live[column.Table] = map[string]bool{}
		}
		live[column.Table][column.Name] = true
	}

	var missing []string
	for _, table := range TableSpecs() {
		for _, column := range table.Columns {
			if !live[table.Name][column.Name] {
				missing = append(missing, table.Name+"."+column.Name)
			}
		}
	}
	sort.Strings(missing)
	return missing
}

func parseCheckedSchemaColumns(path string) (map[string]map[string]bool, error) {
	file, err := os.Open(path)
	if err != nil {
		if !filepath.IsAbs(path) {
			if found, findErr := findFileUpward(".", path); findErr == nil {
				file, err = os.Open(found)
			}
		}
		if err != nil {
			return nil, err
		}
	}
	defer file.Close()

	createRe := regexp.MustCompile(`(?i)^\s*CREATE\s+TABLE\s+IF\s+NOT\s+EXISTS\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*\(`)
	alterRe := regexp.MustCompile(`(?i)^\s*ALTER\s+TABLE\s+([a-zA-Z_][a-zA-Z0-9_]*)\s+ADD\s+COLUMN\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-zA-Z_][a-zA-Z0-9_]*)`)
	columnRe := regexp.MustCompile(`^\s*([a-zA-Z_][a-zA-Z0-9_]*)\s+`)

	result := map[string]map[string]bool{}
	currentTable := ""
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}
		if match := createRe.FindStringSubmatch(line); len(match) == 2 {
			currentTable = match[1]
			if result[currentTable] == nil {
				result[currentTable] = map[string]bool{}
			}
			continue
		}
		if currentTable != "" {
			if strings.HasPrefix(line, ");") || line == ")" || line == ");" {
				currentTable = ""
				continue
			}
			if match := columnRe.FindStringSubmatch(line); len(match) == 2 {
				keyword := strings.ToUpper(match[1])
				if keyword != "CONSTRAINT" && keyword != "PRIMARY" && keyword != "FOREIGN" && keyword != "UNIQUE" && keyword != "CHECK" {
					result[currentTable][match[1]] = true
				}
			}
		}
		if match := alterRe.FindStringSubmatch(line); len(match) == 3 {
			if result[match[1]] == nil {
				result[match[1]] = map[string]bool{}
			}
			result[match[1]][match[2]] = true
		}
	}
	return result, scanner.Err()
}

func findFileUpward(start, relative string) (string, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(abs, relative)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
		next := filepath.Dir(abs)
		if next == abs {
			return "", fmt.Errorf("%s not found", relative)
		}
		abs = next
	}
}
