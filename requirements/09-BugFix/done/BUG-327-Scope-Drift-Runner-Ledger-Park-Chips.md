# BUG-327: Scope Drift Runner Ledger Exemption and Park Action Chips

## Metadata

- Document ID: `BUG-327`
- Title: `Scope drift runner ledger exemption and park action chips`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot maintainers`
- Created: `2026-08-24`
- Last Updated: `2026-08-24`
- Parent Documents: `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`, `requirements/06-System-Tech-Design/SD-06-AI-Provider-Integration.md`
- Child Documents: `none`
- Related Documents: `change-audit/CA-627-scope-drift-runner-ledger-exemption-and-park-chips.md`
- Replaces: `none`
- Tags: `local-runner, change-contract, cli-tui, scope-drift, park-state, chips`

## AI Quick View

### Summary

- In `run-218125` (Rag Harness with Grok 4.5), the frozen contract declared `format.go` and `format_test.go`. The runner automatically updated `.flowpilot/ledger/chat_summary.ndjson` and `feature_history.ndjson`, which the gate diff treated as scope drift and escalated to `WAITING_USER_APPROVAL`.
- When escalated, `finishTurn` post-gate settle kept child status as `running` and `turnStream` active. The TUI's `flowLoopBlocked()` returned `false` because `hasLiveWorkingChild()` did not skip the main run row and checked `turnStream != nil`, keeping the Thinking timer running (`Thinking 4m 08s`) and hiding `[Continue]` `[Stop]` chips.

### Key Decisions

- `F-1`: Add exact-path exemption in `frozen_scope.go` and `gate_hook.go` for runner ledger bookkeeping files (`.flowpilot/ledger/chat_summary.ndjson`, `.flowpilot/ledger/feature_history.ndjson`), without blanket exempting `.flowpilot/**` or `settings/flow-rules.json`.
- `F-2`: On park/escalate, transition child runs to `waiting_user_approval` preserving existing summary fields (`ActivationSeq`, `ProviderKey`, `ModelName`, `WaitForResult`, `DependsOn`).
- `F-3`: Update TUI `hasLiveWorkingChild` to skip main run, `flowLoopBlocked` to not block on `turnStream != nil`, and `focusedChildLive` to treat `waiting_user_approval` as parked.

## 1. Issue Summary

During flow runs against a frozen contract, runner ledger updates caused false-positive scope drift escalations. Once escalated, missing park state cleanup in the runner and TUI caused the UI to hide action chips and keep the Thinking timer active.

## 2. Root Cause

1. Gate scope diff did not exclude runner internal ledger bookkeeping files.
2. Park handlers reset child status to running on gate settle and did not update the orchestrator summary cleanly.
3. TUI liveness predicates treated main agent and open turn streams as live work even when the flow was parked.

## 3. Resolution

1. Added `RunnerLedgerBookkeepingPaths` and `IsRunnerLedgerBookkeepingPath` in `frozen_scope.go` and checked in `gate_hook.go`.
2. Cleaned up park handlers in `interactive_service.go` to preserve summary metadata and set `waiting_user_approval`.
3. Updated `agents_focus.go` and `step_runtime.go` in TUI to properly recognize parked states and render `[Continue] [Stop]` chips across Claude, Codex, and Grok.
