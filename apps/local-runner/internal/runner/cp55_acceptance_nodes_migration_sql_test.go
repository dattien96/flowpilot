package runner

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestAcceptanceNodesMigrationAddsNotNullDefaultEmptyJSONBColumn is the
// focused migration SQL-content test for CP-55 P-1 pass-3 review blocker #2
// (Task-263/CA-424): prove
// supabase/migrations/20260730121000_add_workflow_acceptance_nodes.sql
// actually adds `acceptance_nodes_json` to `workflows` as `jsonb not null
// default '[]'`, and that the migration is additive/compatible with existing
// rows (no separate backfill/update statement is needed, since `add column
// ... default '[]'::jsonb` already populates every pre-existing row). Mirrors
// the regex-over-migration-text style already used by
// workflow_flow_migration_sql_test.go in this same package. New file — no
// pre-existing test is modified.
func TestAcceptanceNodesMigrationAddsNotNullDefaultEmptyJSONBColumn(t *testing.T) {
	path := "../../../../supabase/migrations/20260730121000_add_workflow_acceptance_nodes.sql"
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := strings.ToLower(string(raw))

	re := regexp.MustCompile(`(?is)alter\s+table\s+workflows\s+add\s+column\s+if\s+not\s+exists\s+acceptance_nodes_json[^;]*;`)
	stmt := re.FindString(sql)
	if stmt == "" {
		t.Fatalf("could not find an ALTER TABLE workflows ADD COLUMN IF NOT EXISTS acceptance_nodes_json statement, got: %s", string(raw))
	}

	for _, want := range []string{"jsonb", "not null", "default", "'[]'::jsonb"} {
		if !strings.Contains(stmt, want) {
			t.Fatalf("acceptance_nodes_json column statement missing %q: %s", want, stmt)
		}
	}

	// IF NOT EXISTS (rather than a bare ADD COLUMN) is what makes this
	// migration safe to run against a database that may already have this
	// column (idempotent reruns); the column DEFAULT is what makes it
	// compatible with rows that already exist in `workflows` today — every
	// one of them gets '[]' at column-creation time, with no separate
	// backfill/UPDATE statement required or present.
	if !strings.Contains(stmt, "if not exists") {
		t.Fatalf("acceptance_nodes_json column must be added with IF NOT EXISTS for a safe/idempotent rerun: %s", stmt)
	}
	if strings.Contains(sql, "update ") || strings.Contains(sql, "backfill") {
		t.Fatalf("migration must be purely additive (the column DEFAULT covers every existing row) with no separate backfill/update statement: %s", string(raw))
	}
}
