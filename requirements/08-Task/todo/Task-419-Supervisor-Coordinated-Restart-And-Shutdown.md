# Task-419: Supervisor Coordinated Restart And Shutdown (CP-81 P-6)

## Metadata

- Document ID: `Task-419`
- Title: `Supervisor Coordinated Restart And Shutdown`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-22`
- Last Updated: `2026-09-22`
- Parent Documents: [CP-81](../../07-Coding-Plan/todo/CP-81-Shared-Runner-Lifecycle.md) `P-6`, [SD-28](../../06-System-Tech-Design/SD-28-Shared-Runner-Lifecycle.md) `D-3`, `D-5`, §6.5
- Child Documents: `None`
- Related Documents: [Task-415](./Task-415-Runner-Lifecycle-API-And-Durable-Stop-All.md), [Task-416](./Task-416-Runnerboot-Decoupling-And-Stale-Replacement.md), `BUG-240`, `CA-240`, `CA-901`
- Replaces: `None`
- Tags: `runner-lifecycle, supervisor, restart, shutdown, windows, process-tree`
- Feature Keys: `runner-lifecycle`

## AI Quick View

### Summary

- Make `scripts/supervisor.js` a lifecycle-aware controller instead of a competing runner owner.
- Runner launched under supervision uses `lifecycleMode=supervised`; supervisor does not hold a permanent client lease.
- Planned restart uses a fenced command record carrying `runnerInstanceId` + `restartId`; unexpected runner exit does not silently respawn behind clients.
- Preserve the Windows tree-kill fix so `go.exe` wrappers and compiled runner binaries are both cleaned up on explicit stack shutdown.

### Current Ask

- Coordinate supervisor restart/shutdown with the new runner lifecycle while preserving the existing dev-stack UX.

### Key Decisions

- `T-1` Supervisor launches runner with `FLOWPILOT_RUNNER_LIFECYCLE_MODE=supervised` or equivalent flag.
- `T-2` Supervisor writes/reads fenced command records; stale commands for another `runnerInstanceId` are ignored and removed.
- `T-3` Planned runner restart restarts only the runner dependency chain required by the supervisor; Desktop/web clients remain in reconnect mode.
- `T-4` Explicit stack shutdown still performs full tree kill; unexpected runner exit logs and applies the frozen no-ghost-restart policy.
- `T-5` Supervisor clears stale `.flowpilot/supervisor.cmd`/control records at startup.

### Constraints

- Supervisor must not count as a user client or block idle shutdown forever.
- Do not regress `BUG-240`: Windows shutdown must kill wrapper + compiled child.
- Commands must be fenced so an old command cannot kill a new runner generation.
- No silent respawn after unplanned runner death unless the contract explicitly marks it planned.

### Open Questions

- `Q-1` Resolved — supervisor gets controller identity via env/`X-Client: supervisor`, not a user lease.

### Source Refs

- `SS-24 AC-8..AC-10`, `AC-13..AC-15`, `E-6..E-10`, `E-20`.
- `SD-28 D-3`, `D-5`, §6.5 supervisor contract, §8 F-8/F-9.
- Current code: `scripts/supervisor.js`, `internal/cli/root.go` supervisor command write/read paths.

## 1. Goal

Unify supervisor process control with the lease-based runner lifecycle so dev-stack start/stop/restart is safe, fenced, and orphan-free.

## 2. Parent Links

- coding plan: `CP-81 P-6`
- tech design: `SD-28 D-3`, `D-5`, §6.5
- system spec: `SS-24 AC-8..AC-10`, `AC-13..AC-15`, `E-6..E-10`, `E-20`
- previous slices: `Task-415`, `Task-416`

## 3. Trigger

Supervisor currently watches runner process exit and can kill/restart the dev stack independently. Without fencing, a stale restart/shutdown command from an old runner can affect a new runner, and an unexpected runner exit can be misread as planned restart.

## 4. Exact Change

- `T-1` Launch mode:
  - runner child gets `FLOWPILOT_RUNNER_LIFECYCLE_MODE=supervised` and supervisor metadata (`SUPERVISOR_PID`, boot grace).
  - supervisor remains a controller, not a user lease.
- `T-2` Fenced commands:
  - command payload includes `action`, `runnerInstanceId`, `restartId`, `requestedAt`, `expiresAt`, `requester`;
  - supervisor validates PID/instance before acting;
  - stale/mismatched commands are ignored and removed.
- `T-3` Planned restart:
  - runner enters `draining_restart` and writes fenced restart command;
  - supervisor waits for old runner exit, starts replacement runner, preserves web/Desktop processes;
  - clients reconnect to new generation.
- `T-4` Full shutdown:
  - explicit `Turn off FlowPilot`/stack stop kills runner through lifecycle endpoint, then performs existing supervisor cleanup;
  - Windows keeps `killProcessTree` behavior for `go.exe` wrapper and compiled child.
- `T-5` Unexpected runner exit:
  - supervisor logs runner exit reason and does not silently start a hidden new generation;
  - if clients should close, lifecycle/unplanned-loss semantics do so; supervisor exits or stops stack according to frozen policy.
- `T-6` Startup hygiene:
  - clear stale supervisor command files and state that reference dead `runnerInstanceId`s;
  - log PID, child PID, action, and final exit outcome.

## 5. Touched Areas

- files:
  - `scripts/supervisor.js`
  - supervisor command/state file handling under `.flowpilot/`
  - `internal/cli/root.go` restart command write path if needed
  - supervisor test harness/scripts if present
- modules: dev-stack supervisor, runner process handoff.
- routes: uses lifecycle endpoints and supervisor control files.
- tables: none.

## 6. Code Guide Signatures

```js
// scripts/supervisor.js
function startRunnerProcess() {}
function readSupervisorCommand() {}
function validateSupervisorCommand(cmd, currentRunner) {}
function handlePlannedRunnerRestart(cmd) {}
function handleRunnerExitUnexpected(code, signal) {}
function shutdownRunnerTree(child) {}
function clearStaleSupervisorCommand() {}
```

```json
// fenced command record
{
  "action": "restart-runner",
  "runnerInstanceId": "instance_...",
  "restartId": "restart_...",
  "requestedAt": "...",
  "expiresAt": "...",
  "requester": "runner"
}
```

## 7. Test Signatures

- `TestSupervisor_StartsRunnerInSupervisedMode`
- `TestSupervisor_DoesNotRegisterPermanentUserLease`
- `TestSupervisor_RunnerPlannedRestartRestartsOnlyRunner`
- `TestSupervisor_UnexpectedRunnerExitDoesNotGhostRestart`
- `TestSupervisor_IgnoresStaleCommandForOldInstance`
- `TestSupervisor_ForceShutdownStopsRunnerWebDesktop`
- `TestSupervisor_WindowsTreeKillIncludesCompiledRunner`
- `TestSupervisor_TerminalCloseLeavesNoOrphans`
- `TestSupervisor_CommandExpiryIgnored`

## 8. Acceptance Check

- Supervisor tests/integration checks pass.
- Manual CP-81 section H passes.
- `netstat`/process inspection shows no orphaned compiled runner after shutdown/restart.
- GitNexus impact/detect evidence recorded, or unavailable attempt documented.

## 9. Out of Scope

- Lease manager internals (`Task-414`).
- TUI/Desktop close dialogs (`Task-417`, `Task-418`).
- Stale binary replacement logic beyond supervisor handoff (`Task-416`).

## 10. Definition of Done

- [x] §6 signatures landed.
- [x] §7 tests exist and pass.
- [x] Supervisor is a controller, not a lease holder.
- [x] Planned restart and unexpected exit are distinguishable.
- [x] Windows wrapper/compiled-child cleanup remains intact.
- [x] `feature_key: runner-lifecycle`; CA note written.
