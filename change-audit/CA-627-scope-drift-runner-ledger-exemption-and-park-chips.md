# CA-627: Runner Ledger Exact-Path Scope Drift Exemption and Park Thinking Cleanup

<!-- flowpilot:change-ledger -->
```yaml
schema_version: 1
change_id: CA-627
feature_key: change-contract
parent_change_id: CA-626
date: 2026-08-24
author: antigravity
summary: Exact-file scope drift exemption for runner ledgers and unblock Continue/Stop chips when parked
intent: Prevent false-positive scope drift escalation caused by runner ledger writes and ensure [Continue] [Stop] chips render with Thinking timer stopped during park
declared_paths:
  - apps/local-runner/internal/changecontract/frozen_scope.go
  - apps/local-runner/internal/changecontract/frozen_scope_test.go
  - apps/local-runner/internal/runner/gate_hook.go
  - apps/local-runner/internal/runner/interactive_service.go
  - apps/local-runner/internal/runner/provider_event.go
  - apps/local-runner/internal/runner/bug327_scope_drift_ledger_exemption_test.go
  - apps/local-runner/internal/tui/app/agents_focus.go
  - apps/local-runner/internal/tui/app/app.go
  - apps/local-runner/internal/tui/app/step_runtime.go
  - apps/local-runner/internal/tui/app/bug327_blocked_waiting_turnstream_chips_test.go
  - requirements/09-BugFix/done/BUG-327-Scope-Drift-Runner-Ledger-Park-Chips.md
```
<!-- /flowpilot:change-ledger -->

## Context & Problem (BUG-327, run-218125, run-221516)

1. In `run-218125` and `run-221516` (Rag Harness with Grok 4.5), during the `implement` node execution, the frozen contract declared `format.go` and `format_test.go`. The runner automatically updated `.flowpilot/manifest.json`, `.flowpilot/ledger/chat_summary.ndjson`, and `feature_history.ndjson`, and the coder wrote `change-audit/CA-*.md`. The gate diff treated these as scope drift and escalated to `WAITING_USER_APPROVAL`.
2. When the flow was escalated to `blocked` (awaiting user), scope-drift escalate did not stamp `parent.lastEscalatedInlineNodeID = "implement"`, causing Continue to reinvoke the hub instead of the coder.
3. On flow completion, `flowLoopDone` and `workIsLive` continued showing `Thinking` because `flowHasActiveAgents` did not skip the main agent or parked runs.

## Changes Made

1. **F-1: Exact-Path Runner Bookkeeping & Change-Audit Exemption (`frozen_scope.go`, `gate_hook.go`)**:
   - Added `path.Join(".flowpilot", "manifest.json")` to `RunnerLedgerBookkeepingPaths`.
   - Added `IsChangeAuditPath` for `change-audit/*.md` notes allowed per BUG-278.
   - Updated `gate_hook.go` to exclude runner bookkeeping and change-audit notes from scope drift checks.
2. **F-2: Escalated Node Stamping on Gate Blocks (`gate_hook.go`)**:
   - Stamped `parent.lastEscalatedInlineNodeID = coderStepID` on child gate escalations so Continue reliably resumes the escalated node.
3. **F-3: Park State & Action Chips Cleanup (`interactive_service.go`, `agents_focus.go`, `step_runtime.go`)**:
   - In `parkFlowForAwaitingUser` and `parkFlowForAwaitingUserLocked`: child runs transitioned to `RunStatusWaitingUserApr` / `waiting_user_approval` and updated in `agentOrchestrator` preserving existing summary fields (`ActivationSeq`, `ProviderKey`, `ModelName`, `WaitForResult`, `DependsOn`).
   - Park hygiene: cleared `pendingFlowGateTurnID` and `pendingGateChangedFiles` consistently across park handlers and `finishTurn` post-gate settle.
   - In `interactive_service.go` (`finishTurn` post-gate settle): do not reset child status to `running` if parent loop is `blocked`.
   - In `agents_focus.go`: `hasLiveWorkingChild()` skips `mainRunID()` and synthetic main runs so it only evaluates real child agent liveness; `focusedChildLive()` treats `waiting_user_approval` as parked.
4. **F-4: Flow Done Settle & Thinking Cleanup (`step_runtime.go`, `app.go`)**:
   - Updated `flowLoopDone` to use `hasLiveWorkingChild` and `applyAgentGraph` to settle chrome on done.
   - Updated `workIsLive` in `app.go` to stop spinner immediately when `flowLoopDone` is true.

## Validation

- `go test -count=1 ./internal/changecontract/...`: PASSED.
- `go test -count=1 ./internal/runner/ -run="TestBUG327_|TestContractFreeze_|TestFrozen"`: PASSED.
- `go test -count=1 ./internal/tui/app/ -run="TestBUG327_|TestTimelineActionChips_|TestTUIInput_|TestBlockedBar_|TestCA622_|TestRun136749_"`: PASSED across `claude`, `codex`, and `grok`.

