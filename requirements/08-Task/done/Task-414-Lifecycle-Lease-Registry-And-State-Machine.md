# Task-414: Lifecycle Lease Registry And State Machine (CP-81 P-1)

## Metadata

- Document ID: `Task-414`
- Title: `Lifecycle Lease Registry And State Machine`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-22`
- Last Updated: `2026-09-22`
- Parent Documents: [CP-81](../../07-Coding-Plan/done/CP-81-Shared-Runner-Lifecycle.md) `P-1`, [SD-28](../../06-System-Tech-Design/SD-28-Shared-Runner-Lifecycle.md) `D-1..D-5`
- Child Documents: `None`
- Related Documents: [Task-413](./Task-413-Shared-Runner-Lifecycle-Contract-Freeze.md), `SS-24`, `SD-25`
- Replaces: `None`
- Tags: `runner-lifecycle, lease, heartbeat, state-machine, go`
- Feature Keys: `runner-lifecycle`

## AI Quick View

### Summary

- Add a new `internal/lifecycle` package that owns generation-scoped client leases, heartbeat TTL expiry, idle grace, workload-aware transitions, confirmation tokens, and restart markers.
- Keep the domain manager deterministic: injectable clock, sweeper interval, work snapshot provider, and lifecycle callbacks; no `os.Exit` inside the package.
- This package is the single lifecycle authority consumed by runner HTTP handlers, runnerboot, TUI/Desktop clients, and supervisor-facing commands.

### Current Ask

- Implement the closed state machine and lease registry before touching TUI/Desktop UI or process ownership.

### Key Decisions

- `T-1` Leases are in-memory only and generation-scoped; restart invalidates them by design.
- `T-2` `leaseToken` and `confirmToken` are random and stored hashed; raw tokens are returned once to the caller and never logged.
- `T-3` Every mutation is serialized by the manager; stale generation/instance/token requests return typed errors.
- `T-4` `idle_grace` starts only when live leases = 0 and protected work = 0; active work transitions to `orphaned_work` instead.

### Constraints

- Provider-agnostic and storage-free.
- Race-safe under concurrent register/heartbeat/release/expire/shutdown/restart calls.
- Timeouts/timers must be fakeable in unit tests.
- Must not kill processes or call `os.Exit`; it returns intents/callbacks to `internal/cli`/`internal/runner`.

### Open Questions

- None; API/state names are frozen in `SD-28`.

### Source Refs

- `SS-24 AC-1..AC-5`, `AC-13`, `BR-1..BR-8`, `E-15..E-19`.
- `SD-28 D-1..D-5`, §5 data model, §8 failure matrix.
- `internal/cli/root.go` existing shutdown/restart flow that will consume the manager in Task-415.

## 1. Goal

Create a deterministic lifecycle domain package that can prove all shared-runner state transitions and race cases without starting real clients or processes.

## 2. Parent Links

- coding plan: `CP-81 P-1`
- tech design: `SD-28 D-1..D-5`
- system spec: `SS-24 AC-1..AC-5`, `AC-13`, `E-15..E-19`

## 3. Trigger

The runner currently has no durable ownership model. A raw client counter cannot handle crash/Task Manager death, and process-parent ownership cannot support TUI+Desktop sharing. A lease registry plus explicit state machine is required before any UI or process work.

## 4. Exact Change

- `T-1` New package `apps/local-runner/internal/lifecycle` with:
  - `Manager`
  - `ClientLease`
  - `LifecycleSnapshot`
  - `ClientLeaseView`
  - `WorkloadSnapshot` / `WorkloadItem`
  - `RunnerPhase`, `LifecycleMode`, `ClientKind`
  - typed lifecycle errors (`lease_unknown`, `invalid_lease_token`, `stale_generation`, `runner_draining`, `lifecycle_confirmation_required`, `stale_lifecycle_snapshot`)
- `T-2` Register/heartbeat/release methods with:
  - generation/instance fencing;
  - per-client-instance idempotent registration;
  - secure token generation + hashed storage;
  - heartbeat refresh and release idempotence.
- `T-3` Sweeper and timers:
  - expire leases after TTL;
  - boot grace for `client-managed`/`supervised`;
  - 30s `idle_grace` deadline;
  - cancellation on new lease or protected work.
- `T-4` Confirmation token flow:
  - `RequestShutdown` / `RequestRestart` return `confirmToken` + inventory when busy/shared;
  - confirmed request binds token to action kind, `runnerInstanceId`, and `InventoryRevision`.
- `T-5` Restart state:
  - `draining_restart` carries `restartId`, deadline, and handoff metadata;
  - snapshots expose reconnect eligibility.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/lifecycle/manager.go` (new)
  - `apps/local-runner/internal/lifecycle/types.go` (new)
  - `apps/local-runner/internal/lifecycle/manager_test.go` (new)
- modules: new domain package; consumed later by `internal/cli` and `internal/runner`.
- routes: none in this task.
- tables: none.

## 6. Code Guide Signatures

```go
// apps/local-runner/internal/lifecycle/types.go
type ClientKind string
const (
    ClientKindTUI     ClientKind = "tui"
    ClientKindDesktop ClientKind = "desktop"
)

type LifecycleMode string
const (
    ModePersistent    LifecycleMode = "persistent"
    ModeClientManaged LifecycleMode = "client-managed"
    ModeSupervised    LifecycleMode = "supervised"
)

type RunnerPhase string
const (
    PhaseStarting         RunnerPhase = "starting"
    PhaseReady            RunnerPhase = "ready"
    PhaseIdleGrace        RunnerPhase = "idle_grace"
    PhaseOrphanedWork     RunnerPhase = "orphaned_work"
    PhaseDrainingRestart  RunnerPhase = "draining_restart"
    PhaseDrainingShutdown RunnerPhase = "draining_shutdown"
    PhaseStopped          RunnerPhase = "stopped"
)

type ClientLease struct { /* SD-28 §5 */ }
type LifecycleSnapshot struct { /* SD-28 §5 */ }
type WorkloadSnapshot struct { ActiveCount int; Items []WorkloadItem }

// manager.go
type Config struct {
    RunnerInstanceID string
    Mode             LifecycleMode
    HeartbeatTTL     time.Duration
    BootGrace        time.Duration
    IdleGrace        time.Duration
    ReconnectGrace   time.Duration
    Now              func() time.Time
    WorkSnapshot     func(context.Context) (WorkloadSnapshot, error)
    OnPhase          func(RunnerPhase, LifecycleSnapshot)
}

type Manager struct { /* mutex, leases, timers, pending confirmations */ }
func NewManager(cfg Config) *Manager
func (m *Manager) Register(ctx context.Context, in RegisterInput) (RegisterResult, error)
func (m *Manager) Heartbeat(ctx context.Context, in HeartbeatInput) (LifecycleSnapshot, error)
func (m *Manager) Release(ctx context.Context, in ReleaseInput) (LifecycleSnapshot, error)
func (m *Manager) Snapshot(ctx context.Context) (LifecycleSnapshot, error)
func (m *Manager) RequestShutdown(ctx context.Context, in SystemActionInput) (SystemActionResult, error)
func (m *Manager) RequestRestart(ctx context.Context, in SystemActionInput) (SystemActionResult, error)
func (m *Manager) Close() // test cleanup; stops timers only
```

## 7. Test Signatures

- `TestLifecycle_RegisterIssuesGenerationScopedLease`
- `TestLifecycle_RegisterSameClientInstanceIsIdempotent`
- `TestLifecycle_RegisterDuringDrainRejected`
- `TestLifecycle_HeartbeatExtendsLease`
- `TestLifecycle_HeartbeatStaleGenerationRejected`
- `TestLifecycle_HeartbeatInvalidTokenRejected`
- `TestLifecycle_ReleaseRemovesClient`
- `TestLifecycle_ReleaseIsIdempotent`
- `TestLifecycle_ExpiredLeaseTransitionsIdleGrace`
- `TestLifecycle_RegisterDuringIdleGraceCancelsShutdown`
- `TestLifecycle_WorkloadBlocksIdleGrace`
- `TestLifecycle_OrphanedWorkTransitionsToIdleGraceWhenDrained`
- `TestLifecycle_RequestShutdownBusyReturnsConfirmation`
- `TestLifecycle_ConfirmedShutdownUsesFreshInventoryToken`
- `TestLifecycle_StaleConfirmTokenRejected`
- `TestLifecycle_RequestRestartEntersDrainingRestart`
- `TestLifecycle_RestartSnapshotCarriesReconnectDeadline`
- `TestLifecycle_ConcurrentRegisterAndExpiryAreAtomic`
- `TestLifecycle_ConcurrentShutdownAndAttachAreSerialized`
- `TestLifecycle_TokensAreHashedAndNotLogged`

## 8. Acceptance Check

- `go test -count=1 ./internal/lifecycle/...` green.
- `go test -race ./internal/lifecycle/...` green where supported.
- No references to `os.Exit`, `exec.Command`, HTTP handlers, or provider keys inside `internal/lifecycle`.
- GitNexus impact/detect evidence recorded, or unavailable attempt documented.

## 9. Out of Scope

- HTTP routes and `runner serve` wiring (`Task-415`).
- Runner process spawning or Job Object changes (`Task-416`).
- TUI/Desktop/supervisor clients (`Task-417..419`).

## 10. Definition of Done

- [x] §6 signatures landed.
- [x] §7 tests exist and pass.
- [x] State/event matrix is closed for register, heartbeat, release, expiry, attach, work drain, shutdown, restart.
- [x] Tokens are random, hashed at rest, and never logged.
- [x] `feature_key: runner-lifecycle`; CA note written.
