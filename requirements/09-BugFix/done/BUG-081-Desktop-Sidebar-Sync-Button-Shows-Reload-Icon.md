---
name: BUG-081-Desktop-Sidebar-Sync-Button-Shows-Reload-Icon
description: The per-chat and per-project sync buttons in the Navigator sidebar rendered a single circular-arrow (reload) glyph instead of the intended two-arrow sync glyph.
metadata:
  type: bugfix
---

## Metadata

- Document ID: `BUG-081`
- Title: Desktop Sidebar Sync Button Shows Reload Icon Instead of Sync Icon
- Phase: `bugfix`
- Status: `done`
- Owner: DatNguyen
- Reviewers: —
- Created: 2026-06-18
- Last Updated: 2026-06-18
- Parent Documents: `Task-068`, `Task-069`
- Child Documents: —
- Related Documents: `BUG-078-Desktop-Sidebar-History-Does-Not-Auto-Update.md`
- Replaces: —
- Tags: ui, navigator, icon, regression, severity-low

## AI Quick View

### Summary

- The `SyncGlyph` component in `Navigator.tsx` used a single nearly-complete arc with an L-shaped corner arrowhead — the standard browser reload / refresh icon shape (⟳).
- The sync button appears on every unsynced chat row and on the per-project "Sync all" chip.
- Users see a reload icon where they should see a two-arrow sync icon, causing confusion about the action's intent.

### Current Ask

- Replace the single-arc reload SVG with a two-arc opposing-arrow sync SVG.

### Key Decisions

- `V-1` The replacement icon must use two opposing arcs with arrowheads to clearly communicate bidirectional sync, not page reload.

### Constraints

- Icon must render legibly at 11×11 px (the rendered size of the `SyncGlyph`).
- Must use `currentColor` stroke so it inherits theme color.
- Must not introduce any new dependencies (no icon library import).

### Open Questions

- None.

### Source Refs

- Screenshot provided by user showing ⟳ reload icon on all sync buttons.
- `apps/desktop-flowpilot/src/components/Navigator.tsx` lines 36–43 (original), 36–48 (fixed).

## 1. Issue Summary

The per-chat sync button and per-project "Sync N" chip in the left-sidebar Navigator both use a component called `SyncGlyph` to render a small icon. The original SVG drew a single circular arc covering ~350° of a circle with an L-shaped arrowhead at the open end — visually identical to a browser reload / refresh icon. The correct icon for a sync action is two opposing curved arrows, each spanning roughly half the circle, indicating bidirectional or two-way synchronization.

## 2. Parent Links

- impacted coding plan: `CP-18-Refactor-Workflow-With_Session.md` (chat history sync surface)
- impacted tech design: —
- impacted system spec: —

## 3. Environment and Reproduction

- environment: Desktop Electron app, any OS
- reproduction steps: Open Navigator sidebar → observe the circular icon on any unsynced chat row or the project-level "Sync N" chip
- frequency: 100% reproducible — all users on all builds

## 4. Expected vs Actual

- expected: Two opposing curved arrows (standard sync icon) indicating two-way sync to Google Drive
- actual: Single circular arrow (standard browser reload icon) suggesting a page refresh action

## 5. Impact

- users affected: All users who have unsynced chat history
- workflows affected: Visual clarity of the history sync affordance
- severity: Low — functional behavior is unaffected; only icon aesthetics are wrong

## 6. Root Cause

- hypothesis: `SyncGlyph` SVG was hand-authored with a single-arc + corner-polyline pattern identical to a reload icon.
- confirmed cause: The SVG path `M10.2 5.2A4.2 4.2 0 1 0 10.4 7.4` draws a large (flag=1) counter-clockwise arc covering nearly the full circle. The polyline `10.2,1.6 10.2,5.2 6.7,5.2` forms an L-shaped arrowhead at the top-right, which is the reload icon pattern. There was no second opposing arc.
- evidence: Direct SVG inspection of `Navigator.tsx:36–43`.

## 7. Fix Strategy

- `F-1` Replace the single-arc SVG in `SyncGlyph` with two opposing arcs:
  - Upper arc: `M2.2 7.5 A4.5 4.5 0 0 1 9.5 3.8` (CW, SW→NE via top) with V-arrowhead at `(9.5, 3.8)`
  - Lower arc: `M9.8 4.5 A4.5 4.5 0 0 1 2.5 8.2` (CW, NE→SW via bottom) with V-arrowhead at `(2.5, 8.2)`
  - Same `width="11" height="11" viewBox="0 0 12 12"` and `currentColor` stroke — no size or color regression

## 8. Validation

- `V-1` TypeScript typecheck (`npx tsc --noEmit`) passes with zero errors after the change.
- `V-2` Visual inspection: the icon renders as two opposing curved arrows rather than a reload circle.

## 9. Regression Guard

- tests: No automated test for SVG paths; visual regression is low-risk (purely presentational, single component, no logic).
- alerts: —
- audit checks: `SyncGlyph` has no callers outside `Navigator.tsx` (GitNexus confirmed single-component scope).

## 10. Follow-Up Document Updates

- upstream docs that must change: None — this is a cosmetic icon correction, not a design rule change.
- notes left unchanged on purpose: The component name `SyncGlyph` is correct and unchanged.
