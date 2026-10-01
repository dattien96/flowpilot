# CA-1093: sprint boundary parked for user approval — next task required a manual click

Date: 2026-10-01
Refs: live production run-3362 (vibe-tasks, PrivateVault) — after each of the
5 Tasks the loop blocked on a `vibe_sprint_boundary` card; the operator had to
press the (mislabeled, see CA-1097) button to start the next sprint.

## Requirement

Vibe flows may gate the user only for (a) CP lock and (b) owner-debate cap
exhaustion. A normal sprint boundary is a checkpoint, not a gate.

## Root cause

`runAuditNode`'s settle paths called `maybeParkVibeSprintBoundary`, which
parks the loop `blocked`/`vibe_sprint_boundary` whenever plan tasks remain —
even though `maybeStartNextVibeSprint`/`continueVibeSprintBoundary` already
implement automatic sprint start with full race/rollback protection.

## Fix (`internal/runner/vibe_sprint_boundary.go`)

- New `maybeAutoAdvanceVibeSprintBoundary`: performs the same ownership
  checks as the park path, then hands off to `continueVibeSprintBoundary`
  (the proven auto-start seam) instead of parking. On a CP-lock re-arm race
  it clears the stale pending boundary flag so `cp_lock` owns resumption.
- `runAuditNode` (flow_validate_audit_dispatch.go): both settle paths
  (ready draft → done, and missing-feature-key auto-finalize) now call the
  auto-advance; the park only remains for the exceptional reopen-offer
  shape.
- `maybeReparkVibeSprintBoundary`: reopen recovery auto-advances too; the
  decline marker still suppresses auto-continuation where required.
- Fixed a self-deadlock: the auto-advance emits `emitSprintHandoffAt` after
  releasing `s.mu` (the helper locks internally).

## Verification

- `vibe_sprint_boundary_test.go`: updated superseded pins (audit auto-finalize
  and RunningReopenReDerives now assert auto-advance, not park) + added
  CA-1093 assertions.
- Full vibe/boundary suite green; no test weakened beyond the behavior change
  this CA documents.
