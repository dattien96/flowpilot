# CA-187: Require Service Role Key and Enforce Built-In Read-Only at RLS (BUG-NOTE-CP42 #19, #20)

## Scope

Verified and fixed two P2 issues from `requirements/09-BugFix/todo/BUG-NOTE-CP42.md`, both about the gap between what the app enforces and what the database actually allows for `workflows`/`workflow_steps`.

## BUG#19 — anon-key fallback incompatible with RLS

`FlowDefinitionStoreFor` fell back to `resp.AnonKey` when the service role key was empty. `workflows`/`workflow_steps`' RLS policies are scoped `to authenticated`; a bare anon key authenticates PostgREST as Postgres role `anon`, not `authenticated`, so every write through that fallback would have been silently rejected by RLS — a confusing 401/403 with no clear "not configured" signal. The fallback was also already unreachable in the normal flow: `SaveSupabaseWorkspaceConfig` requires a service role key to save the workspace config at all, so a properly-configured workspace always has one.

**Fix**: `FlowDefinitionStoreFor` now requires the service role key directly — no fallback. Updated the pre-existing `TestFlowDefinitionStoreForConfiguredRunnerYieldsSupabaseStore` (which had encoded the old anon-key-only expectation) to seed a service role key via the injectable secret store, and added `TestFlowDefinitionStoreForAnonKeyOnlyYieldsNil` asserting the corrected behavior.

## BUG#20 — built-in read-only only enforced in app code

`FlowDefinitionResolver.UpdateUserFlow` and `SupabaseAdminRepository.saveWorkflow` both correctly refuse to modify a non-editable (built-in) row — but the actual RLS policies (`20260522101000_open_workflow_engine_rls_for_admin.sql`) grant every `authenticated` user unconditional `using (true) with check (true)` access. A direct Supabase client call, bypassing the app entirely, could still mutate a mirrored built-in row — exactly the failure case CP-42 explicitly lists as something to prevent.

**Fix**: added a new migration (`20260701100000_restrict_builtin_workflow_writes_at_rls.sql`) — the existing RLS migration is already applied, so amended via a new migration rather than editing it in place, per standard practice. Splits the single `for all` policies into separate INSERT/UPDATE/DELETE policies gated on `is_builtin = false`:
- `workflows`: direct column check.
- `workflow_steps` (no `is_builtin` column of its own): an `exists` subquery against the parent `workflows` row via `workflow_id`.
- DELETE gets its own explicit `using` clause on both tables — a bare `with check` never applies to DELETE, so a delete-only `with check` policy would have silently left built-in rows deletable.

This does not block the legitimate mirror-sync path: as of BUG#19's fix, mirror-sync always authenticates with the service role key, and Postgres/Supabase's `service_role` always bypasses RLS entirely by design.

**Not applied to any live database** from this environment — creating the migration file is the appropriate scope for this fix; actual deployment (`supabase db push` or equivalent) is a separate, deliberate step outside this session.

## Verification

- `TestFlowDefinitionStoreForAnonKeyOnlyYieldsNil` (new) / `TestFlowDefinitionStoreForConfiguredRunnerYieldsSupabaseStore` (updated) — 4/4 `TestFlowDefinitionStoreFor*` tests pass.
- `TestBuiltinWorkflowWritesRestrictedAtRLS` (new, `workflow_flow_migration_sql_test.go`): asserts the migration text directly — all six policies exist, each references `is_builtin`, and both DELETE policies have their own `using` clause. No live Postgres is available in this environment to exercise the actual RLS denial directly.
- Full suite: 1014 passed (runner), 15 pre-existing/environmental failures unchanged; `internal/agentpack` unaffected.
- `go build ./...` clean.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-NOTE-CP42
change_type: bugfix
summary: FlowDefinitionStoreFor now requires a real service role key instead of falling back to an anon key RLS would reject, and a new migration enforces is_builtin=false at the RLS level for authenticated writes to workflows/workflow_steps so a direct Supabase client can no longer mutate a built-in row
# --->8---
