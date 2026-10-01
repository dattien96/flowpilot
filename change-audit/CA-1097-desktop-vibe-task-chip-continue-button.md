# CA-1097: desktop showed no active-Task indicator and labeled continuation "Retry"

Date: 2026-10-01
Refs: live production run-3362 (vibe-tasks, PrivateVault) — a 5-Task CP run
gave no hint which task the sprint was on, and each sprint boundary showed a
card whose action button read "Retry" (clicking it did continue — the label
was wrong).

## Fix (`apps/desktop-flowpilot`)

- `FlowTimelineSidebar`: new `Task i/N — name` chip in the summary header,
  driven by `loopState.vibeTaskIndex/vibeTaskTotal/vibeTaskName` (already in
  the `AgentLoopState` contract and serialized by the runner) — mirrors the
  TUI `vibeTaskChip`. Rendered only while a multi-task vibe run is live;
  styling via new `.flow-sidebar-task` (accent chip, ellipsizes long names).
- `FlowAwaitingUserCard`: `blockReason === "vibe_sprint_boundary"` now renders
  badge "Sprint done", a primary **Continue** button ("start the next task"),
  and boundary-specific detail text — matching the TUI's
  `[Continue] - start the next sprint`. `awaitingUserDriftState` exposes
  `isSprintBoundary` so the blockReason interpretation stays in one helper.

## Verification

- `flowAwaitingUserDrift.test.ts`: new case asserts `isSprintBoundary` for
  `vibe_sprint_boundary` and that it is neither stalled nor cap.
- `npm run typecheck` clean; node --test on the drift suite green.
- Note: after CA-1093 the boundary card is the exceptional path (normal
  boundaries auto-advance), so Continue is the rare manual override.
