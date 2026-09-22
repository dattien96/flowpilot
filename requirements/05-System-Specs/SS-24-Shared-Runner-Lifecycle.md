# SS-24: Shared Runner Lifecycle

## Metadata

- Document ID: `SS-24`
- Title: `Shared Runner Lifecycle — Lease Ownership, Idle Shutdown, Controlled Stop/Restart`
- Phase: `system_spec`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-22`
- Last Updated: `2026-09-22`
- Parent Documents: `None`
- Child Documents: `SD-28 (planned)`, `CP-81 (planned)`
- Related Documents: `CP-56`, `CP-68`, `CP-71`, `BUG-240`, `BUG-328`, `CA-445`, `CA-474`, `CA-901`, `CA-911`, `CA-913`
- Replaces: `CA-474 client-exit contract assumption`
- Tags: `runner, lifecycle, lease, heartbeat, tui, desktop, supervisor, shutdown, restart, windows`

## AI Quick View

### Summary

- FlowPilot has two user-facing clients — TUI and Desktop — sharing one local runner. Current lifecycle logic is process-owned: a client exit can kill the shared runner, while crash/terminal kill paths can leak it.
- The runner must become the lifecycle authority. Clients register generation-scoped leases and heartbeat; normal close releases only that client's lease; TTL expiry handles crashes and Task Manager kills.
- When the last live lease disappears and no protected work remains, the runner waits **30 seconds**, then cancels/cleans work and exits. Reconnecting during grace cancels the shutdown.
- Deliberate close and deliberate global shutdown are separate actions. A shared-close surface offers `Cancel`, `Close this client only`, and `Turn off FlowPilot`.
- Planned restart keeps clients in reconnect mode; unexpected runner death notifies clients and closes them. Stale runner builds auto-replace only when idle.

### Current Ask

- Define the user-facing lifecycle contract for shared TUI/Desktop runner ownership so `SD-28` and `CP-81` can implement one coherent lease/state machine without reopening product policy.

### Key Decisions

- `AC-1` Client presence is lease-based, not a raw counter and not parent-process ownership.
- `AC-2` A normal client exit releases only that client's lease.
- `AC-3` The zero-client/no-work idle grace is **30 seconds**.
- `AC-4` Active AI work blocks idle shutdown and ordinary client close never kills it.
- `AC-5` `Turn off FlowPilot` is the only global destructive action and is warn-then-force.
- `AC-6` Planned restart reconnects clients; unplanned runner loss closes clients with a clear notice.
- `AC-7` Stale builds are auto-replaced only while idle; busy runners report `Update pending`.

### Constraints

- Local runner supports Desktop, TUI, supervisor-managed dev stack, and manually spawned `runner serve`.
- Windows process behavior is mandatory: Task Manager kill, terminal close, `go run` wrapper + compiled child, and Job Object semantics must be handled explicitly.
- No remote coordination service exists; all lifecycle state must be local, low-latency, and recoverable enough for diagnostics.
- Existing active work (chat turns, flow runs, agent children, scaffold dispatch, provider subprocesses) must not disappear silently.

### Open Questions

- `Q-1` Resolved — the shared-close dialog is exactly three choices: `Cancel`, `Close this client only`, `Turn off FlowPilot`.
- `Q-2` Resolved — runner restart is a planned lifecycle event that asks connected clients to reconnect rather than treating them as dead.

### Source Refs

- Live incident 2026-09-22: `POST /system/shutdown` from an unidentified Go client killed the runner while TUI remained alive; CA-911/CA-913 telemetry was added to identify future callers.
- `CA-445`, `CA-474` — prior close behavior deliberately killed/reused runners under the obsolete assumption that TUI and Desktop are not used concurrently.
- `BUG-240`/`CA-240` — Windows `go run` wrapper vs compiled child requires process-tree kill for supervisor-owned shutdown.
- `BUG-328`/`CA-671` — TUI input freeze and runner-job coupling evidence.
- `CP-51`/`SD-25` — durable dispatch/recovery semantics that forced shutdown must respect.

## 1. Goal

Give the local runner a predictable shared-daemon lifecycle: clients can come and go safely, crashes do not leak ownership, active work is protected, explicit system shutdown remains possible, planned restart reconnects clients, and stale runner binaries are replaced only when safe.

## 2. Problem

Today three independent mechanisms decide whether the runner lives:

1. TUI quit posts `/system/shutdown` even when it reused another client's runner.
2. Desktop/Electron quit paths can issue global shutdown independently of connected clients/work.
3. TUI-spawned Windows runners are assigned to a Job Object with `KILL_ON_JOB_CLOSE`, so TUI process death kills the runner immediately.

This causes both failure directions:

- **Accidental kill:** closing one client can terminate the runner while the other client still needs it.
- **Leak/stale binary:** if shutdown is skipped to avoid the first problem, crashed or repeatedly restarted clients leave orphaned runners holding old binaries and consuming resources.

The live incident showed a third problem: the runner accepted a shutdown request from a Go process that could not be attributed until extra requester telemetry was added. Lifecycle commands need identity, fencing, and an explicit authority model.

## 3. Scope

- In scope:
  - Runner-owned client lease registration, heartbeat, release, expiry, and lifecycle snapshot.
  - Shared TUI/Desktop close UX and global `Turn off FlowPilot` confirmation.
  - Idle shutdown after 30 seconds when no clients and no protected work remain.
  - Planned restart with client reconnect; unexpected runner loss handling.
  - Stale runner build detection and idle-only replacement.
  - Supervisor/TUI spawn ownership semantics and Windows Job Object decoupling.
  - Process-exit telemetry and requester attribution for lifecycle commands.
- Out of scope:
  - Multi-machine runner coordination.
  - Remote/Drive state locking.
  - Admin Web lifecycle controls.
  - Persisting client leases across runner restart.
  - Changing provider model, prompt, gate, or chat semantics.

## 4. Non-Goals

- Not a generic process supervisor for arbitrary user daemons.
- Not a mechanism to keep an abandoned runner alive indefinitely because it once had work.
- Not automatic update while work is running.
- Not a replacement for CP-51 durable recovery — lifecycle shutdown must settle runs, not invent a second recovery model.

## 5. User Stories or Primary Use Cases

- `US-1` As a user running TUI and Desktop together, I can close either client without breaking the other.
- `US-2` As a user who closes both clients, I want the runner to stop shortly afterward so it does not consume resources or keep an old binary alive.
- `US-3` As a user whose terminal or Desktop process crashes, I want the runner to forget that client automatically instead of waiting forever.
- `US-4` As a user with an AI run/scaffold in progress, I want ordinary close to protect it and explicit `Turn off FlowPilot` to warn me before stopping it.
- `US-5` As a user, when the runner needs a planned restart, I want both clients to reconnect instead of closing unexpectedly.
- `US-6` As a user, when the runner dies unexpectedly, I want a clear notification rather than a frozen TUI/Desktop window.
- `US-7` As a developer/user, when my runner binary is stale, I want FlowPilot to replace it automatically only when idle and tell me when replacement is pending.

## 6. Acceptance Criteria

- `AC-1` Every TUI/Desktop session registers a generation-scoped lease containing stable client identity, client kind, PID/session identity, heartbeat timestamps, and expiry. Heartbeat interval is 5 seconds and lease TTL is 15 seconds.
- `AC-2` Normal `/exit`, TUI quit, or Desktop window/app close releases only that client's lease and never calls global shutdown by itself.
- `AC-3` When the last live lease is released or expires and no active work exists, runner enters `idle_grace` with a 30-second shutdown deadline; a valid attach during grace cancels it.
- `AC-4` If the last client disappears while active work exists, runner does not silently exit mid-work; it enters an orphaned-work state, finishes/settles or awaits cancellation, then starts idle grace when no work remains.
- `AC-5` When a user asks to close a client while another client or active work exists, UI offers exactly `Cancel`, `Close this client only`, and `Turn off FlowPilot` with visible client/work inventory.
- `AC-6` `Turn off FlowPilot` warns with the affected client and workload list, requires confirmation, cancels active AI runs/scaffolds/provider subprocesses, then stops the runner and closes remaining clients.
- `AC-7` Ctrl+C or terminal close on a client is normal client loss; it never kills a runner needed by another lease. Lease expiry performs cleanup if it was the last client.
- `AC-8` Ctrl+C in the runner terminal is operator intent: idle runner exits; busy/shared runner warns once and requires a second confirmation within a bounded window before force-stopping.
- `AC-9` Task Manager kill of a client causes lease expiry; Task Manager kill of the runner causes clients to detect heartbeat/health loss and close with a notification.
- `AC-10` Planned runner restart emits a restart state/token to connected clients; clients show reconnecting state, wait for a new runner instance, re-register, and restore durable state. Reconnect timeout closes the client with a clear notice.
- `AC-11` A stale compatible runner build is auto-replaced only when zero clients and zero protected work are reported; otherwise clients show `Update pending` and the current runner is not killed silently.
- `AC-12` Runner reuse requires matching workspace semantics and compatible protocol/build identity; a port occupied by a non-FlowPilot/unknown process is a port conflict and must not be killed blindly.
- `AC-13` Lifecycle actions are idempotent and fenced by runner instance/generation: stale heartbeat, duplicate registration, stale release, stale confirm token, or stale supervisor command cannot affect a new runner generation.
- `AC-14` All shutdown/restart requests log lifecycle reason, requester lease/client kind, remote endpoint, user agent, and resolved requester PID/process name when available.
- `AC-15` Forced shutdown and planned restart leave no durable run/turn/scaffold permanently `running` without evidence; each item is terminalized, marked recoverable/cancelled, or reported as uncertain.
- `AC-16` The lifecycle contract is provider-agnostic: Claude, Codex, Gemini, Grok, OpenCode, and Devin use the same lease/stop/restart semantics.

## 7. Business Rules

- `BR-1` The runner is the lifecycle authority; clients never decide global shutdown based only on their own exit.
- `BR-2` `Close this client only` is the default safe close path when the user chooses to exit; global stop is never implied.
- `BR-3` Lease expiry is the crash-recovery mechanism; abnormal exits cannot decrement counters.
- `BR-4` Warm provider sessions/caches are not protected work; they may be cleaned during shutdown. Active turns, flow/agent loops, and scaffold dispatches are protected work.
- `BR-5` `persistent` manual `runner serve` remains operator-owned and does not self-exit merely because no app client is connected; `client-managed` and `supervised` runners follow the idle policy.
- `BR-6` Supervisor may control restart/shutdown but must not hold a permanent user-client lease that would leak the runner forever.
- `BR-7` Any destructive action against a busy/shared runner requires a current confirmation token derived from the lifecycle snapshot.
- `BR-8` The runner must reject or gate new work once it enters draining shutdown/restart.
- `BR-9` No lifecycle path may kill an unknown process merely because it occupies port 4317.

## 8. Edge Cases

- `E-1` TUI only connected, no work → TUI close releases lease; runner exits after 30s unless reopened.
- `E-2` Desktop only connected, no work → same behavior.
- `E-3` TUI + Desktop connected → closing either shows three choices; `Close this client only` leaves the other client and runner alive.
- `E-4` Last client closes while a run/scaffold is active → runner enters orphaned-work, then idle grace after work drains.
- `E-5` Client crash/terminal close/Task Manager kill → heartbeat stops; lease expires after TTL; remaining clients unaffected.
- `E-6` Runner terminal Ctrl+C while busy → warning inventory + second Ctrl+C force; while idle → bounded cleanup and exit.
- `E-7` Runner process killed externally → clients detect unplanned loss, notify, and close; next launch performs normal recovery.
- `E-8` Client reconnects during `idle_grace` → countdown cancels atomically and the client attaches normally.
- `E-9` Planned restart while clients connected → clients enter reconnect mode, runner performs confirmed drain, new generation accepts leases.
- `E-10` Planned restart times out or never returns → clients show restart failure and close with notice.
- `E-11` Stale runner while idle → fenced shutdown and replacement with expected build.
- `E-12` Stale runner while busy → `Update pending`; explicit restart may be offered but never silent kill.
- `E-13` Existing runner reports a different workspace/cwd → client does not attach as if compatible; it surfaces a mismatch/choice per UX contract.
- `E-14` Port 4317 occupied by an unknown non-FlowPilot process → report port conflict; do not kill.
- `E-15` Duplicate heartbeat/release/restart/shutdown requests → idempotent success or typed stale-generation errors, never double cleanup.
- `E-16` Release races heartbeat → last write wins under the lease lock; expired/released lease cannot be revived by an older heartbeat.
- `E-17` Global shutdown races new attach → after drain begins, new registrations are rejected with `runner_draining`.
- `E-18` Restart races heartbeat → heartbeats return `draining_restart` + `restartId`; clients must not interpret that as normal online state.
- `E-19` Runner exits before persisting planned restart marker → clients classify loss as unplanned and close.
- `E-20` Windows `go run` wrapper dies but compiled child remains → process-tree cleanup must still find the child; no orphan binary remains.

## 9. Dependencies

- Runner HTTP server and process exit paths (`internal/cli/root.go`, `internal/runner/*`).
- Durable run/turn/scaffold state and cancellation paths (`InteractiveService`, scaffold dispatcher).
- TUI Bubble Tea app, runner boot package, TUI HTTP client.
- Desktop Electron main lifecycle, preload IPC, renderer store/status components.
- `scripts/supervisor.js` and supervisor command files.
- Windows process/job semantics and Unix signal behavior.
- CP-51/SD-25 recovery/durability contracts.

## 10. Open Questions

- `Q-1` Resolved — close dialog labels: `Cancel`, `Close TUI only`/`Close Desktop only`, `Turn off FlowPilot`.
- `Q-2` Resolved — reconnect window is a bounded retry rather than indefinite waiting; exact timeout is a technical decision in SD-28.
- `Q-3` Resolved — leases stay in-memory and generation-scoped; a runner restart intentionally invalidates them.

## 11. Definition of Done

- This spec is complete when `SD-28` and `CP-81` trace every `AC`, `BR`, and `E` item, and no product decision remains required to implement the shared lifecycle UX.
