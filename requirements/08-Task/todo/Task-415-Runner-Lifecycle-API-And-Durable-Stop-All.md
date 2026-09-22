# Task-415: Runner Lifecycle API And Durable Stop-All (CP-81 P-2)

## Metadata

- Document ID: `Task-415`
- Title: `Runner Lifecycle API And Durable Stop-All`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-22`
- Last Updated: `2026-09-22`
- Parent Documents: [CP-81](../../07-Coding-Plan/todo/CP-81-Shared-Runner-Lifecycle.md) `P-2`, [SD-28](../../06-System-Tech-Design/SD-28-Shared-Runner-Lifecycle.md) `D-4`, `D-7`, §6
- Child Documents: `None`
- Related Documents: [Task-414](./Task-414-Lifecycle-Lease-Registry-And-State-Machine.md), `CP-51`, `CP-68`, `CA-901`, `CA-911`, `CA-913`
- Replaces: `None`
- Tags: `runner-lifecycle, http-api, shutdown, restart, workload, scaffold, go`
- Feature Keys: `runner-lifecycle`

## AI Quick View

### Summary

- Wire `internal/lifecycle` into the runner HTTP server and process exit path.
- Replace unconditional `/system/shutdown` and `/system/restart` with fenced, confirmation-token actions.
- Add `InteractiveService.LiveWorkSnapshot()` and `StopAllForSystemAction()` so forced shutdown/restart settles active turns, flow/agent loops, scaffolds, and provider executions.
- Preserve CA-911/CA-913 requester attribution and CA-901 bounded cleanup.

### Current Ask

- Make the runner API the authoritative lifecycle surface: clients can register/heartbeat/release, inspect status, request shutdown/restart, and force safely with an inventory-derived confirmation token.

### Key Decisions

- `T-1` `/system/lifecycle` returns the full snapshot; `/health` remains additive and lightweight.
- `T-2` Busy/shared shutdown/restart returns `409 lifecycle_confirmation_required` with a short-lived token; confirmed requests must include `expectedInstanceId`.
- `T-3` Forced stop first settles durable work through `StopAllForSystemAction`, then calls existing bounded provider cleanup.
- `T-4` `scaffoldInFlight` becomes a cancellable claim record so system stop can cancel and terminalize scaffold work.
- `T-5` SIGINT/SIGTERM uses warn-then-force semantics where the platform allows it; OS kill remains unplanned loss.

### Constraints

- No `os.Exit` inside `internal/lifecycle` or `InteractiveService`; `internal/cli` owns final process exit.
- New work is rejected once draining begins.
- Shutdown remains bounded; cleanup must not hang forever.
- All lifecycle logs include requester PID/process attribution when available.
- Do not bypass existing durable run settlement; no second run-state model.

### Open Questions

- `Q-1` Resolved — restart handoff details are delegated to Task-419 for supervisor and Task-416 for client-managed successor; this task only enters `draining_restart` and invokes a handoff callback.

### Source Refs

- `SS-24 AC-6`, `AC-8`, `AC-13..AC-15`, `BR-7..BR-9`.
- `SD-28 D-4`, `D-5`, `D-7`, §6.1–§6.5, §7 force/restart flows.
- Current code: `internal/cli/root.go`, `internal/runner/interactive_service.go`, `internal/runner/scaffold_handler.go`, `internal/runner/runner.go`, `internal/runner/types.go`.

## 1. Goal

Expose the lifecycle manager over HTTP and make every global lifecycle action workload-aware, durable, fenced, and observable.

## 2. Parent Links

- coding plan: `CP-81 P-2`
- tech design: `SD-28 D-4`, `D-5`, `D-7`, §6 API contracts
- system spec: `SS-24 AC-6`, `AC-8`, `AC-13..AC-15`
- prior task: `Task-414`

## 3. Trigger

`/system/shutdown` currently exits after cleanup but does not distinguish normal client close, global force, planned restart, stale replacement, or operator SIGINT. It also lacks a complete active-work inventory, so active scaffold/run state can be killed without terminal evidence.

## 4. Exact Change

- `T-1` Lifecycle routes:
  - `GET /system/lifecycle`
  - `POST /system/clients/register`
  - `POST /system/clients/{leaseId}/heartbeat`
  - `POST /system/clients/{leaseId}/release`
  - Update `POST /system/shutdown` and `POST /system/restart` to the fenced contract.
- `T-2` Additive `/health` fields: `runnerInstanceId`, `generation`, `protocolVersion`, `buildId`, `lifecycleMode`, `phase`.
- `T-3` `internal/runner` workload inventory:
  - `LiveWorkSnapshot()` reads active `turnInFlight`, flow/agent child runs, pending reinvokes, gate settlement, scaffold claims, running provider executions.
  - Warm provider sessions are not counted as active work.
- `T-4` `StopAllForSystemAction(ctx, reason)`:
  - snapshots active work;
  - cancels turn/scaffold/provider contexts;
  - terminalizes or marks recoverable each run/dispatch record;
  - logs stopped IDs and uncertain outcomes;
  - invokes existing bounded session/process cleanup.
- `T-5` `scaffoldInFlight` changes from `map[string]bool` to a claim struct containing project ID, trigger, startedAt, cancel func, and completion status; forced shutdown cancels it and writes terminal status.
- `T-6` Runner serve lifecycle mode:
  - `--lifecycle-mode persistent|client-managed|supervised` or `FLOWPILOT_RUNNER_LIFECYCLE_MODE`;
  - boot grace and idle policy come from `lifecycle.Config`.
- `T-7` Signal handling:
  - first SIGINT on busy/shared runner prints inventory and starts a 5s force window;
  - second SIGINT confirms force;
  - idle SIGINT uses existing bounded cleanup path.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/cli/root.go`
  - `apps/local-runner/internal/cli/shutdown_requester.go` (preserve/extend attribution)
  - `apps/local-runner/internal/runner/lifecycle_handlers.go` (new)
  - `apps/local-runner/internal/runner/interactive_service.go`
  - `apps/local-runner/internal/runner/scaffold_handler.go`
  - `apps/local-runner/internal/runner/{types.go,runner.go}` as needed
- modules: runner HTTP server, interactive service, scaffold dispatch, CLI serve process.
- routes: lifecycle/client/system routes listed above.
- tables: none.

## 6. Code Guide Signatures

```go
// internal/runner/interactive_service.go
type LiveWorkSnapshot struct { /* compatible with lifecycle.WorkloadSnapshot */ }
func (s *InteractiveService) LiveWorkSnapshot(ctx context.Context) (LiveWorkSnapshot, error)
func (s *InteractiveService) StopAllForSystemAction(ctx context.Context, reason string) (StopAllResult, error)

// internal/runner/scaffold_handler.go
type scaffoldClaim struct {
    projectID string
    trigger   string
    startedAt time.Time
    cancel    context.CancelFunc
    status    string
}

// internal/cli/root.go / lifecycle handler adapter
func registerLifecycleRoutes(mux *http.ServeMux, lm *lifecycle.Manager, svc *runner.InteractiveService)
func handleLifecycleSnapshot(w http.ResponseWriter, r *http.Request)
func handleClientRegister(w http.ResponseWriter, r *http.Request)
func handleClientHeartbeat(w http.ResponseWriter, r *http.Request)
func handleClientRelease(w http.ResponseWriter, r *http.Request)
func handleFencedShutdown(w http.ResponseWriter, r *http.Request)
func handleFencedRestart(w http.ResponseWriter, r *http.Request)
```

## 7. Test Signatures

- `TestLifecycleHTTP_RegisterHeartbeatReleaseRoundTrip`
- `TestLifecycleHTTP_SnapshotIncludesClientsAndWork`
- `TestLifecycleHTTP_HeartbeatUnknownLeaseReturns404`
- `TestLifecycleHTTP_StaleGenerationReturns409`
- `TestShutdown_BusyReturnsConfirmationSnapshot`
- `TestShutdown_ConfirmTokenRequired`
- `TestShutdown_ForceStopsAllActiveWork`
- `TestShutdown_StaleConfirmTokenRejected`
- `TestShutdown_DrainingRejectsNewWorkAndLeases`
- `TestShutdown_PreservesRequesterAttribution`
- `TestRestart_BusyRequiresConfirmation`
- `TestRestart_ConfirmedEmitsDrainingRestartSnapshot`
- `TestSIGINT_FirstSignalWarnsSecondForces`
- `TestLiveWorkSnapshot_IncludesRunsChildrenAndScaffolds`
- `TestLiveWorkSnapshot_IgnoresWarmProviderSessions`
- `TestScaffoldClaim_CancelledBySystemStop`
- `TestStopAllForSystemAction_NoRunLeftRunningWithoutEvidence`
- `TestCleanup_RemainsBoundedByDeadline`

## 8. Acceptance Check

- `go test -count=1 ./internal/lifecycle/... ./internal/cli/... ./internal/runner/...` green.
- `go test -race` on lifecycle + focused runner tests where supported.
- `GET /health` remains backward compatible and adds identity fields.
- CA-911/CA-913 telemetry remains present on shutdown/restart.
- GitNexus impact/detect evidence recorded, or unavailable attempt documented.

## 9. Out of Scope

- TUI/Desktop UX and lease clients (`Task-417`, `Task-418`).
- Process spawn/Job Object and stale binary replacement (`Task-416`).
- Supervisor command handling (`Task-419`).

## 10. Definition of Done

- [x] §6 signatures landed.
- [x] §7 tests exist and pass.
- [x] Busy/shared shutdown/restart cannot bypass confirmation.
- [x] Forced stop terminalizes or marks recoverable all active work and logs uncertainty.
- [x] New work/leases are rejected once draining starts.
- [x] `feature_key: runner-lifecycle`; CA note written.
