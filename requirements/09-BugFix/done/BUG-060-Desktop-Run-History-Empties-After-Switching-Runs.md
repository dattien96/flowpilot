# BUG-060: Desktop Run History Empties After Switching Runs

## Metadata

- Document ID: `BUG-060`
- Title: `Desktop Run History Empties After Switching Runs`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-15`
- Last Updated: `2026-06-15`
- Parent Documents: [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md), [04: Detailed Coding Plan](../../10-Refactor/New-System/04-Detailed-Coding-Plan.md), [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: `none`
- Related Documents: [BUG-052: Desktop Stale Stream Events Cross Runs](../done/BUG-052-Desktop-Stale-Stream-Events-Cross-Runs.md), [Task-044: Desktop Chat Mode Split And Provider Controls](../../08-Task/done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md), [08: Desktop Chat New Plan](../../10-Refactor/New-System/08-Desktop-Chat-New-Plan.md), [CA-020: Workflow UI Chat Feed and Continue Flow](../../change-audit/CA-020-workflow-ui-chat-feed-and-continue-flow.md), [CA-021: Workflow Chat Runtime and Font Tuning](../../change-audit/CA-021-workflow-chat-runtime-and-font-tuning.md), [CA-075: Desktop Chat Mode Split And BUG-060 History Fix](../../change-audit/CA-075-desktop-chat-mode-split-and-bug060-history-fix.md)
- Replaces: `none`
- Tags: `desktop, run-history, workflow-runs, persistence, runner, race-condition, regression`

## AI Quick View

### Summary

- Starting two runs and switching back and forth between them eventually makes the desktop History panel show empty ("No runs for this project"), even though runs exist.
- The runner served run history exclusively from an in-memory map (`InteractiveService.s.runs`) that was never rehydrated from the persisted session store — so any `InteractiveService` recreation (app-server/active-account change or process restart) dropped history.
- Desktop `loadRunHistory` had no stale-response guard; overlapping calls (e.g. `toggleRunHistory` + `sendPrompt` finally-refetch) let a slower/older response win, and errors left `runHistory` at the post-`selectProject` empty baseline.

### Current Ask

- Fixed. All of F-1–F-5 implemented. F-2 (`SupabaseWorkflowStore.ListProviderSessionsByProject`) resolved by Task-056; F-5 migration is present in `20260615120000_add_workflow_provider_tables.sql` and must be confirmed applied in the production Supabase project before deploy.

### Key Decisions

- `V-1` Run history must survive a runner / app-server / active-account recreation within the same machine, since session state is already persisted.
- `V-2` The history read path sources from the persisted session store via an optional `SessionHistoryReader` interface (read-through, not rehydration into `s.runs`).
- `V-3` Desktop `loadRunHistory` uses a sequence counter to discard stale responses; errors surface as a distinct error state (not an empty list).

### Constraints

- `SessionHistoryReader` is now implemented by both `fakeWorkflowStore` (dev/demo) and `SupabaseWorkflowStore` (production). The interface remains optional (type assertion at `interactive_handlers.go:612`).

### Open Questions

- (F-5) Confirm `workflow_provider_sessions` migration (`20260615120000_add_workflow_provider_tables.sql`) is applied in the production Supabase project before deploying.

### Source Refs

- `apps/local-runner/internal/runner/workflow_store.go` — `SessionHistoryReader` interface + `fakeWorkflowStore.ListProviderSessionsByProject`.
- `apps/local-runner/internal/runner/interactive_handlers.go` — `projectRunHistory` updated.
- `apps/local-runner/internal/runner/bug060_test.go` — `TestRunHistoryEmptiesAfterServiceRecreation`.
- `apps/desktop-flowpilot/src/state/store.ts` — `_historyLoadSeq`, `historyLoadError`, updated `loadRunHistory`.
- `apps/desktop-flowpilot/src/components/RunStatus.tsx` — error state vs empty state.

## 1. Issue Summary

On the desktop FlowPilot app, after starting two workflow runs and switching back and forth between them (via the History panel) for a while, opening the History panel at some point shows an empty list ("No runs for this project"), even though the runs were started and should appear.

## 2. Parent Links

- impacted coding plan: [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md), [04: Detailed Coding Plan](../../10-Refactor/New-System/04-Detailed-Coding-Plan.md)
- impacted tech design: [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md)
- impacted system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)

## 3. Environment and Reproduction

- environment: desktop-flowpilot (Electron renderer) against the local Go runner (`apps/local-runner`), `HttpWsRunnerClient` transport.
- reproduction steps:
  1. Select a project.
  2. Start run A (send a prompt), then start run B (New run → send another prompt).
  3. Open History, switch to run A; open History, switch to run B; repeat several times.
  4. At some point the `InteractiveService` is recreated (app-server recreation or dev runner restart), causing `s.runs` to be wiped.
  5. Open History again — empty.
- frequency: intermittent; deterministically reproduced by the `TestRunHistoryEmptiesAfterServiceRecreation` test.

## 4. Expected vs Actual

- expected: the History panel lists all runs for the selected project regardless of how many times the user switched between runs, and survives runner/app-server recreation since session state is persisted.
- actual: after recreation, the History panel renders "No runs for this project" (empty list).

## 5. Impact

- users affected: desktop users running multiple runs per project.
- workflows affected: run history / resume-from-history; also blocked Task-044 normal-chat history (`T-7`).
- severity: medium — no data corruption, but users lose the ability to find/resume prior runs.

## 6. Root Cause

- confirmed cause:
  1. `projectRunHistory` iterated only `s.runs` (in-memory, process-scoped). No rehydration from any persisted store on startup.
  2. Session state IS persisted via `persistProviderSession` → `UpsertProviderSession`, but the history read path ignored it. With the fake store, persistence was a no-op (`return nil`), so nothing survived at all.
  3. Desktop `loadRunHistory` had no request-sequencing guard; overlapping calls let a slower response win. On fetch error it left `runHistory` at the empty baseline from `selectProject`.

- evidence: `TestRunHistoryEmptiesAfterServiceRecreation` confirmed `len=0, want 2` before the fix.

## 7. Fix Strategy

- `F-1` ✓ Added optional `SessionHistoryReader` interface; `fakeWorkflowStore` implements `ListProviderSessionsByProject`; `projectRunHistory` merges persisted sessions.
- `F-2` ✓ `SupabaseWorkflowStore.ListProviderSessionsByProject` implemented via PostgREST inner join on `workflow_runs` (Task-056). Compile-time interface check in `supabase_workflow_store_test.go`.
- `F-3` ✓ `_historyLoadSeq` stale-response guard added to desktop `loadRunHistory`.
- `F-4` ✓ `historyLoadError` state; `RunStatus.tsx` shows distinct error state vs empty state.
- `F-5` ⏳ `workflow_provider_sessions` migration (`20260615120000_add_workflow_provider_tables.sql`) exists; must be confirmed applied in production Supabase before deploying.

## 8. Validation

- `V-1` ✓ `TestRunHistoryEmptiesAfterServiceRecreation` — PASS after F-1.
- `V-2` ✓ `TestSupabaseWorkflowStoreListProviderSessionsByProject` — PASS; verifies GET shape, inner join filter, and `ProviderSessionState` mapping. `var _ SessionHistoryReader = (*SupabaseWorkflowStore)(nil)` compile-time guard.
- `V-3` ✓ Stale/error guard in place; error shows distinct state in History panel.
- `V-4` Manual: switch between 2 runs repeatedly; History remains populated throughout (in dev/demo mode with fake store).

## 9. Regression Guard

- tests: `TestRunHistoryEmptiesAfterServiceRecreation` in `apps/local-runner/internal/runner/bug060_test.go`.
- alerts: none.
- audit checks: [CA-075](../../change-audit/CA-075-desktop-chat-mode-split-and-bug060-history-fix.md).

## 10. Follow-Up Document Updates

- F-2: `SupabaseWorkflowStore` implements `SessionHistoryReader` — update this doc when done.
- F-5: `workflow_provider_sessions` migration confirmed — update `SS-11` / `SD-12` / `04` shared contract when confirmed.
