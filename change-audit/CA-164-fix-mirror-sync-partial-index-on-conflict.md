# CA-164: Fix Built-In Mirror Sync `on_conflict` vs Partial Unique Index (BUG-NOTE-CP42 #2)

## Scope

Verified and fixed a P1 issue from `requirements/09-BugFix/todo/BUG-NOTE-CP42.md`: the built-in flow mirror-sync upsert would likely fail against a real Postgres/PostgREST database.

## The bug

`supabase/migrations/20260701090000_add_flow_engine_attrs_to_workflows.sql` created `workflows_pack_flow_uidx` as a **partial** unique index (`on workflows(pack_id, pack_flow_id) where is_builtin = true`). `SupabaseWorkflowFlowStore.Upsert` (`supabase_workflow_flow_store.go`) targets built-in rows with `?on_conflict=pack_id,pack_flow_id` — PostgREST renders this as a bare `ON CONFLICT (pack_id, pack_flow_id)`, with no `WHERE` clause. Postgres cannot infer a partial unique index from an unqualified conflict target; every mirror-sync upsert of a built-in flow (`EnsureBuiltinFlowMirrors` at app startup) would fail with `there is no unique or exclusion constraint matching the ON CONFLICT specification` against a real database. The existing test (`supabase_workflow_flow_store_test.go`) only asserted the request URL string against a fake HTTP transport, so it could not catch this — a fake transport doesn't enforce real Postgres constraint-inference rules.

## Fix

Dropped the `where is_builtin = true` predicate — the index is now a plain `unique index on workflows(pack_id, pack_flow_id)`, matching the bare `on_conflict` target PostgREST emits. This migration was new/uncommitted (never applied to any live database), so rewriting it in place was safe; no data migration was needed. The predicate turns out to be unnecessary for correctness: non-builtin rows always leave both columns `NULL` (`Upsert`'s non-builtin branch never sets `pack_id`/`pack_flow_id` in its payload), and a standard btree unique index never treats two `NULL`s as conflicting with each other or with a builtin row's non-null values.

## Verification

- New test `TestWorkflowsPackFlowUniqueIndexIsNotPartial` (`workflow_flow_migration_sql_test.go`): reads the actual migration SQL and asserts the `CREATE UNIQUE INDEX ... workflows_pack_flow_uidx` statement contains no `WHERE` clause. No live Postgres is available in this environment to exercise the real `INSERT ... ON CONFLICT` path directly, so this guards the SQL text itself against regressing back to a partial index.
- Existing `TestSupabaseWorkflowFlowStore*` tests (URL/payload shape assertions against a fake transport) continue to pass unchanged.
- `go build ./...` clean.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-NOTE-CP42
change_type: bugfix
summary: drop the partial WHERE predicate from workflows_pack_flow_uidx so PostgREST's bare on_conflict=pack_id,pack_flow_id target can actually match it
# --->8---
