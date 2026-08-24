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

- In `run-218125` and `run-221516` (Rag Harness with Grok 4.5), the frozen contract declared `format.go` and `format_test.go`. The runner automatically updated `.flowpilot/manifest.json`, `.flowpilot/ledger/chat_summary.ndjson`, and the coder created `change-audit/CA-*.md`. The gate diff treated these as scope drift and escalated to `WAITING_USER_APPROVAL`.
- When escalated, scope-drift escalate did not stamp `parent.lastEscalatedInlineNodeID = "implement"`, causing Continue to reinvoke the hub instead of the coder.
- On flow completion, `flowLoopDone` and `workIsLive` continued showing `Thinking` because `flowHasActiveAgents` did not skip the main agent or parked runs.

### Key Decisions

- `F-1`: Add exact-path exemption in `frozen_scope.go` and `gate_hook.go` for runner ledger bookkeeping files (`.flowpilot/manifest.json`, `.flowpilot/ledger/chat_summary.ndjson`, `.flowpilot/ledger/feature_history.ndjson`) and change-audit notes (`change-audit/*.md`, per BUG-278).
- `F-2`: Stamp `parent.lastEscalatedInlineNodeID` on child gate blocks (scope drift, missing contract, diff observation error) so Continue reliably resumes the escalated node.
- `F-3`: On park/escalate, transition child runs to `waiting_user_approval` preserving existing summary fields (`ActivationSeq`, `ProviderKey`, `ModelName`, `WaitForResult`, `DependsOn`).
- `F-4`: Update TUI `flowLoopDone`, `hasLiveWorkingChild`, and `workIsLive` to immediately settle statusline to `done` and stop Thinking spinner when the flow finishes.

## 1. Issue Summary

During flow runs against a frozen contract, runner bookkeeping and audit note updates caused false-positive scope drift escalations. Once escalated, Continue reinvoked the wrong node, and upon completion the TUI statusline kept Thinking active.

## 2. Root Cause

1. Gate scope diff did not exclude runner internal ledger bookkeeping files or `change-audit/*.md`.
2. Child gate escalations did not set `lastEscalatedInlineNodeID`, causing `/continue` to default to the hub.
3. TUI liveness predicates treated main agent and open turn streams as live work even when the flow loop was done or parked.

## 3. Resolution

1. Added `RunnerLedgerBookkeepingPaths`, `IsRunnerLedgerBookkeepingPath`, and `IsChangeAuditPath` in `frozen_scope.go` and checked in `gate_hook.go`. `IsChangeAuditPath` matches only `change-audit/CA-*.md` (flat) — `FEATURE-KEYS.md` and nested notes remain in scope.
2. Stamped `lastEscalatedInlineNodeID` in `gate_hook.go` on all child gate escalations, and routed Continue for WRITER/delegate nodes (agent.code, e.g. implement scope-drift) to `reinvokeMatchingFlowChild` (retry the child) instead of the no-op `tryAdvanceFlowThroughInline` / hub->done skip. validate/audit still run.
3. Cleaned up park handlers in `interactive_service.go` to preserve summary metadata, set `waiting_user_approval` (status + agentStatus), and keep the gate-block settle as a park when the loop is blocked.
4. Updated `agents_focus.go`, `step_runtime.go`, and `app.go` in TUI to settle chrome on done and stop Thinking timer when parked or completed; fixed the `TestWorkIsLive_Matrix` active-agent regression from the empty-RunID skip.
