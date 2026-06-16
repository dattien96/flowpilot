# Task-056: History Supabase Reader Production Fix

## Metadata

- Document ID: `Task-056`
- Title: `History Supabase Reader Production Fix`
- Phase: `task`
- Status: `todo`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-16`
- Last Updated: `2026-06-16`
- Parent Documents: [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md), [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: `none`
- Related Documents: [BUG-060: Desktop Run History Empties After Switching Runs](../../09-BugFix/done/BUG-060-Desktop-Run-History-Empties-After-Switching-Runs.md), [Task-037: Desktop Project Run History Popover](../done/Task-037-Desktop-Project-Run-History-Popover.md), [CA-075: Desktop Chat Mode Split And BUG-060 History Fix](../../change-audit/CA-075-desktop-chat-mode-split-and-bug060-history-fix.md), [Task-057: Cross-PC Provider Chat Sync](./Task-057-Cross-PC-Provider-Chat-Sync.md)
- Replaces: `none`
- Tags: `local-runner, history, supabase, session, production, bug-060`

## AI Quick View

### Summary

- BUG-060 marked *done* but only the fake/dev store was fixed. The production `SupabaseWorkflowStore` does not implement `SessionHistoryReader`, so History still empties after any runner/app-server recreation in production.
- This task closes F-2 and F-5 from BUG-060: implement `ListProviderSessionsByProject` on `SupabaseWorkflowStore` and verify the `workflow_provider_sessions` migration is applied in production Supabase.
- No new UI changes required — the read path (`projectRunHistory` type assertion) and the desktop stale-response guard already exist.

### Current Ask

- Implement `SupabaseWorkflowStore.ListProviderSessionsByProject()` querying `workflow_provider_sessions` by project.
- Confirm `workflow_provider_sessions` migration is applied in the production Supabase project (F-5).
- Add a Supabase-backed integration test (V-2 from BUG-060).
- Re-label BUG-060 status to reflect full production fix.

### Key Decisions

- `T-1` Query `workflow_provider_sessions` joined/filtered by project via `workflow_run_id → workflow_runs.project_id`. Do not add a redundant `project_id` column to the sessions table.
- `T-2` Return results sorted by `updated_at desc` to match the in-memory merge in `projectRunHistory`.
- `T-3` The `SessionHistoryReader` interface remains optional (type assertion in `interactive_handlers.go:612`) — no interface change needed.
- `T-4` On query error, log and return an empty slice; do not propagate error to the caller (matches current merge pattern).

### Constraints

- Do not alter `workflow_provider_sessions` table schema — the migration exists and must not be changed.
- Do not change the `SessionHistoryReader` interface signature.
- Do not touch the desktop or admin-web layers — backend Go only.
- `SupabaseWorkflowStore` already uses the PostgREST client; stay consistent with that pattern.

### Open Questions

- Does the `workflow_provider_sessions` migration (`20260615120000_add_workflow_provider_tables.sql`) need to be manually applied to the production Supabase project, or is it applied via CI? Verify before merging.
- Should the project-filter query use a Supabase PostgREST join (`workflow_run_id!inner(project_id=eq.X)`) or a separate lookup? Prefer the join approach if PostgREST supports it for this schema.

### Source Refs

- `apps/local-runner/internal/runner/workflow_store.go:39` — `SessionHistoryReader` interface
- `apps/local-runner/internal/runner/workflow_store.go:195` — `fakeWorkflowStore.ListProviderSessionsByProject` (reference impl)
- `apps/local-runner/internal/runner/interactive_handlers.go:612` — read path type assertion
- `apps/local-runner/internal/runner/supabase_workflow_store.go` — target file for implementation
- `supabase/migrations/20260615120000_add_workflow_provider_tables.sql` — `workflow_provider_sessions` table
- BUG-060 F-2, F-5; V-2

## 1. Goal

Make the production History panel actually survive runner/app-server recreation by implementing the Supabase-backed session reader. After this task, `SupabaseWorkflowStore` satisfies `SessionHistoryReader` so that `projectRunHistory` can rehydrate runs from the persisted `workflow_provider_sessions` table on any machine using the same Supabase project.

## 2. Parent Links

- coding plan: [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md)
- tech design: [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md)
- system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- specific upstream ids: BUG-060 F-2, F-5, V-2

## 3. Trigger

BUG-060 was marked *done* after fixing the dev/demo (fake store) path with F-1, F-3, F-4. However, the production `SupabaseWorkflowStore` does not implement `SessionHistoryReader`, so in production the History panel still empties after any app-server/runner recreation. The analysis in `08-Desktop-Chat-New-Plan.md §1.2` confirmed this is the real outstanding gap.

## 4. Exact Change

- `T-1` In `apps/local-runner/internal/runner/supabase_workflow_store.go` — add method `ListProviderSessionsByProject(ctx context.Context, projectID string) ([]ProviderSessionState, error)` implementing `SessionHistoryReader`. Query: select from `workflow_provider_sessions` where the associated `workflow_run_id` belongs to the given `projectID`, ordered by `updated_at desc`. Use PostgREST join or two-step lookup (fetch run IDs for project, then sessions for those IDs).
- `T-2` Add integration test `TestSupabaseListProviderSessionsByProject` in `supabase_provider_session_store_test.go` or a new `supabase_history_reader_test.go`: insert a session row via `UpsertSession`, call `ListProviderSessionsByProject`, assert the row is returned. Mirrors the existing `fakeWorkflowStore` regression test logic.
- `T-3` Verify that `workflow_provider_sessions` migration (`20260615120000_add_workflow_provider_tables.sql`) is applied to the production Supabase project. Document result in completion notes (F-5).
- `T-4` Update `BUG-060` status from `done` to `done` with a note that F-2 and F-5 are now resolved, and mark V-2 as passing.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/supabase_workflow_store.go` (add method)
  - `apps/local-runner/internal/runner/supabase_provider_session_store_test.go` (add test, or new file)
  - `requirements/09-BugFix/done/BUG-060-Desktop-Run-History-Empties-After-Switching-Runs.md` (update status)
- modules: `local-runner`
- routes: none (backend-only change)
- tables: `workflow_provider_sessions` (read only), `workflow_runs` (join/lookup for project filter)

## 6. Acceptance Check

- `SupabaseWorkflowStore` satisfies the `SessionHistoryReader` interface (compile-time check passes).
- `TestSupabaseListProviderSessionsByProject` passes: a session upserted via `UpsertSession` is returned by `ListProviderSessionsByProject` for the correct project.
- Integration test simulating runner recreation (similar to `TestRunHistoryEmptiesAfterServiceRecreation`) passes with the Supabase store.
- Manual: start two runs, restart the runner, press History — both runs appear.
- `workflow_provider_sessions` migration confirmed applied in production Supabase (F-5 documented).

## 7. Out of Scope

- Changing the `workflow_provider_sessions` table schema.
- Adding `project_id` directly to the sessions table.
- Any desktop or admin-web UI changes.
- Phase 2 cross-PC sync (see Task-057).
- Live-updating History panel (separate follow-up from §1.3 of the plan doc).
- Provider-aware `/skills` and `/agents` slash commands.

## 8. Completion Notes

- result:
- follow-ups: Task-057 (Phase 2 cross-PC sync)
- upstream docs updated: BUG-060 (F-2/F-5/V-2), `08-Desktop-Chat-New-Plan.md §1.2`
