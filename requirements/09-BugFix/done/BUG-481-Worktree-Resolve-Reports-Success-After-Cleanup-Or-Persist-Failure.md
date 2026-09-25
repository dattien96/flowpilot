# BUG-481: worktree resolve reports terminal success after cleanup or state persistence failure

## Metadata

- Document ID: `BUG-481`
- Phase: `bugfix`
- Status: `done`
- Severity: `high`
- Evidence: `code-confirmed; fault-injection tests green (CA-972)`
- Feature Keys: `run-worktree`
- Parent Documents: `CP-71`, `SS-23`, `SD-27`
- Related Documents: `BUG-472`, `BUG-473`
- Affected Area: `internal/runner/run_worktree_merge.go`

## Summary

All three normal resolution modes ignore `Manager.Cleanup` errors and then
stamp/report a terminal worktree state. State persistence is also best-effort:
`persistRunWorktreeLocked` ignores read/write failures and returns no result to
the caller. The API can therefore return `merged`, `kept_branch`, or
`discarded` while the filesystem and durable session still describe an active
or merge-pending binding.

## Evidence

- `apply_patch`: cleanup and patch-artifact removal errors are ignored before
  `setWorktreeState("merged")`.
- `keep_branch`: cleanup error is ignored before `kept_branch` response.
- `discard`: cleanup error is ignored before `discarded` response.
- `persistRunWorktreeLocked` returns silently on session read failure and
  discards `persistProviderSession` errors.
- There are no fault-injection tests for cleanup or persistence failure.

## Expected vs Actual

- Expected: response state is committed only when required filesystem and
  durable-state effects reach a defined recoverable outcome.
- Actual: partial failure is presented as success; restart may resurrect stale
  merge state or leave orphaned worktrees/branches.

## Impact

This creates split-brain between API/UI, Git worktree registration, filesystem
and session state. A retry can repeat effects, while GC may later classify the
leftover binding incorrectly.

## Required Fix Contract

1. Define records-first or filesystem-first resolution phases with durable,
   replayable intermediate state.
2. Propagate cleanup/persist failures as typed non-success outcomes.
3. Make retry idempotent after each partial-effect boundary.
4. Never terminalize the binding solely in RAM.

## Required Tests

- RED fault injection for cleanup failure in all three modes.
- RED persistence failure after cleanup with restart/replay recovery.
- Duplicate resolution requests are idempotent.
- E2E verifies API state, session state, branch list and worktree list agree.

## Implementation Plan

### P-1 — Enumerate effect barriers

- Write RED tests for failure after patch apply, during cleanup, during sidecar
  removal and during session persistence for each resolution mode.
- Capture expected post-failure filesystem, branch, binding and API state at
  every barrier.
- Use narrow injectable filesystem/Git/session seams; do not weaken real-Git
  happy-path tests.

### P-2 — Durable resolution phases

- Add a versioned worktree resolution intent/result to the existing session
  binding rather than a parallel store. Suggested phases:
  `resolution_requested → repository_effect_committed → cleanup_committed → finalized`.
- Persist requested mode/resolution ID before external effects. Each phase
  transition must be durable and monotonic.
- Resume/retry reads the phase and executes only missing idempotent effects.

### P-3 — Error propagation and response

- Make `persistRunWorktreeLocked` return errors; callers must not report a
  terminal state when persistence fails.
- Check cleanup, patch-artifact removal and branch operations. Return typed
  retryable status with the durable phase and resolution ID.
- Emit `worktree_resolved` only after finalization; intermediate failures remain
  visible as merge/repair attention.

### P-4 — Recovery

- On restart, detect non-final resolution intents and reconcile filesystem/Git
  state before accepting a new resolve request or GC.
- Replayed same resolution ID returns committed outcome; conflicting action is
  rejected.

## Definition of Done

- [ ] Every external-effect boundary has a RED crash/failure test.
- [ ] Resolution intent and phases survive kill/restart.
- [ ] API success is returned only after durable finalization.
- [ ] Cleanup/session errors remain retryable and operator-visible.
- [ ] Replaying one resolution is idempotent; conflicting mode is rejected.
- [ ] Filesystem, Git registration, branch and session binding converge.
- [ ] BUG-472/473 fail-closed behavior composes with this state machine.
- [ ] CP-71 full HTTP/live matrix passes for all resolve modes.
