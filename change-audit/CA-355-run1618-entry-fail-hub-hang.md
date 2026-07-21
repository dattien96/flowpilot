# CA-355 — run-1618 A1: entry-node fail leaves hub hung

## Context

Live A1 (Review Loop, chat bug sub-mode) on `run-1618`:

- Hub: Grok `main` stayed **Running / active**
- Entry child: `coder` (Claude haiku) **failed** with  
  `no connected local account found for provider "claude"`
- UI: only “Spawned agent **coder**”; no recovery / hub reinvoke

Evidence: `.flowpilot/chats/run-1618-*.ndjson`,  
`.flowpilot/chats/db51ec26-…/dispatch.ndjson`,  
`.flowpilot/logs/features/agent-flow-engine/run-1618.ndjson`.

## Root cause (three cooperating gaps)

1. **flowStartOnly synthetic hub turn**  
   First turn with `FlowRef` suppresses the hub provider turn and emits synthetic  
   `EventTurnCompleted`. For `flowEngineDriven` roots, `emitLocked` stamps  
   `pendingFlowGateSettle=true` as if a real post-turn gate were owed.  
   No live `postTurnGateCancel` → hub forever `gate_in_progress` on any later  
   `startTurn` (same class as CA-354 stale settle, but at **entry**, not Continue).

2. **Non-cohort entry fail path** (`EventTurnFailed`, no `flowCohortId`)  
   Flow entry nodes spawn with `Wait=false`. On fail the old branch only appended  
   a `pendingAgentContext` note when `uiInitiated || !waitForResult`, and never:  
   - stamped the flow step **FAILED** (only cohort members did)  
   - cleared hub stale `pendingFlowGateSettle`  
   - reinvoked the hub  

   So coder failed while main stayed “active” waiting for a reinvoke that never ran.

3. **CP-51 dispatch (secondary)**  
   - Hub turn stuck at `send_claimed` (no `send_started` / terminal on synthetic path).  
   - Child `send_started` + `transport_error` effect, never `terminal_*` because  
     `finishTurn` emitted `EventTurnFailed` via `emitLocked` (not `turnBridge.Emit`),  
     so `CommitTerminalAndSettleIntent` never ran.

## Changes

### `interactive_service.go`

- **flowStartOnly**: after synthetic complete, clear `pendingFlowGateSettle` when  
  no live gate; linearize + `Terminal(completed)` for the synthetic handoff turn.
- **EventTurnFailed non-cohort**: for flow-engine parents — step FAILED, clear  
  stale hub settle, append fail note, `maybeAutoReinvokeHubWithNote` on explicit loop.
- **finishTurn**: after emit, `commitFinishTurnDispatchTerminal` so adapter-return  
  failures (and cancel) durable-terminalize when bridge did not.

### Tests

- `run1618_entry_fail_hub_hang_test.go`  
  - `TestNonCohortEntryFailSettlesStepAndClearsHubGateSettle`  
  - `TestFlowStartOnlyClearsSyntheticPendingGateSettle`

## Verify

```bash
cd apps/local-runner
go test ./internal/runner/ -count=1 -timeout 3m \
  -run 'TestNonCohortEntryFail|TestFlowStartOnlyClears|TestResumeFlowWithFeedbackClearsStaleHub'
```

Live retest A1: Review Loop with a **missing** child provider account → coder FAILED,  
hub reinvokes / reports failure (not perpetual main active). Retest happy path with  
connected Claude after that.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-51
change_type: bugfix
summary: Fix hub hang when flow entry child fails (run-1618 no Claude account)
# --->8---
