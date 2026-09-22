# Task-418: Desktop Lease Owner And System UX (CP-81 P-5)

## Metadata

- Document ID: `Task-418`
- Title: `Desktop Lease Owner And System UX`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-22`
- Last Updated: `2026-09-22`
- Parent Documents: [CP-81](../../07-Coding-Plan/done/CP-81-Shared-Runner-Lifecycle.md) `P-5`, [SD-28](../../06-System-Tech-Design/SD-28-Shared-Runner-Lifecycle.md) `D-2`, `D-5`, §6.3
- Child Documents: `None`
- Related Documents: [Task-417](./Task-417-TUI-Lease-Close-UX-And-Reconnect.md), `CP-56`, `CA-474`, `CA-901`
- Replaces: `CA-474 Desktop-exit shutdown contract`
- Tags: `runner-lifecycle, desktop, electron, heartbeat, close-dialog, reconnect`
- Feature Keys: `runner-lifecycle`

## AI Quick View

### Summary

- Electron main owns one Desktop lease; renderer reloads and window crashes do not create extra leases or kill the runner.
- Ordinary app/window close releases the Desktop lease only; `Turn off FlowPilot` is a separate confirmed global action.
- Desktop shows shared clients, active work, idle countdown, update-pending state, planned restart reconnecting state, and unplanned runner-loss notice.
- Renderer system controls route through one lifecycle IPC so sidebar actions and window close cannot diverge.

### Current Ask

- Give Desktop the same lifecycle semantics as TUI, with Electron main as the process-level owner.

### Key Decisions

- `T-1` Lease registration/heartbeat/release lives in `electron/main.ts`, not the React renderer.
- `T-2` `before-quit` no longer unconditionally posts `/system/shutdown`; it loads lifecycle state and chooses release/dialog/force through one helper.
- `T-3` `Turn off FlowPilot` and `Restart Runner` go through lifecycle IPC with snapshot-derived confirmation tokens.
- `T-4` Renderer receives lifecycle status via IPC/store so it can render reconnect and loss states without owning tokens.
- `T-5` On unplanned runner loss, Electron shows a native notice then quits; on planned restart it keeps the app open and reconnects.

### Constraints

- Do not let renderer reload/crash drop the lease prematurely.
- Do not let Electron `before-quit` recurse or re-enter while a lifecycle dialog is open.
- Native dialog labels must match the frozen contract: `Cancel`, `Close Desktop only`, `Turn off FlowPilot`.
- Keep `HttpWsRunnerClient` additive; renderer direct shutdown calls are replaced by lifecycle IPC.

### Open Questions

- `Q-1` Resolved — `Restart Runner` uses the same warn/confirm inventory semantics as global stop when the runner is busy/shared.

### Source Refs

- `SS-24 AC-1..AC-16`, `BR-1..BR-9`, `E-1..E-20`.
- `SD-28 D-2`, `D-4`, `D-5`, §6.3 client behavior.
- Current code: `apps/desktop-flowpilot/electron/main.ts`, `electron/preload.ts`, `src/client/HttpWsRunnerClient.ts`, `src/types/contract.ts`, `src/state/store.ts`, system status/control components.

## 1. Goal

Make Desktop a safe shared-runner client: process-level lease ownership, explicit close choices, global force only after warning, and reliable planned-restart reconnect.

## 2. Parent Links

- coding plan: `CP-81 P-5`
- tech design: `SD-28 D-2`, `D-4`, `D-5`, §6.3
- system spec: `SS-24 AC-1..AC-16`, `BR-1..BR-9`, `E-1..E-20`
- previous slice: `Task-417`

## 3. Trigger

Desktop can currently act as an independent runner owner. `before-quit`/system controls can request shutdown without considering TUI leases or active work, while renderer state does not expose enough lifecycle information to distinguish planned restart from runner death.

## 4. Exact Change

- `T-1` Electron lifecycle manager:
  - register Desktop lease after app ready and runner attach;
  - heartbeat every 5s;
  - release on `Close Desktop only`;
  - expose lease state to renderer via IPC events.
- `T-2` Quit flow:
  - `before-quit`/window close loads `GET /system/lifecycle`;
  - only self + no work → release then quit;
  - shared/busy → `dialog.showMessageBox` with `Cancel`, `Close Desktop only`, `Turn off FlowPilot` + inventory details;
  - global action confirms then force-shutdowns through `/system/shutdown`.
- `T-3` Restart flow:
  - `Restart Runner` calls fenced `/system/restart`;
  - on `draining_restart`, main process polls `/health` for a new `runnerInstanceId`, re-registers, then notifies renderer.
- `T-4` Unplanned loss:
  - heartbeat/health failure without restart marker → native notice `Runner stopped unexpectedly; FlowPilot will close` → release impossible → app quit.
- `T-5` Renderer/store:
  - extend lifecycle DTOs and `RunnerClient`/mock surfaces;
  - route `restartSystem`/`shutdownSystem` through lifecycle IPC or equivalent client wrapper;
  - render `idle shutdown in Ns`, `shared clients`, `active work`, `Update pending`, `Runner restarting…`.
- `T-6` Keep Desktop API client additive:
  - update `HttpWsRunnerClient` DTOs for health/lifecycle fields;
  - do not expose raw lease token to React components; token stays in Electron main.

## 5. Touched Areas

- files:
  - `apps/desktop-flowpilot/electron/main.ts`
  - `apps/desktop-flowpilot/electron/preload.ts` / global typings if present
  - `apps/desktop-flowpilot/src/client/HttpWsRunnerClient.ts`
  - `apps/desktop-flowpilot/src/types/contract.ts`
  - `apps/desktop-flowpilot/src/state/store.ts`
  - system controls/status components and tests
  - `packages/flowpilot-client-core` lifecycle mapping if still used by this app path
- modules: Electron main lifecycle, preload IPC, renderer store/status.
- routes: consumes lifecycle endpoints; no new runner route.
- tables: none.

## 6. Code Guide Signatures

```ts
// electron lifecycle owner (name may be colocated in main.ts or electron/lifecycle.ts)
interface RunnerLifecycleState {
  leaseId?: string;
  runnerInstanceId?: string;
  generation?: number;
  phase?: string;
  reconnectDeadlineMs?: number;
}

async function registerDesktopLease(): Promise<void>
async function heartbeatDesktopLease(): Promise<void>
async function releaseDesktopLease(): Promise<void>
async function requestDesktopQuit(source: "window"|"app"|"ipc"): Promise<void>
async function requestGlobalShutdown(source: string): Promise<void>
async function requestPlannedRestart(source: string): Promise<void>
```

```ts
// preload / renderer-facing lifecycle bridge
interface RunnerLifecycleBridge {
  getSnapshot(): Promise<LifecycleSnapshot>;
  requestClose(): Promise<"cancelled"|"closed">;
  requestGlobalShutdown(): Promise<LifecycleActionResult>;
  requestRestart(): Promise<LifecycleActionResult>;
}
```

## 7. Test Signatures

- `TestDesktop_RegisterLeaseOncePerApp`
- `TestDesktop_RendererReloadDoesNotCreateSecondLease`
- `TestDesktop_HeartbeatKeepsLeaseAlive`
- `TestDesktop_WindowCloseShowsThreeChoices`
- `TestDesktop_CloseDesktopOnlyReleasesLease`
- `TestDesktop_TurnOffWarnsThenForces`
- `TestDesktop_RestartUsesConfirmTokenAndReconnects`
- `TestDesktop_PlannedRestartKeepsAppOpen`
- `TestDesktop_UnplannedRunnerLossClosesWithNotice`
- `TestDesktop_StatusShowsIdleCountdownAndUpdatePending`
- `TestDesktop_MockRunnerClientLifecycleParity`

## 8. Acceptance Check

- Desktop typecheck/build and focused tests green.
- Manual CP-81 sections B, C, E, F pass on Windows.
- Ordinary Desktop close never calls global shutdown directly.
- GitNexus impact/detect evidence recorded, or unavailable attempt documented.

## 9. Out of Scope

- Runner API internals (`Task-415`).
- TUI app UX (`Task-417`).
- Supervisor process tree management (`Task-419`).

## 10. Definition of Done

- [x] §6 signatures landed.
- [x] §7 tests exist and pass.
- [x] Electron main owns exactly one Desktop lease.
- [x] Three-choice close and global warning inventory are implemented.
- [x] Planned restart reconnects; unplanned runner death notifies then closes.
- [x] `feature_key: runner-lifecycle`; CA note written.
