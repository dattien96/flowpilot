# Task-417: TUI Lease Client, Close UX, And Reconnect (CP-81 P-4)

## Metadata

- Document ID: `Task-417`
- Title: `TUI Lease Client, Close UX, And Reconnect`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-22`
- Last Updated: `2026-09-22`
- Parent Documents: [CP-81](../../07-Coding-Plan/todo/CP-81-Shared-Runner-Lifecycle.md) `P-4`, [SD-28](../../06-System-Tech-Design/SD-28-Shared-Runner-Lifecycle.md) `D-2`, `D-5`, §6.3
- Child Documents: `None`
- Related Documents: [Task-416](./Task-416-Runnerboot-Decoupling-And-Stale-Replacement.md), `CP-56`, `BUG-328`, `CA-445`, `CA-474`, `CA-910`
- Replaces: `CA-474 TUI-exit shutdown contract`
- Tags: `runner-lifecycle, tui, bubble-tea, heartbeat, close-dialog, reconnect`
- Feature Keys: `runner-lifecycle`

## AI Quick View

### Summary

- TUI registers a lease after `EnsureRunner`, heartbeats every 5 seconds, and releases on graceful close.
- `/exit`, Ctrl+C quit, and terminal close become client-release paths — never unconditional `/system/shutdown`.
- When another client or active work exists, TUI shows `Cancel`, `Close TUI only`, `Turn off FlowPilot` with the runner's live inventory.
- Planned restart enters reconnect mode; unplanned runner loss shows a clear notice and closes the TUI.

### Current Ask

- Update the TUI from a runner owner to a lifecycle participant while preserving normal chat behavior and existing quit ergonomics.

### Key Decisions

- `T-1` TUI lease identity lives in `internal/tui/client`; the Bubble Tea model owns heartbeat/release commands.
- `T-2` `cmdShutdownAndQuit` is replaced by an exit-intent flow that first loads `/system/lifecycle`.
- `T-3` `killRunnerByURL` is removed from ordinary close and may only be used as a fenced post-shutdown fallback by `Task-416` semantics.
- `T-4` `Ctrl+C` keeps its current turn-interrupt behavior; when it reaches quit intent, lifecycle policy decides whether a dialog is needed.
- `T-5` Heartbeat failure distinguishes last-seen `draining_restart` from unplanned transport loss.

### Constraints

- Do not block Bubble Tea update loop on network calls; lifecycle calls use commands/messages.
- Runner status must stay readable in narrow terminals; three-choice dialog is compact and keyboard-accessible.
- Existing busy/scaffold indicators (CA-910) remain intact.
- CA-474 tests may only change under documented contract supersession; new behavior must be stronger for shared-client safety.

### Open Questions

- `Q-1` Resolved — TUI label is `TUI <pid>` and dialog copy is `Close TUI only`.

### Source Refs

- `SS-24 AC-1..AC-10`, `AC-14`, `E-1..E-10`, `E-15..E-19`.
- `SD-28 D-2`, `D-5`, §6.3 client behavior.
- Current code: `internal/tui/client/client.go`, `internal/tui/app/app.go`, `internal/tui/app/model.go`, `internal/tui/app/quit_kills_reused_runner_test.go`.

## 1. Goal

Turn the TUI into a lease participant: shared-close-safe, crash-recoverable by TTL, restart-aware, and explicit about global shutdown.

## 2. Parent Links

- coding plan: `CP-81 P-4`
- tech design: `SD-28 D-2`, `D-5`, §6.3
- system spec: `SS-24 AC-1..AC-10`, `AC-14`, `E-1..E-10`, `E-15..E-19`
- previous slice: `Task-416`

## 3. Trigger

TUI currently posts `/system/shutdown` on quit and can kill a runner used by Desktop. Conversely, if shutdown is skipped, a crashed TUI could leave the runner alive indefinitely without a lease/TTL model.

## 4. Exact Change

- `T-1` Extend `internal/tui/client` with lifecycle methods:
  - `RegisterLifecycleClient`
  - `HeartbeatLifecycleClient`
  - `ReleaseLifecycleClient`
  - `GetLifecycleSnapshot`
  - `RequestRunnerShutdown`
  - `RequestRunnerRestart`
- `T-2` `ChatConfig` / app state stores `leaseId`, token, `runnerInstanceId`, generation, heartbeat interval, reconnect deadline.
- `T-3` Start heartbeat command after runner attach; refresh on response; surface lifecycle phase/status.
- `T-4` Replace quit paths:
  - fetch lifecycle snapshot;
  - only self + no work → release + quit;
  - shared/busy → three-choice dialog;
  - `Close TUI only` → release + quit;
  - `Turn off FlowPilot` → confirmed fenced shutdown + quit after accepted.
- `T-5` Runner status text adds compact lifecycle info: `shared: TUI+Desktop`, `idle shutdown in Ns`, `update pending`, `runner restarting…`.
- `T-6` Reconnect handling:
  - heartbeat sees `draining_restart` → store restart ID/deadline, stop ordinary requests, poll `/health`;
  - new `runnerInstanceId` → re-register and restore durable chat/run state;
  - deadline → notice + quit.
- `T-7` Unplanned runner loss:
  - heartbeat/health failure without restart marker → show `Runner stopped unexpectedly; TUI will close` and quit.
- `T-8` Update stale contract tests:
  - old `quit_kills_reused_runner` assumptions are superseded by CP-81;
  - replacement tests assert no shutdown on client-only close.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/tui/client/client.go`
  - `apps/local-runner/internal/tui/app/app.go`
  - `apps/local-runner/internal/tui/app/model.go`
  - `apps/local-runner/internal/tui/app/*lifecycle*_test.go` (new)
  - `apps/local-runner/internal/tui/app/quit_kills_reused_runner_test.go` (superseded contract update)
  - `apps/local-runner/internal/tui/config/config.go`
- modules: TUI HTTP client, Bubble Tea model/update loop, close UX/statusline.
- routes: consumes `/system/lifecycle`, `/system/clients/*`, `/system/shutdown`, `/system/restart`, `/health`.
- tables: none.

## 6. Code Guide Signatures

```go
// internal/tui/client/client.go
func (c *Client) RegisterLifecycleClient(ctx context.Context, in RegisterLifecycleRequest) (RegisterLifecycleResult, error)
func (c *Client) HeartbeatLifecycleClient(ctx context.Context, leaseID, token string) (LifecycleSnapshot, error)
func (c *Client) ReleaseLifecycleClient(ctx context.Context, leaseID, token string) error
func (c *Client) GetLifecycleSnapshot(ctx context.Context) (LifecycleSnapshot, error)
func (c *Client) RequestRunnerShutdown(ctx context.Context, in SystemActionRequest) (SystemActionResult, error)
func (c *Client) RequestRunnerRestart(ctx context.Context, in SystemActionRequest) (SystemActionResult, error)

// internal/tui/app model flow
type LifecycleSnapshotMsg struct{ Snapshot client.LifecycleSnapshot }
type LifecycleHeartbeatTickMsg struct{}
type LifecycleLostMsg struct{ Reason string; Planned bool }
type ExitIntentMsg struct{ Source string }
type CloseChoiceMsg struct{ Choice CloseChoice }
```

## 7. Test Signatures

- `TestTUI_RegisterLeaseAfterRunnerReady`
- `TestTUI_HeartbeatKeepsLeaseAlive`
- `TestTUI_HeartbeatFailurePlannedRestartReconnects`
- `TestTUI_HeartbeatFailureUnplannedLossShowsNoticeAndQuits`
- `TestTUI_ExitAloneReleasesLeaseWithoutShutdown`
- `TestTUI_ExitWithDesktopShowsThreeChoices`
- `TestTUI_CloseThisClientOnlyLeavesRunner`
- `TestTUI_CancelExitKeepsTUIAlive`
- `TestTUI_TurnOffFlowPilotSendsConfirmedForce`
- `TestTUI_LastClientExitStartsIdleGrace`
- `TestTUI_RunnerRestartReattachReRegistersLease`
- `TestTUI_ReconnectDeadlineExpiresCloses`
- `TestTUI_StatusShowsSharedClientsAndUpdatePending`
- `TestTUI_ExitDoesNotCallKillRunnerByURL`

## 8. Acceptance Check

- `go test -count=1 ./internal/tui/...` green.
- Manual CP-81 sections A, B, C, E pass on Windows.
- Old `/exit` path no longer posts shutdown on ordinary close.
- GitNexus impact/detect evidence recorded, or unavailable attempt documented.

## 9. Out of Scope

- Runner lifecycle server internals (`Task-415`).
- Spawn/job/build replacement details (`Task-416`).
- Desktop/Electron UX (`Task-418`).
- Supervisor changes (`Task-419`).

## 10. Definition of Done

- [x] §6 signatures landed.
- [x] §7 tests exist and pass.
- [x] Ordinary TUI close releases lease only.
- [x] Three-choice close appears exactly when another client/workload exists.
- [x] Planned restart reconnects; unplanned loss closes with a notice.
- [x] `feature_key: runner-lifecycle`; CA note written.
