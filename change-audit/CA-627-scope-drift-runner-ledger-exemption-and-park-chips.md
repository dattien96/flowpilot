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
  - apps/local-runner/internal/tui/app/step_runtime.go
  - apps/local-runner/internal/tui/app/bug327_blocked_waiting_turnstream_chips_test.go
```
<!-- /flowpilot:change-ledger -->

## Context & Problem (BUG-327, run-218125)

1. In `run-218125` (Rag Harness with Grok 4.5), during the `implement` node execution, the frozen contract declared `format.go` and `format_test.go`. The runner automatically updated its internal ledger files (`.flowpilot/ledger/chat_summary.ndjson` and `feature_history.ndjson`). Because CA-427 intentionally did not blanket-exempt `.flowpilot/**` to prevent tampering with `settings/flow-rules.json`, the gate diff treated runner ledger writes as scope drift and escalated to `WAITING_USER_APPROVAL`.
2. When the flow was escalated to `blocked` (awaiting user), `finishTurn` post-gate settle kept child status as `running`, leaving `turnStream` active. The TUI's `flowLoopBlocked()` returned `false` because `hasLiveWorkingChild()` did not filter out the main run row and checked `turnStream != nil`, keeping the Thinking timer running (`Thinking 4m 08s`) and hiding the `[Continue]` `[Stop]` action chips.

## Changes Made

1. **F-1: Exact-Path Runner Ledger Exemption (`frozen_scope.go`, `gate_hook.go`)**:
   - Added `RunnerLedgerBookkeepingPaths` and `IsRunnerLedgerBookkeepingPath` matching `.flowpilot/ledger/chat_summary.ndjson` and `.flowpilot/ledger/feature_history.ndjson`.
   - Updated `gate_hook.go` to exclude runner ledger bookkeeping files from the written paths compared against `FrozenContractScopeDrift`.
   - Preserves security: does NOT blanket exempt `.flowpilot/**` or `settings/flow-rules.json`.
2. **F-2: Park State & Action Chips Cleanup (`interactive_service.go`, `agents_focus.go`, `step_runtime.go`)**:
   - In `parkFlowForAwaitingUser` and `parkFlowForAwaitingUserLocked`: child runs transitioned to `RunStatusWaitingUserApr` / `waiting_user_approval` and recorded in `agentOrchestrator`.
   - In `interactive_service.go` (`finishTurn` post-gate settle): do not reset child status to `running` if parent loop is `blocked`.
   - In `agents_focus.go`: `hasLiveWorkingChild()` skips `mainRunID()` and synthetic main runs so it only evaluates real child agent liveness.
   - In `step_runtime.go`: `flowLoopBlocked()` no longer blocks on `turnStream != nil` when no child is working.

## Validation

- `go test -count=1 ./internal/changecontract/...`: PASSED.
- `go test -count=1 ./internal/runner/ -run="TestBUG327_|TestContractFreeze_|TestFrozen"`: PASSED.
- `go test -count=1 ./internal/tui/app/ -run="TestBUG327_|TestTimelineActionChips_|TestTUIInput_|TestBlockedBar_|TestCA622_|TestRun136749_"`: PASSED across `claude`, `codex`, and `grok`.
