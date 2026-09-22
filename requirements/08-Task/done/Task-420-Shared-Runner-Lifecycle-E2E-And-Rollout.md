# Task-420: Shared Runner Lifecycle E2E And Rollout (CP-81 P-7)

## Metadata

- Document ID: `Task-420`
- Title: `Shared Runner Lifecycle E2E, Migration Hardening, And Audit`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-22`
- Last Updated: `2026-09-22`
- Parent Documents: [CP-81](../../07-Coding-Plan/done/CP-81-Shared-Runner-Lifecycle.md) `P-7`, [SS-24](../../05-System-Specs/SS-24-Shared-Runner-Lifecycle.md), [SD-28](../../06-System-Tech-Design/SD-28-Shared-Runner-Lifecycle.md)
- Child Documents: `None`
- Related Documents: `Task-414..Task-419`, [CP-81-Test-Steps](../../07-Coding-Plan/done/CP-81-Test-Steps.md), `BUG-240`, `BUG-328`, `CA-445`, `CA-474`, `CA-901`, `CA-911`, `CA-913`
- Replaces: `None`
- Tags: `runner-lifecycle, e2e, migration, windows, audit, verification`
- Feature Keys: `runner-lifecycle`

## AI Quick View

### Summary

- Prove the complete lifecycle matrix through real HTTP/process/client tests rather than isolated unit tests only.
- Harden migration paths for old clients/old runners, legacy presence, wrong workspace, unknown port process, stale binary, and restart timeout.
- Replace/supersede CA-474-era assertions with the new shared-client contract and write final CA evidence.

### Current Ask

- Close CP-81 only after the full `SS-24` state×event matrix is automated where possible and manually verified on Windows.

### Key Decisions

- `T-1` E2E coverage uses real local HTTP listeners/processes where practical; domain-only tests remain in `internal/lifecycle`.
- `T-2` Existing tests that encode the old client-owned shutdown contract are updated only with documented supersession, not silently weakened.
- `T-3` Provider parity is proven by shared lifecycle boundaries, not by duplicating provider-specific logic.
- `T-4` Every destructive event must leave a diagnosable log trail and no orphaned Go process.

### Constraints

- Do not mark CP-81 done with only unit tests.
- Do not claim PR/status without authoritative evidence.
- If GitNexus remains unavailable, record failed attempts and perform manual impact review before commit.
- Keep old health fields and backward-compatible behavior until both clients are lease-capable.

### Open Questions

- None; open implementation questions should have been resolved in `SD-28` or task-level notes.

### Source Refs

- `SS-24 AC-1..AC-16`, `BR-1..BR-9`, `E-1..E-20`.
- `SD-28 D-1..D-9`, §8 failure matrix, §11 validation.
- `CP-81-Test-Steps` manual matrix.

## 1. Goal

Prove that the shared-runner lifecycle works end-to-end across TUI, Desktop, supervisor, process ownership, active work, restart, stale replacement, and failure modes.

## 2. Parent Links

- coding plan: `CP-81 P-7`
- system spec: `SS-24`
- tech design: `SD-28`
- implementation slices: `Task-414..Task-419`

## 3. Trigger

The lifecycle bug was caused by interacting ownership systems. Individual package tests cannot prove that TUI exit, Desktop quit, Windows Job Object behavior, supervisor restart, and runner idle cleanup cooperate correctly.

## 4. Exact Change

- `T-1` Add cross-client E2E suite for lifecycle states:
  - register TUI + Desktop clients against one runner;
  - release one client and verify runner remains;
  - release both and verify `idle_grace` + exit;
  - expire a lease artificially and verify crash semantics;
  - keep runner alive while active work exists;
  - force shutdown cancels/terminalizes work;
  - planned restart changes `runnerInstanceId` and reconnects clients;
  - unexpected listener loss closes clients.
- `T-2` Add process-level Windows verification where feasible:
  - detached TUI-spawned runner survives TUI death while another lease exists;
  - last-client kill leads to lease expiry + idle shutdown;
  - supervisor shutdown removes `go.exe` wrapper and compiled child;
  - unknown port owner is never killed.
- `T-3` Migration tests:
  - old health shape still decodes;
  - legacy runner classified `legacy_unknown`;
  - legacy `X-Client` traffic produces bounded presence instead of instant kill;
  - stale generation/token/command cannot affect new runner generation.
- `T-4` UX status assertions:
  - TUI status shows shared clients/idle countdown/update pending/restarting/loss;
  - Desktop Runner Health shows equivalent fields;
  - both clients show identical confirmation inventory for global actions.
- `T-5` Audit closure:
  - write final CA note linking `CA-445`, `CA-474`, `CA-901`, `CA-911`, `CA-913`, `BUG-240`, `BUG-328`;
  - explicitly mark CA-474's concurrent-client assumption as superseded;
  - update upstream doc statuses if the repo workflow requires it.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/cp81_lifecycle_e2e_test.go` (new)
  - focused lifecycle/runnerboot/TUI/Desktop/supervisor test files from earlier tasks
  - `requirements/07-Coding-Plan/done/CP-81-Test-Steps.md` if manual matrix needs adjustment
  - `change-audit/CA-NNN-shared-runner-lifecycle.md`
- modules: verification layer across runner/TUI/Desktop/supervisor.
- routes: all lifecycle routes.
- tables: none.

## 6. Code Guide Signatures

```go
// internal/runner/cp81_lifecycle_e2e_test.go
func TestE2E_TUIAndDesktopShareRunner(t *testing.T)
func TestE2E_CloseOneClientKeepsRunner(t *testing.T)
func TestE2E_LastClientNormalCloseStartsIdleShutdown(t *testing.T)
func TestE2E_LastClientCrashExpiresLeaseThenIdleShutdown(t *testing.T)
func TestE2E_ClientAttachDuringIdleGraceCancelsShutdown(t *testing.T)
func TestE2E_ActiveWorkBlocksIdleShutdown(t *testing.T)
func TestE2E_ForceShutdownStopsRunsScaffoldAndClients(t *testing.T)
func TestE2E_PlannedRestartReconnectsBothClients(t *testing.T)
func TestE2E_UnplannedRunnerDeathClosesClients(t *testing.T)
func TestE2E_StaleRunnerAutoReplacesOnlyWhenIdle(t *testing.T)
func TestE2E_LegacyRunnerRequiresExplicitReplacement(t *testing.T)
func TestE2E_UnknownPortProcessIsNotKilled(t *testing.T)
func TestE2E_NoMCPChildCanShutdownRunner(t *testing.T)
```

## 7. Test Signatures

- `TestE2E_TUIAndDesktopShareRunner`
- `TestE2E_CloseOneClientKeepsRunner`
- `TestE2E_LastClientNormalCloseStartsIdleShutdown`
- `TestE2E_LastClientCrashExpiresLeaseThenIdleShutdown`
- `TestE2E_ClientAttachDuringIdleGraceCancelsShutdown`
- `TestE2E_ActiveWorkBlocksIdleShutdown`
- `TestE2E_ForceShutdownStopsRunsScaffoldAndClients`
- `TestE2E_PlannedRestartReconnectsBothClients`
- `TestE2E_ReconnectTimeoutClosesClient`
- `TestE2E_UnplannedRunnerDeathClosesClients`
- `TestE2E_StaleRunnerAutoReplacesOnlyWhenIdle`
- `TestE2E_StaleRunnerBusyReportsUpdatePending`
- `TestE2E_LegacyRunnerRequiresExplicitReplacement`
- `TestE2E_WorkspaceMismatchRejected`
- `TestE2E_UnknownPortProcessIsNotKilled`
- `TestE2E_SupervisorStaleCommandCannotKillNewGeneration`
- `TestE2E_NoMCPChildCanShutdownRunner`
- `TestE2E_NoGoProcessOrCompiledRunnerOrphaned`

## 8. Acceptance Check

- All focused tests green:
  - `go test -count=1 ./internal/lifecycle/... ./internal/cli/... ./internal/runner/... ./internal/tui/...`
  - `go test ./...` under `apps/local-runner`
  - Desktop typecheck/build/focused tests
  - supervisor integration checks
- `CP-81-Test-Steps` manual matrix completed on Windows.
- Logs prove requester PID/process name, lifecycle reason, lease count, workload inventory, restart ID, and final exit outcome.
- GitNexus `detect_changes` run before final commit, or unavailable attempt recorded.

## 9. Out of Scope

- New provider features.
- Admin Web lifecycle UI.
- Remote/multi-machine lease coordination.

## 10. Definition of Done

- [x] All `SS-24 AC-1..AC-16` have automated or documented manual evidence.
- [x] `SS-24 E-1..E-20` are covered or explicitly explained.
- [x] Old CA-474 contract supersession is documented in code tests and CA note.
- [x] No orphaned runner/binary remains in the Windows matrix.
- [x] Final CA ledger written under `feature_key: runner-lifecycle`.
- [x] `CP-81` and child task statuses updated according to repository workflow.
