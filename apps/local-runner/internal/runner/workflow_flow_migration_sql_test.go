package runner

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestWorkflowsPackFlowUniqueIndexIsNotPartial is the regression test for
// BUG-NOTE-CP42 #2: SupabaseWorkflowFlowStore.Upsert targets built-in rows
// with `?on_conflict=pack_id,pack_flow_id` (see supabase_workflow_flow_store_test.go),
// which PostgREST renders as a bare `ON CONFLICT (pack_id, pack_flow_id)` with
// no WHERE clause. Postgres cannot infer a partial unique index from an
// unqualified conflict target, so if workflows_pack_flow_uidx ever regains a
// `where is_builtin = true` predicate, every built-in mirror-sync upsert
// against a real database would fail with "no unique or exclusion constraint
// matching the ON CONFLICT specification" even though local fake-transport
// tests (which never touch real Postgres constraint resolution) keep passing.
//
// There is no live Postgres available in this test environment to exercise
// the actual INSERT ... ON CONFLICT failure, so this asserts the SQL text
// directly against the one migration that defines the index.
func TestWorkflowsPackFlowUniqueIndexIsNotPartial(t *testing.T) {
	path := "../../../../supabase/migrations/20260701090000_add_flow_engine_attrs_to_workflows.sql"
	sql, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}

	re := regexp.MustCompile(`(?is)create\s+unique\s+index[^;]*workflows_pack_flow_uidx[^;]*;`)
	stmt := re.FindString(string(sql))
	if stmt == "" {
		t.Fatal("could not find the workflows_pack_flow_uidx CREATE UNIQUE INDEX statement in the migration")
	}
	if strings.Contains(strings.ToLower(stmt), "where") {
		t.Fatalf(
			"workflows_pack_flow_uidx must not be a partial index (found a WHERE clause), "+
				"because supabase_workflow_flow_store.go's Upsert uses the bare "+
				"on_conflict=pack_id,pack_flow_id target, which cannot infer a partial index. "+
				"statement: %s", stmt,
		)
	}
}

// TestBuiltinWorkflowWritesRestrictedAtRLS is the regression test for
// BUG-NOTE-CP42 #20: "built-in workflows are read-only" was only enforced in
// application code — the original RLS policies grant every authenticated
// user unconditional read/write access, so a direct Supabase client call
// (bypassing the app entirely) could still mutate a mirrored built-in row.
// There is no live Postgres available in this test environment to exercise
// the actual RLS denial, so this asserts the migration text directly: the
// authenticated write policies for workflows/workflow_steps must gate on
// is_builtin = false, and DELETE must have its own `using` clause (a bare
// `with check` does not gate DELETE at all).
func TestBuiltinWorkflowWritesRestrictedAtRLS(t *testing.T) {
	path := "../../../../supabase/migrations/20260701100000_restrict_builtin_workflow_writes_at_rls.sql"
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := strings.ToLower(string(raw))

	for _, want := range []string{
		`create policy "authenticated insert workflows"`,
		`create policy "authenticated update workflows"`,
		`create policy "authenticated delete workflows"`,
		`create policy "authenticated insert workflow_steps"`,
		`create policy "authenticated update workflow_steps"`,
		`create policy "authenticated delete workflow_steps"`,
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration missing expected policy %q", want)
		}
	}

	// Every policy statement must reference is_builtin somewhere in its own
	// body (either directly for workflows, or via the workflows join for
	// workflow_steps) — a policy with none is a policy that grants
	// unconditional access again.
	re := regexp.MustCompile(`(?is)create\s+policy\s+"authenticated (insert|update|delete) workflow(_steps)?"[^;]*;`)
	for _, stmt := range re.FindAllString(sql, -1) {
		if !strings.Contains(stmt, "is_builtin") {
			t.Fatalf("policy statement has no is_builtin condition, grants unconditional access: %s", stmt)
		}
	}

	// DELETE specifically needs its own `using` clause — a `with check`
	// alone never applies to DELETE, so a delete-only "with check" policy
	// would silently allow deleting a built-in row.
	deleteRe := regexp.MustCompile(`(?is)create\s+policy\s+"authenticated delete workflow(_steps)?"[^;]*;`)
	for _, stmt := range deleteRe.FindAllString(sql, -1) {
		if !strings.Contains(stmt, "using") {
			t.Fatalf("DELETE policy has no USING clause, so is_builtin is never actually checked for delete: %s", stmt)
		}
	}
}
