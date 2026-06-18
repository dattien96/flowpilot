# BUG-078: Desktop Sidebar History Does Not Auto-Update

## Metadata

- Document ID: `BUG-078`
- Title: `Desktop Sidebar History Does Not Auto-Update`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-17`
- Last Updated: `2026-06-17`
- Parent Documents: [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md), [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: `none`
- Related Documents: [BUG-060: Desktop Run History Empties After Switching Runs](./BUG-060-Desktop-Run-History-Empties-After-Switching-Runs.md), [CA-057: Desktop Project Run History Popover](../../../change-audit/CA-057-desktop-project-run-history-popover.md), [CA-075: Desktop Chat Mode Split And BUG-060 History Fix](../../../change-audit/CA-075-desktop-chat-mode-split-and-bug060-history-fix.md), [CA-094: Fix Desktop Sidebar History Auto-Update](../../../change-audit/CA-094-fix-desktop-sidebar-history-auto-update.md)
- Replaces: `none`
- Tags: `desktop, run-history, navigator, sidebar, polling, regression`

## AI Quick View

### Summary

- The left sidebar (`Navigator.tsx`) showed stale run history after new runs were created or run state changed — it only refreshed when the user pressed the "HISTORY" button in the top-right header.
- `loadRunHistory()` in the store was gated behind `historyOpen` in the `sendPrompt` finally block, so it only fired when the top-right popover was open.
- No polling or state-change trigger was wired to the Navigator sidebar, leaving it permanently stale between project switches.
- The top-right History button duplicated history already visible in the left sidebar.

### Current Ask

- Fixed. Polling added to Navigator, `sendPrompt` always refreshes history, and the redundant History button removed.

### Key Decisions

- `V-1` TypeScript compile passes with zero errors after all changes.

### Constraints

- Polling is intentionally lightweight: 3 s when a run is active, 10 s when idle. Do not reduce polling interval below 3 s without confirming server load impact.

### Open Questions

- `none`

### Source Refs

- `apps/desktop-flowpilot/src/components/Navigator.tsx`
- `apps/desktop-flowpilot/src/components/RunStatus.tsx`
- `apps/desktop-flowpilot/src/state/store.ts`
- [CA-094](../../../change-audit/CA-094-fix-desktop-sidebar-history-auto-update.md)

## 1. Issue Summary

The left sidebar Navigator shows run history grouped by project. After a new run was created or an existing run changed state (running → completed, waiting_approval, etc.), the sidebar did not update. The only way to see fresh data was to press the "HISTORY" button in the top-right header, which triggered `toggleRunHistory()` → `loadRunHistory()`.

Additionally, the "HISTORY" button in the header was redundant since history is already visible in the left sidebar. Pressing it opened a duplicate popover with the same data.

## 2. Parent Links

- impacted coding plan: [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md)
- impacted tech design: [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md)
- impacted system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)

## 3. Environment and Reproduction

- environment: desktop-flowpilot (Electron), any OS
- reproduction steps:
  1. Select a project with an existing run history.
  2. Send a new prompt — observe the left sidebar does not add the new run entry until manually pressing "HISTORY".
  3. During an active run (status `running`), observe the sidebar status dot is stale until the History button is pressed.
- frequency: 100% reproducible

## 4. Expected vs Actual

- expected: The left sidebar refreshes automatically when runs are created or their state changes, with no manual trigger required.
- actual: The sidebar only refreshed on project switch or when the top-right "HISTORY" button was pressed. The `sendPrompt` finally block guarded `loadRunHistory()` behind `historyOpen` (the popover's open state), so closing or never opening the popover left the sidebar permanently stale.

## 5. Impact

- users affected: all desktop users using the Navigator sidebar
- workflows affected: normal chat runs, workflow runs — any flow that creates or transitions a run
- severity: UX — medium. History was accessible via the button; sidebar just required a manual refresh step.

## 6. Root Cause

- hypothesis: `loadRunHistory()` was only called when `historyOpen` was true (popover open) or on project switch.
- confirmed cause: In `store.ts` `sendPrompt` finally block, the call was: `if (get().historyOpen) { void get().loadRunHistory(); }`. The Navigator sidebar reads `runHistory` from the same store but had no independent trigger to refresh it. No polling existed for the sidebar.
- evidence: Code inspection of `store.ts` lines 431–434 (pre-fix) and `Navigator.tsx` effects (lines 64–67 only fired on `selectedProjectId` change).

## 7. Fix Strategy

- `F-1` Remove the `historyOpen` guard in `sendPrompt` finally — always call `void get().loadRunHistory()` so history refreshes after every run turn regardless of popover state.
- `F-2` Add a polling `useEffect` in `Navigator.tsx` that calls `loadRunHistory()` every 3 s when a run is active (`running | starting | waiting_approval | waiting_question`) and every 10 s when idle — matches the pattern used by `admin-web` for active sessions.
- `F-3` Remove `historyOpen: boolean` state field, `toggleRunHistory()` action, and the History button/popover from `RunStatus.tsx`. Clean up all `historyOpen: false` resets from `selectProject`, `openHistoryRun`, and `resetRun`.

## 8. Validation

- `V-1` `npx tsc --noEmit -p apps/desktop-flowpilot/tsconfig.json` → **TypeScript: No errors found**
- `V-2` Grep for `historyOpen` and `toggleRunHistory` across `apps/desktop-flowpilot/src` → **No matches found** (all references removed)
- `V-3` Manual smoke test not run in this session (no preview server available for Electron app). Structural correctness confirmed by type check.

## 9. Regression Guard

- tests: no automated UI tests exist for this component; type check is the primary gate
- alerts: none
- audit checks: confirm `runHistory` in the store still updates after `sendPrompt` completes — the finally block always fires regardless of run outcome (success or error)

## 10. Follow-Up Document Updates

- upstream docs that must change: none — this is a pure UX/auto-refresh improvement, not a behavior contract change
- notes left unchanged on purpose: BUG-060 documents the stale-response guard (`_historyLoadSeq`) which is still active and unaffected by this fix
