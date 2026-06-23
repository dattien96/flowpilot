## Metadata

- Document ID: `BUG-097`
- Title: `Desktop History Panel Limits Local History To Five Items And Collapses`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-06-20`
- Last Updated: `2026-06-20`
- Parent Documents: [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: none
- Related Documents: [BUG-060: Desktop Run History Empties After Switching Runs](./BUG-060-Desktop-Run-History-Empties-After-Switching-Runs.md), [BUG-078: Desktop Sidebar History Does Not Auto Update](./BUG-078-Desktop-Sidebar-History-Does-Not-Auto-Update.md)
- Replaces: none
- Tags: desktop, history, navigator, ux, regression

## AI Quick View

### Summary

- The desktop history list showed every local run at once, which made the left rail noisy and inconsistent with the remote chats UX.
- The bug was purely presentation-side: history data existed, but the navigator lacked a local collapse/expand boundary.
- The fix caps the visible list at five items and adds a local show more/show less toggle.

### Current Ask

- Capture the history-list regression as a standalone bugfix record with its root cause and validation.

### Key Decisions

- `V-1` The local history list should default to a short collapsed view so the sidebar stays scannable.
- `V-2` The toggle must be local UI state; it should not mutate stored run history.

### Constraints

- Do not change the underlying run-history data model.
- Preserve the existing order of history items.

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/src/components/Navigator.tsx`
- `apps/desktop-flowpilot/src/components/ChatWorkspace.tsx`
- `apps/desktop-flowpilot/src/state/store.ts`

## 1. Issue Summary

The local desktop history list did not cap the number of visible items and did not offer a collapsed state. Users with long histories had to scan an oversized sidebar, unlike the remote chats section that already uses a compact, collapsible presentation.

## 2. Parent Links

- impacted coding plan: [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md)
- impacted tech design: [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md)
- impacted system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)

## 3. Environment and Reproduction

- environment: desktop FlowPilot sidebar
- reproduction steps:
  1. Open the app with more than five local chat runs in history.
  2. Expand the history section.
  3. Observe that the list continues beyond the compact range instead of collapsing.
- frequency: deterministic when the local history list exceeds five items

## 4. Expected vs Actual

- expected: the history list shows a compact collapsed view by default and allows expand/collapse control
- actual: the list rendered all available items and had no local cap

## 5. Impact

- users affected: desktop users with long local histories
- workflows affected: history browsing, chat switching, sidebar scanning
- severity: low to medium, because the issue is visual but makes the sidebar harder to use

## 6. Root Cause

- hypothesis: the navigator rendered the full local history set directly
- confirmed cause: `Navigator.tsx` had no explicit local history cap or collapse toggle for the history section
- evidence: the fixed UI now uses a five-item limit with a local show more/show less control

## 7. Fix Strategy

- `F-1` Cap the visible local history list at five items.
- `F-2` Add a local expand/collapse toggle for the history section.

## 8. Validation

- `V-1` Desktop typecheck passes.
- `V-2` Manual smoke check confirms the list collapses and expands without changing the stored history.

## 9. Regression Guard

- tests: covered by desktop typecheck plus manual sidebar smoke
- alerts: none
- audit checks: the toggle only changes presentation state

## 10. Follow-Up Document Updates

- upstream docs that must change: none
- notes left unchanged on purpose: history data remains unchanged; only the presentation boundary moved
