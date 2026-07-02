# BUG-159: Flow Sidebar Progress Count And Yolo Visibility

## Metadata

- Document ID: `BUG-159`
- Title: `Flow Sidebar Progress Count And Yolo Visibility`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-02`
- Last Updated: `2026-07-02`
- Parent Documents: `requirements/09-BugFix/done/BUG-158-Flow-Timeline-Missing-Step-Name-Model-Agent-Flow-Yolo.md`
- Child Documents: `none`
- Related Documents: `none`
- Replaces: `none`
- Tags: `agent-flow-engine, ui, desktop, workflow-steps-runtime`

## AI Quick View

### Summary

- After `BUG-158` shipped and the runner was rebuilt, a live run confirmed the step names, provider badges ("CODEX"), and connected-circle icons all render correctly.
- Two things were still off: the progress counter read "0/4 steps" while standing on step 1 (which was `RUNNING`), because the counter only counted `DONE` steps; and the flow-level yolo state never rendered when yolo was off, because the badge was conditionally rendered only when `meta.yoloMode` was `true` — so "off" looked identical to "not shown at all."

### Current Ask

- Progress counter should read "1/4" while on step 1, not "0/4" — count the step you're currently standing on as reached.
- Always show the flow's yolo posture (on or off), not only when it happens to be on.

### Key Decisions

- `F-1` Progress numerator counts steps whose status is `DONE`, `RUNNING`, or `WAITING_USER_APPROVAL` ("reached"), not just `DONE` ("finished") — matches how a user reads "N/4 steps" while standing on step N.
- `F-2` The `YOLO` badge always renders in the expanded summary header, with explicit `ON`/`OFF` text and a distinct muted style (`.yolo-off`) for the off state, instead of disappearing entirely when off.

### Constraints

- No change to the underlying `WorkflowStepRuntimeStatus` semantics or the run-level `yoloMode` data path from `BUG-158` — this is a pure display-logic fix in `FlowTimelineSidebar`.

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/src/components/FlowTimelineSidebar.tsx` (`reachedCount`, always-rendered `YOLO ON`/`YOLO OFF` badge)
- `apps/desktop-flowpilot/src/styles.css` (`.wsr-retry-badge.yolo-off`)

## 1. Issue Summary

Live-verified after `BUG-158`: the sidebar's "N/4 steps" counter undercounted by one while a step was actively running, and the yolo badge was invisible whenever yolo was off (indistinguishable from "no yolo data at all").

## 2. Parent Links

- impacted coding plan: `none`
- impacted tech design: `none`
- impacted system spec: `none`

## 3. Environment and Reproduction

- environment: desktop-flowpilot, a running Flow Mode run.
- reproduction steps: start a Flow Mode run; while step 1 is `RUNNING`, observe the header reads "0/4 steps"; with yolo off, observe no yolo indicator anywhere in the expanded header.
- frequency: deterministic.

## 4. Expected vs Actual

- expected: "1/4 steps" while standing on step 1; a "YOLO OFF" (or "YOLO ON") badge always visible.
- actual: "0/4 steps" while on step 1 (undercounts the in-progress step); no yolo indicator when off.

## 5. Impact

- users affected: anyone running Flow Mode.
- workflows affected: sidebar display only.
- severity: low — display-only, same class as `BUG-155`/`BUG-156`/`BUG-158`.

## 6. Root Cause

- confirmed cause: `doneCount` in `FlowTimelineSidebar` was `steps.filter((s) => s.status === "DONE").length` — a step that is actively `RUNNING` (or `WAITING_USER_APPROVAL`) was not counted, undercounting progress by exactly the in-progress step. Separately, the yolo badge was gated behind `meta.yoloMode &&`, so `false` rendered nothing rather than an explicit "off" state.
- evidence: `apps/desktop-flowpilot/src/components/FlowTimelineSidebar.tsx` (pre-fix `doneCount` computation and conditional badge).

## 7. Fix Strategy

- `F-1` Renamed `doneCount` to `reachedCount` and extended the filter to `DONE || RUNNING || WAITING_USER_APPROVAL`.
- `F-2` Moved the yolo badge into the always-rendered `.flow-sidebar-meta` row (alongside provider/model, which already render conditionally on presence, not on truthiness of a boolean), with text `YOLO ON` / `YOLO OFF` and a `.yolo-off` CSS class for a visually muted off-state so it doesn't compete with the on-state's amber emphasis.

## 8. Validation

- `V-1` `npm run typecheck` in `apps/desktop-flowpilot` — clean.
- `V-2` Not executed: a further live re-check of the corrected counter/badge, since this fix was authored directly from the user's live screenshot feedback and no backend/Supabase instance is available in this environment to re-verify interactively. The user should confirm "1/4" and the always-visible `YOLO ON`/`OFF` badge on next use.

## 9. Regression Guard

- tests: none added — this is a small, purely presentational arithmetic/conditional fix in a component with no existing render-test harness (same constraint noted in `BUG-156`/`BUG-158`).
- alerts: none.
- audit checks: recorded in `change-audit/CA-196-flow-sidebar-progress-and-yolo-visibility.md`.

## 10. Follow-Up Document Updates

- upstream docs that must change: none.
- notes left unchanged on purpose: none.
