# Task-416: Runnerboot Decoupling And Stale Replacement (CP-81 P-3)

## Metadata

- Document ID: `Task-416`
- Title: `Runnerboot Decoupling And Stale Replacement`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-22`
- Last Updated: `2026-09-22`
- Parent Documents: [CP-81](../../07-Coding-Plan/done/CP-81-Shared-Runner-Lifecycle.md) `P-3`, [SD-28](../../06-System-Tech-Design/SD-28-Shared-Runner-Lifecycle.md) `D-6`, `D-8`, §6.4
- Child Documents: `None`
- Related Documents: [Task-415](./Task-415-Runner-Lifecycle-API-And-Durable-Stop-All.md), `BUG-240`, `BUG-328`, `CA-474`, `CA-901`
- Replaces: `CA-474 process-close contract`
- Tags: `runner-lifecycle, runnerboot, process, windows, job-object, build-id, go`
- Feature Keys: `runner-lifecycle`

## AI Quick View

### Summary

- Decouple TUI process lifetime from runner lifetime: remove `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` and equivalent process-group coupling for shared runner mode.
- Add machine-scoped boot serialization, executable build identity, lifecycle-aware reuse classification, and idle-only stale replacement.
- Keep direct process kill only as a fenced fallback after the runner accepts shutdown for the same `runnerInstanceId`.

### Current Ask

- Make `EnsureRunner` safe for a shared daemon: reuse compatible runners, auto-replace only idle stale runners, never kill unknown listeners, and never let TUI death kill a runner needed by Desktop.

### Key Decisions

- `T-1` `runnerboot` spawns `runner serve --lifecycle-mode client-managed` detached enough to survive client process exit.
- `T-2` Build identity is computed from the current executable artifact (`debug.ReadBuildInfo` + executable SHA-256 fallback) so `go run` `dev` builds are distinguishable.
- `T-3` Reuse classification is `compatible`, `idle_stale`, `busy_stale`, `protocol_incompatible`, `legacy_unknown`, `workspace_mismatch`, `port_conflict`.
- `T-4` Stale replacement is fenced by `expectedInstanceId`; after accepted shutdown, wait for old instance disappearance before starting the expected binary.
- `T-5` Concurrent starters serialize on a boot lock under `.flowpilot/` or OS named mutex.

### Constraints

- Preserve BUG-240: explicit stack shutdown must still kill both `go.exe` wrapper and compiled child.
- Do not kill an unknown process merely because it occupies the runner port.
- Do not auto-replace a busy or legacy-unknown runner.
- Windows and Unix spawn semantics must both be covered; platform code stays behind existing `runnerboot` abstractions.

### Open Questions

- `Q-1` Resolved — `client-managed` planned restart uses a detached successor only if it can wait for the old instance to release the port; otherwise return `restart_unavailable` instead of destructive fallback.

### Source Refs

- `SS-24 AC-7`, `AC-11`, `AC-12`, `E-11..E-14`, `E-20`.
- `SD-28 D-6`, `D-8`, §6.4 process ownership, §7 stale replacement.
- Current code: `internal/tui/runnerboot/runnerboot.go`, `sysprocattr_windows.go`, `sysprocattr_unix.go`, `killRunnerByURL`, `internal/cli/chat.go`, `internal/tui/config/config.go`.

## 1. Goal

Make runner spawn/reuse/replacement lifecycle-aware and independent of any single client process while preserving safe explicit cleanup.

## 2. Parent Links

- coding plan: `CP-81 P-3`
- tech design: `SD-28 D-6`, `D-8`, §6.4, §7 stale replacement
- system spec: `SS-24 AC-7`, `AC-11`, `AC-12`, `E-11..E-14`, `E-20`

## 3. Trigger

`runnerboot` currently binds a spawned Windows runner to the TUI Job Object with `KILL_ON_JOB_CLOSE`, so closing the TUI kills the shared runner even if Desktop still needs it. At the same time, `ExpectedVersion` is not enough to detect stale `go run` binaries, causing old builds to be reused.

## 4. Exact Change

- `T-1` Process decoupling:
  - remove/avoid `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` for shared runner mode;
  - use platform-appropriate independent process group/session spawn;
  - rely on lease expiry + idle shutdown for orphan prevention.
- `T-2` Spawn args/env:
  - pass `--lifecycle-mode client-managed` when TUI/Desktop launches a runner;
  - preserve existing workspace/cwd propagation and log paths.
- `T-3` Build identity:
  - implement `CurrentBuildID()` using `debug.ReadBuildInfo` and executable SHA-256 fallback;
  - pass expected protocol/build into `EnsureRunner`.
- `T-4` Reuse classification:
  - health/lifecycle fields determine `compatible`, `idle_stale`, `busy_stale`, `protocol_incompatible`, `legacy_unknown`, `workspace_mismatch`, `port_conflict`;
  - `ExplicitURL` remains an explicit attach path but still reports stale/mismatch status to UX.
- `T-5` Idle stale replacement:
  - call fenced `/system/shutdown` with `reason=stale_build` only when snapshot proves zero clients/work;
  - wait for old `runnerInstanceId` to disappear;
  - spawn current binary;
  - poll until compatible health.
- `T-6` Boot lock:
  - serialize concurrent EnsureRunner calls on this machine so TUI/Desktop do not double-spawn;
  - stale lock recovery bounded and logged.
- `T-7` Fenced kill fallback:
  - direct port/PID kill is only allowed after accepted shutdown and only if live health still reports the expected `runnerInstanceId`.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/tui/runnerboot/runnerboot.go`
  - `apps/local-runner/internal/tui/runnerboot/sysprocattr_windows.go`
  - `apps/local-runner/internal/tui/runnerboot/sysprocattr_unix.go`
  - `apps/local-runner/internal/tui/runnerboot/build_identity.go` (new)
  - `apps/local-runner/internal/tui/runnerboot/boot_lock.go` (new)
  - `apps/local-runner/internal/cli/chat.go`
  - `apps/local-runner/internal/tui/config/config.go`
- modules: TUI runner boot, CLI chat bootstrap.
- routes: uses `/health`, `/system/lifecycle`, `/system/shutdown`.
- tables: none.

## 6. Code Guide Signatures

```go
// runnerboot/build_identity.go
type BuildIdentity struct {
    ProtocolVersion int
    BuildID         string
    Version         string
}
func CurrentBuildIdentity() (BuildIdentity, error)

// runnerboot/boot_lock.go
type BootLock interface {
    Unlock() error
}
func AcquireRunnerBootLock(ctx context.Context) (BootLock, error)

// runnerboot/runnerboot.go
type RunnerClassification string
const (
    ClassCompatible            RunnerClassification = "compatible"
    ClassIdleStale             RunnerClassification = "idle_stale"
    ClassBusyStale             RunnerClassification = "busy_stale"
    ClassProtocolIncompatible  RunnerClassification = "protocol_incompatible"
    ClassLegacyUnknown         RunnerClassification = "legacy_unknown"
    ClassWorkspaceMismatch     RunnerClassification = "workspace_mismatch"
    ClassPortConflict          RunnerClassification = "port_conflict"
)

type EnsureResult struct {
    Reused         bool
    Launched       bool
    URL            string
    OwnsRunner     bool
    Classification RunnerClassification
    Snapshot       client.LifecycleSnapshot // additive DTO
    UpdatePending  bool
}
```

## 7. Test Signatures

- `TestCurrentBuildIdentity_DeterministicForExecutable`
- `TestEnsureRunner_CompatibleReuses`
- `TestEnsureRunner_IdleStaleAutoReplaces`
- `TestEnsureRunner_BusyStaleDoesNotReplace`
- `TestEnsureRunner_ProtocolIncompatibleRejected`
- `TestEnsureRunner_LegacyUnknownRequiresConfirmation`
- `TestEnsureRunner_WorkspaceMismatchRejected`
- `TestEnsureRunner_UnknownPortProcessNotKilled`
- `TestEnsureRunner_ConcurrentStartsSingleWinner`
- `TestRunnerBoot_SpawnPassesClientManagedMode`
- `TestRunnerBoot_TUIDeathDoesNotKillSharedRunner`
- `TestKillRunnerFallback_RequiresExpectedInstanceID`
- `TestRunnerBoot_StaleLockRecoveredBounded`

## 8. Acceptance Check

- `go test -count=1 ./internal/tui/runnerboot/... ./internal/cli/...` green.
- Windows process test proves TUI death does not kill a runner while another lease exists.
- Windows supervisor tree-kill contract remains available for explicit stack shutdown.
- GitNexus impact/detect evidence recorded, or unavailable attempt documented.

## 9. Out of Scope

- TUI lease heartbeat/close dialog (`Task-417`).
- Desktop/Electron lifecycle (`Task-418`).
- Supervisor command fencing (`Task-419`).

## 10. Definition of Done

- [x] §6 signatures landed.
- [x] §7 tests exist and pass.
- [x] TUI death does not kill shared runner; last-client expiry still cleans up.
- [x] Stale runner auto-replaces only when lifecycle proves idle.
- [x] Unknown port process is never killed.
- [x] `feature_key: runner-lifecycle`; CA note written.
