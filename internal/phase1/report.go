package phase1

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

type TableCount struct {
	Table    string
	Source   int64
	Target   int64
	Imported int64
	Passed   bool
}

type MigrationReport struct {
	StartedAt   time.Time
	CompletedAt time.Time
	Source      string
	SQLitePath  string
	DryRun      bool
	Applied     []string
	Counts      []TableCount
	Checks      []CheckResult
}

func (r MigrationReport) Passed() bool {
	for _, count := range r.Counts {
		if !count.Passed {
			return false
		}
	}
	return ChecksPassed(r.Checks)
}

func (r MigrationReport) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Supabase to SQLite Migration Report\n\n")
	fmt.Fprintf(&b, "- Migration started at: %s\n", r.StartedAt.UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "- Migration completed at: %s\n", r.CompletedAt.UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "- Source database: `%s`\n", RedactDSN(r.Source))
	fmt.Fprintf(&b, "- Target SQLite file: `%s`\n", r.SQLitePath)
	fmt.Fprintf(&b, "- Dry run: `%t`\n", r.DryRun)
	fmt.Fprintf(&b, "- Result: `%s`\n\n", passFail(r.Passed()))

	fmt.Fprintf(&b, "## Applied Migrations\n\n")
	if len(r.Applied) == 0 {
		fmt.Fprintf(&b, "- No new migrations applied.\n\n")
	} else {
		for _, version := range r.Applied {
			fmt.Fprintf(&b, "- `%s`\n", version)
		}
		fmt.Fprintf(&b, "\n")
	}

	fmt.Fprintf(&b, "## Table Counts\n\n")
	fmt.Fprintf(&b, "| Table | Source | Imported | Target | Result |\n")
	fmt.Fprintf(&b, "| --- | ---: | ---: | ---: | --- |\n")
	for _, count := range r.Counts {
		fmt.Fprintf(&b, "| `%s` | %d | %d | %d | %s |\n", count.Table, count.Source, count.Imported, count.Target, passFail(count.Passed))
	}

	fmt.Fprintf(&b, "\n## Validation\n\n")
	fmt.Fprintf(&b, "| Check | Result | Detail |\n")
	fmt.Fprintf(&b, "| --- | --- | --- |\n")
	for _, check := range r.Checks {
		fmt.Fprintf(&b, "| %s | %s | %s |\n", escapeMarkdownTable(check.Name), passFail(check.Passed), escapeMarkdownTable(check.Detail))
	}
	return b.String()
}

func RedactDSN(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		return "redacted"
	}
	if parsed.User != nil {
		username := parsed.User.Username()
		if username == "" {
			username = "user"
		}
		parsed.User = url.UserPassword(username, "redacted")
	}
	if parsed.RawQuery != "" {
		query := parsed.Query()
		for key := range query {
			query.Set(key, "redacted")
		}
		parsed.RawQuery = query.Encode()
	}
	return parsed.String()
}

func passFail(ok bool) string {
	if ok {
		return "pass"
	}
	return "fail"
}

func escapeMarkdownTable(value string) string {
	return strings.ReplaceAll(value, "|", "\\|")
}
