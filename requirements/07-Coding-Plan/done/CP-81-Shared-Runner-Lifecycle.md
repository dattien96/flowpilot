# CP-81: Shared Runner Lifecycle (Lease + Idle Grace + Controlled Stop/Restart)

## Metadata

- Document ID: `CP-81`
- Title: `Shared Runner Lifecycle — Lease Registry, Idle Shutdown, Controlled Stop/Restart`
- Feature Keys: `runner-lifecycle`
- Phase: `coding_plan`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-22`
- Last Updated: `2026-09-22`
- Parent Documents: [SS-24: Shared Runner Lifecycle](../../05-System-Specs/SS-24-Shared-Runner-Lifecycle.md), [SD-28: Shared Runner Lifecycle](../../06-System-Tech-Design/SD-28-Shared-Runner-Lifecycle.md)
- Child Documents: `Task-413..Task-420`, [CP-81-Test-Steps](./CP-81-Test-Steps.md)
- Related Documents: [CP-56](../done/CP-56-Terminal-TUI-Chat-And-Flow-Client.md), [CP-68](../done/CP-68-Skill-Anchored-Scaffold-And-AI-Guided-Init.md), [CP-71](./CP-71-Run-Worktree-Isolation.md), `BUG-240`, `BUG-328`, `CA-445`, `CA-474`, `CA-901`, `CA-911`, `CA-913`
- Replaces: `CA-474 client-exit contract assumption`
- Tags: `runner, lifecycle, lease, heartbeat, tui, desktop, supervisor, shutdown, restart, windows`

## AI Quick View

### Summary

- Implement `SS-24`/`SD-28` in eight slices: contract freeze, lifecycle manager, runner API + durable stop-all, runnerboot/process decoupling, TUI lifecycle UX, Desktop lifecycle UX, supervisor handoff, then E2E/migration hardening.
- The runner becomes the lifecycle authority: generation-scoped leases + heartbeat TTL replace parent-process ownership and raw counters.
- Normal close releases one lease; zero leases + zero work starts a **30-second idle grace**; attach during grace cancels shutdown.
- Global shutdown/restart is fenced by runner instance + confirmation token; busy/shared requests return inventory and require explicit confirmation.
- Planned restart is distinguishable from runner death: clients reconnect after a marked `draining_restart`; unplanned loss closes clients with a notice.

### Current Ask

- Land one coherent lifecycle architecture across Runner, TUI, Desktop, and supervisor. Do not fix only `/system/shutdown`; the Windows Job Object ownership and stale-binary reuse rules must be redesigned in the same contract.

### Key Decisions

- `P-0` Freeze `SS-24`, `SD-28`, `CP-81`, `CP-81-Test-Steps`, feature key `runner-lifecycle`, and `Task-413..420` before code edits.
- `P-1` New `internal/lifecycle` package owns all state transitions and lease tokens; domain logic never calls `os.Exit` directly.
- `P-2` `LiveWorkSnapshot` + `StopAllForSystemAction` make forced shutdown/restart durable for turns, children, gates, scaffold dispatches, and provider executions.
- `P-3` `runnerboot` removes TUI-owned Windows Job coupling and replaces stale builds only when lifecycle proves idle.
- `P-4`/`P-5` TUI and Desktop expose `Cancel`, `Close this client only`, `Turn off FlowPilot`; neither performs global shutdown on ordinary close.
- `P-6` Supervisor is a fenced controller, not a permanent client lease; planned restart uses a command record carrying `runnerInstanceId` + `restartId`.
- `P-7` E2E verifies the complete state×event matrix, including crash, Task Manager kill, planned restart, stale build, and wrong-workspace/port-conflict cases.

### Constraints

- `safe-fix-contract`: additive coverage for every changed path; old tests stay green unless CP-81 explicitly supersedes a stale contract and the CA documents it.
- Preserve CA-911/CA-913 requester attribution (`remote`, `ua`, `X-Client`, resolved PID/process name).
- Preserve BUG-240 process-tree cleanup on Windows; removing Job Object coupling must not orphan `go run` compiled children.
- No Supabase schema migration; lifecycle leases are in-memory and generation-scoped.
- Provider-agnostic: no provider-specific shutdown semantics.
- GitNexus `impact` before symbol edits and `detect_changes` before commits; if unavailable, record the failed tool attempt and perform manual blast-radius review.

### Open Questions

- `Q-1` Resolved — close labels are `Cancel`, `Close TUI only`/`Close Desktop only`, `Turn off FlowPilot`.
- `Q-2` Resolved — reconnect deadline is 60 seconds unless SD-28 implementation evidence requires tuning.
- `Q-3` Resolved — manual `runner serve` defaults to `persistent`; TUI/Desktop/supervisor-spawned runners use auto idle lifecycle modes.

### Source Refs

- `SS-24 AC-1..AC-16`, `BR-1..BR-9`, `E-1..E-20`.
- `SD-28 D-1..D-9`, §5 data model, §6 contracts, §7 flows, §8 failures.
- Current code: `internal/cli/root.go`, `internal/runner/interactive_service.go`, `internal/runner/scaffold_handler.go`, `internal/tui/client/client.go`, `internal/tui/runnerboot/*`, `apps/desktop-flowpilot/electron/main.ts`, `src/client/HttpWsRunnerClient.ts`, `src/state/store.ts`, `scripts/supervisor.js`.

## 1. Goal

Replace destructive client-owned shutdown with a shared-runner lifecycle that is safe when TUI and Desktop run together, still cleans up abandoned runners after 30 seconds, protects active work, supports deliberate global stop and restart, and replaces stale binaries only when idle.

## 2. Input Documents

- `SS-24` — user-facing lifecycle contract and acceptance criteria.
- `SD-28` — lease/state machine/API/process ownership design.
- `CP-56` — existing TUI client and runner boot behavior.
- `CP-68` — scaffold dispatch and init workload surface.
- `CP-51`/`SD-24`/`SD-25`/`SD-26` — durable run/recovery contracts.
- `BUG-240`, `BUG-328`, `CA-445`, `CA-474`, `CA-901`, `CA-911`, `CA-913` — lifecycle failure history and telemetry requirements.

## 3. Implementation Strategy

- **Overall approach:** bottom-up, contract-first. Land the lifecycle manager and HTTP contract before changing client behavior; land process decoupling before relying on lease expiry; land durable stop-all before exposing force shutdown.
- **Sequencing:** `P-0` contract freeze → `P-1` manager → `P-2` runner API/work stop → `P-3` boot/build/process ownership → `P-4` TUI → `P-5` Desktop → `P-6` supervisor → `P-7` cross-client E2E/audit.
- **Dependencies:** `P-2` needs `P-1`; `P-4`/`P-5` need `P-2`; `P-6` needs `P-2` restart contract; `P-7` validates the whole matrix.
- **Compatibility:** keep old `/health` fields; add lifecycle fields. Older clients receive bounded compatibility presence, while new clients use explicit leases. Legacy runners that cannot prove idle require explicit replacement confirmation.

## 4. Work Breakdown

- `P-0` **Contract/docs freeze — `Task-413`.** Write `SS-24`, `SD-28`, `CP-81`, `CP-81-Test-Steps`, task docs `Task-413..420`, and append `runner-lifecycle` to `FEATURE-KEYS.md`. Freeze endpoint names, DTOs, labels, timers, error codes, migration behavior, and the state×event matrix.
- `P-1` **Lifecycle lease registry + state machine — `Task-414`.** New `apps/local-runner/internal/lifecycle` package: lease registry, secure token hashes, client instance dedupe, heartbeat/TTL sweeper, phase transitions, workload snapshot input, idle timer, confirmation tokens, restart markers, injectable clock/exit callbacks.
- `P-2` **Runner lifecycle API + durable stop-all — `Task-415`.** Wire `/system/lifecycle`, register/heartbeat/release, fenced shutdown/restart, lifecycle health fields, signal semantics, `InteractiveService.LiveWorkSnapshot`, `StopAllForSystemAction`, scaffold claim cancellation, and requester telemetry.
- `P-3` **Runnerboot decoupling + stale replacement — `Task-416`.** Remove/change Windows `KILL_ON_JOB_CLOSE`, spawn client-managed runners independently, add boot lock, compute build ID, classify compatible/idle stale/busy stale/protocol incompatible/legacy unknown, and implement fenced idle replacement.
- `P-4` **TUI lease client + close/reconnect UX — `Task-417`.** Register/heartbeat/release, ordinary close without global shutdown, three-choice dialog, planned restart reconnect, unplanned runner-loss notice, stale/update-pending status, and updated obsolete shutdown tests.
- `P-5` **Desktop/Electron lease owner + system UX — `Task-418`.** Electron main owns one app lease; window close and system controls share the same IPC/confirmation path; renderer shows lifecycle status, update pending, restart reconnect, and unplanned loss close notice.
- `P-6` **Supervisor coordination — `Task-419`.** Launch runner in `supervised` mode, fence command files by runner instance/restart ID, preserve Windows tree kill, avoid ghost restart after unexpected runner death, and keep full-stack force shutdown semantics.
- `P-7` **Cross-client E2E + migration/audit — `Task-420`.** Real HTTP/process matrix for shared clients, crash/TTL, idle grace, busy work, force shutdown, restart reconnect, unexpected death, stale replacement, wrong workspace, port conflict, old-client compatibility, and final CA ledger.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/lifecycle/*` (new)
  - `apps/local-runner/internal/cli/{root.go,chat.go}` and lifecycle handler files
  - `apps/local-runner/internal/runner/{runner.go,types.go,interactive_service.go,scaffold_handler.go}` plus tests
  - `apps/local-runner/internal/tui/{client,app,runnerboot,config}/*`
  - `apps/desktop-flowpilot/electron/{main.ts,preload.ts}` and renderer lifecycle/status/store/client files
  - `packages/flowpilot-client-core` health/lifecycle mapping if it remains the shared contract owner
  - `scripts/supervisor.js` and supervisor tests
  - `requirements/{05-System-Specs,06-System-Tech-Design,07-Coding-Plan,08-Task}`
  - `change-audit/FEATURE-KEYS.md`, `change-audit/CA-NNN-*`
- modules: lifecycle manager, runner HTTP server, durable run/scaffold cancellation, TUI app loop, Electron main process, supervisor.
- database: none.
- external systems: OS process APIs (`Job Object`, process groups, `taskkill`), local HTTP loopback only.

## 6. Data or Migration Steps

- schema: none. Leases are in-memory and generation-scoped.
- data backfill: none.
- config updates:
  - new runner lifecycle mode flag/env (`persistent`, `client-managed`, `supervised`);
  - `.flowpilot/` boot lock and fenced restart/control records where needed;
  - Desktop/TUI lifecycle client settings remain internal implementation details.
- migration:
  - New clients register leases.
  - New runners keep additive health fields for old clients.
  - Legacy client traffic/open streams create bounded `legacy_presence` evidence until lease-capable clients fully roll out.
  - Old runner lacking lifecycle fields is `legacy_unknown`: do not auto-kill unless the user confirms replacement.

## 7. Validation Plan

- tests to add:
  - lifecycle unit tests for register/idempotency/heartbeat/release/expiry/zero-client grace/work blocking/restart drain/shutdown drain/stale fencing/concurrent races;
  - HTTP contract tests for every endpoint, status code, typed error, and idempotency case;
  - runner work tests for active turn, child run, gate, scaffold, provider process cancellation;
  - runnerboot tests for detached spawn, build identity, stale classification, replacement, startup lock;
  - TUI tests for close choices, heartbeat/release, planned reconnect, unexpected loss;
  - Desktop tests for Electron lease ownership, dialogs, IPC, reconnect, status display;
  - supervisor tests for fenced restart/shutdown and no ghost respawn;
  - cross-client E2E tests for the full `SS-24 E-1..E-20` matrix.
- manual checks: `CP-81-Test-Steps` Windows live matrix.
- failure cases: stale token, stale generation, unknown process on port, wrong workspace, requester attribution, runner crash, Task Manager kill, terminal close, restart timeout, scaffold cancellation, provider subprocess cleanup.

## 8. Rollout and Fallback

- rollout order:
  1. `Task-413` docs/contract;
  2. `Task-414` lifecycle manager;
  3. `Task-415` runner API/work stop;
  4. `Task-416` boot/process identity;
  5. `Task-417` TUI;
  6. `Task-418` Desktop;
  7. `Task-419` supervisor;
  8. `Task-420` E2E/audit.
- feature flags/compatibility:
  - lifecycle API is additive; clients detect support via `/health`/`/system/lifecycle` fields;
  - `persistent` manual runner mode preserves existing operator behavior;
  - client-managed auto-exit should be gated during rollout until both TUI and Desktop lease support is shipped;
  - legacy client presence is bounded and observable in lifecycle status.
- fallback path:
  - if lease endpoints are unavailable, clients behave as `legacy_unknown` and must not claim safe idle shutdown;
  - if restart handoff is unavailable, return `restart_unavailable` rather than killing the runner silently;
  - if work inventory is uncertain, global shutdown response reports uncertainty and requires confirmation.
- monitoring:
  - `[runner] lifecycle` logs for register/release/expire/grace/drain/restart;
  - existing CA-911/CA-913 requester fields on every system action;
  - client-side status for `idle shutdown in Ns`, `Update pending`, `Runner restarting…`, `Runner stopped unexpectedly`.

## 9. Risks

- `R-1` Removing Job Object ownership before lease cleanup lands can leak runners → implement manager first and keep process spawn changes gated until lifecycle API is live.
- `R-2` Workload inventory misses active work → runner may idle-exit mid-work → inventory must cover turns, flow/agent loops, scaffolds, and provider executions; E2E proves no stuck `running`.
- `R-3` Old client compatibility can reintroduce leaks → bounded `legacy_presence` and explicit migration tests.
- `R-4` Restart handoff differs between `go run`, packaged binary, and supervisor → mode-specific handoff tests and fenced control records.
- `R-5` Confirmation token misuse can kill changed inventory → token bound to inventory revision and runner instance; stale token rejected.
- `R-6` UI dialogs diverge between window close and in-app controls → route all global actions through one lifecycle IPC/client path.

## 10. Definition of Done

- [ ] `SS-24`, `SD-28`, `CP-81`, `CP-81-Test-Steps`, and `Task-413..Task-420` exist and link correctly.
- [ ] `runner-lifecycle` exists in `FEATURE-KEYS.md`.
- [ ] Lifecycle manager unit tests cover all state/event and race cases.
- [ ] Runner exposes lease/lifecycle APIs, additive health identity, and fenced shutdown/restart.
- [ ] `LiveWorkSnapshot` + `StopAllForSystemAction` cover active turns, flow/agent loops, scaffold, and provider executions.
- [ ] TUI/Desktop close UX implements the three-choice contract and never global-shuts-down on ordinary close.
- [ ] Planned restart reconnects clients; unplanned runner loss closes clients with a notice.
- [ ] Windows client death does not kill a shared runner; last-client crash expires then idle-exits safely.
- [ ] Stale build auto-replaces only when idle; busy/legacy cases are explicit.
- [ ] Supervisor uses fenced lifecycle commands and leaves no `go run` wrapper/compiled-child orphans.
- [ ] All focused suites and full local-runner tests pass; provider-agnostic evidence documented.
- [ ] CA ledger entry/entries written; GitNexus `detect_changes` evidence or recorded unavailable fallback included.
