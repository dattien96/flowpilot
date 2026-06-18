---
name: BUG-088-Desktop-History-Sync-Actions-Missing-In-Progress-Indicator
description: The Navigator sidebar did not show an in-progress indicator after users triggered per-chat sync or project-level sync-all actions.
metadata:
  type: bugfix
---

## Metadata

- Document ID: `BUG-088`
- Title: Desktop History Sync Actions Missing In-Progress Indicator
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `—`
- Created: `2026-06-19`
- Last Updated: `2026-06-19`
- Parent Documents: [CP-33: Desktop Project Chat Drive Folder Selection](../../07-Coding-Plan/todo/CP-33-Desktop-Project-Chat-Drive-Folder-Selection.md), [SD-14: Codex Cross-Account Chat Resume And Home Sync](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md)
- Child Documents: `—`
- Related Documents: `BUG-081-Desktop-Sidebar-Sync-Button-Shows-Reload-Icon.md`, `CA-103-navigator-sync-progress-indicator.md`
- Replaces: `—`
- Tags: `desktop, navigator, chat-sync, ui, regression, severity-medium`

## AI Quick View

### Summary

- The store already sets `runHistory[i].syncStatus = "syncing"` during Drive sync.
- The Navigator sidebar did not surface that state in the normal history row after a sync started.
- The project-level `Sync all` chip also stayed visually idle while a batch sync was running.

### Current Ask

- Show visible progress feedback for both per-chat sync and project-level sync-all actions in the Navigator.

### Key Decisions

- `V-1` Reuse the existing row-level `syncStatus: "syncing"` state instead of introducing a second sync-progress source of truth.
- `V-2` Derive project-level batch progress from any chat row in that project being `syncing`.

### Constraints

- Keep the fix inside desktop Navigator UI; do not change runner sync semantics.
- Do not regress the existing row `syncStatus` lifecycle used by retry and failure states.

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/src/components/Navigator.tsx`
- `apps/desktop-flowpilot/src/components/navigatorHistory.ts`
- `apps/desktop-flowpilot/src/components/navigatorHistory.test.ts`
- `apps/desktop-flowpilot/src/styles.css`

## 1. Issue Summary

Users could trigger sync from the Navigator, but the sidebar did not clearly show that sync was in progress. This made both single-chat sync and project-level sync-all feel unresponsive even when the request had already started.

## 2. Parent Links

- impacted coding plan: `CP-33-Desktop-Project-Chat-Drive-Folder-Selection.md`
- impacted tech design: `SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md`
- impacted system spec: `SS-11-Workflow-With_Session.md`

## 3. Environment and Reproduction

- environment: Desktop Electron app, Navigator sidebar, project chat history with Drive sync enabled
- reproduction steps:
  - Trigger sync for one chat from the sidebar flow
  - Trigger sync-all for a project with unsynced chats
  - Observe the sidebar immediately after the action starts
- frequency: 100%

## 4. Expected vs Actual

- expected: The affected chat row and the project sync-all chip should visibly indicate that sync is running
- actual: The sidebar remained visually idle, so users could not tell whether sync had started

## 5. Impact

- users affected: Any desktop user syncing chat history to Drive
- workflows affected: Manual sync retry, batch sync, confidence in sidebar actions
- severity: Medium

## 6. Root Cause

- hypothesis: Sync progress existed in store state but was only partially rendered by the Navigator UI
- confirmed cause: `syncHistoryRun` writes `syncStatus: "syncing"` into `runHistory`, but the normal history row did not render any sync-state-specific indicator. The project-level `Sync all` chip also had no derived loading state from project history.
- evidence:
  - `store.ts` already sets `syncStatus: "syncing"`
  - `Navigator.tsx` only showed a spinner inside the selection-mode sync icon path
  - the project-level sync-all button always rendered the static sync glyph and count

## 7. Fix Strategy

- `F-1` Add a pure `isProjectSyncing(history, projectId)` helper so the project-level chip can derive batch progress from row state.
- `F-2` Render a spinner plus `Syncing…` label in the project-level `Sync all` chip while any row in that project is `syncing`.
- `F-3` Render a spinner and `Syncing to Drive…` meta text in the normal history row when that row has `syncStatus === "syncing"`.
- `F-4` Add a focused helper test for the project-level sync detection.

## 8. Validation

- `V-1` `npm --prefix apps/desktop-flowpilot run typecheck`
- `V-2` `npx tsx --test apps/desktop-flowpilot/src/components/navigatorHistory.test.ts`

## 9. Regression Guard

- tests: `navigatorHistory.test.ts` covers the project-level syncing derivation
- alerts: `—`
- audit checks: The UI still uses the existing `runHistory.syncStatus` state and does not introduce a second progress model

## 10. Follow-Up Document Updates

- upstream docs that must change: None
- notes left unchanged on purpose: Runner sync behavior and chat sync transport were not changed
