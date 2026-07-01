# CA-157: Supabase Flow Definition Store And Startup Mirror Sync

## Scope

Close two of Task-175's remaining open items: a concrete Supabase-backed `FlowDefinitionStore` (`Q-1`), and wiring `EnsureBuiltinFlowMirrors` into `cmd/flowpilot` startup.

## Schema decision (Q-1)

Added `supabase/migrations/20260701090000_add_flow_definitions_table.sql`, a new `flow_definitions` table following this repo's existing migration conventions exactly (see `20260615120000_add_workflow_provider_tables.sql` as the template): `flow_ref` as the natural unique upsert-conflict key, `jsonb` columns for the definition body and array-ish fields (`selectable_in_json`, `chat_sub_modes_json`), RLS enabled with `authenticated`-role read/write policies, and an index scoped to mirror rows (`where source = 'supabase_builtin_mirror'`) for `FlowMirrorSyncService`'s lookup path. This is a new, additive table — no existing table touched.

## Completed

- `SupabaseFlowDefinitionStore` (`supabase_flow_definition_store.go`) implements `FlowDefinitionStore` (`GetByRef`, `GetByPackFlow`, `Upsert`) against that table via PostgREST, following `SupabaseWorkflowStore`'s exact conventions: `httpRequestFn` for a mockable transport, `Prefer: resolution=merge-duplicates,return=minimal` + `on_conflict=flow_ref` for upsert, `nilIfEmpty` for optional fields.
- `FlowDefinitionStoreFor(r *Runner, workspace string) FlowDefinitionStore` mirrors `CatalogStoreFor`'s exact precedence (found by reading `supabase_catalog_store.go`): Supabase-backed when the workspace has a resolvable API URL + key via `Runner.LoadSupabaseWorkspaceConfigWithSecret()`, else the file-backed store. This is the same real, established key-resolution path the catalog store already uses — no new credential-handling logic was invented.
- Split `EnsureBuiltinFlowMirrors` into a local-only convenience wrapper and a new `EnsureBuiltinFlowMirrorsWithStore(ctx, store)` that accepts any `FlowDefinitionStore`.
- Wired `cmd/flowpilot runner serve` (`internal/cli/root.go`) to call `runner.FlowDefinitionStoreFor(instance, cwd)` + `EnsureBuiltinFlowMirrorsWithStore` in a background goroutine at startup, right after the existing chat-summary backfill goroutine. Failure is logged, never fatal — built-in flows still resolve directly from the embedded pack as a fallback (`FlowDefinitionResolver.ResolveBuiltin`), matching the existing `sessionStore` fallback pattern in the same command.

## Verification

- `go test ./internal/runner -run 'TestSupabaseFlowDefinitionStore|TestFlowDefinitionStoreFor'` — 9 passed.
- `go build ./...`, `go vet ./internal/runner/... ./internal/cli/...`, `gofmt -l` — clean.
- Full suite: 989 passed in one run (up from 975). Re-running the full suite showed known pre-existing flakiness unrelated to this change — `TestFinalizerHookSurfacesArtifacts` and `TestProjectRunHistoryFiltersRunsByProject` intermittently fail/pass across repeated full-suite runs but pass consistently in isolation (5/5). None of this session's new test files ever appear in a failure list across multiple full-suite runs. Not investigated further — pre-existing and out of this task's scope.

## Follow-ups

- The `flow_definitions` migration has not been applied to any live Supabase project (this session has no database access); it needs a real review/apply pass before `FlowDefinitionStoreFor` will actually pick the Supabase path anywhere.
- The pre-existing test-suite flakiness noted above is worth a separate investigation (likely shared global state across parallel-ish test runs) but is out of scope here.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-175
change_type: feature
summary: add Supabase-backed FlowDefinitionStore, flow_definitions migration, and wire builtin mirror sync into runner serve startup
# --->8---
