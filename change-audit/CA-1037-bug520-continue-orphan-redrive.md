# CA-1037 — BUG-520 Continue re-drives parked orphan child; cap-5 reset asserted through the real edge

Date: 2026-09-27 — second review round (grok pass over a27a3954): two tests
did not reach through the entry point they claimed to pin, and one
production gap was exposed by fixing them.

## Production change — `interactive_service.go` (`resumeFlowWithFeedback`)

BUG-520 residual (a), run-60145 shape: a flow child parked
`waiting_user_approval` by `parkFlowForAwaitingUser` has its armed
gate-reprompt wiped, but retains `pendingGateCodePaths` — the durable
re-check seed. When Continue arrives with no `failedDelegateNodeID` and no
audit writer to target, the generic hub reinvoke left that orphan parked
forever while its gate debt was still owed. Continue now scans the parent's
children for exactly this shape — `waiting_user_approval`, no pending
approval/question card of its own (those answer a user decision, not the
park), `pendingGateCodePaths` non-empty — and re-drives each orphan through
`reinvokeMatchingFlowChild` + step RUNNING before falling through to the
hub reinvoke. The revived turn's post-turn gate force-re-checks the held
paths and re-arms or clears the reprompt.

## Test changes (additive)

`bug520_stall_gate_reprompt_child_test.go`:

- `TestBug520_ContinueReDrivesParkedOrphanChild` — replaces the direct
  `reinvokeMatchingFlowChild` call with the real entry: POST
  `/client/workflow-runs/{id}/agent-loop/continue` on a parent whose child
  is parked `waiting_user_approval`, reprompt wiped, `pendingGateCodePaths`
  retained, and no `failedDelegateNodeID` set. Asserts the child leaves the
  parked state and its reinvoked turn reaches the provider adapter (the
  child inherits the parent's `providerAccountID` stamp, matching real
  spawn behavior — without it `startTurn` rejects "active provider account
  changed since the run started").

`task_harness_cap5_round_reset_test.go`:

- `TestTaskHarnessCap5ContinueThenBlockedThenPlanApproveResets` — the
  reset half no longer calls `resetPlanPhaseRound` directly. A clean plan
  run (run-201295 topology: `plan_synthesis --done-->
  preflight_contract_freeze --done--> agent.code writer`, real git
  workspace, valid preflight draft in the verdict summary) drives
  `turnBridge.SubmitFlowControl{Status:"done"}` — the same seam the hub's
  `submit_review_outcome` tool routes through — into
  `advanceHubDoneThroughEdge`. Asserts `preflight_contract_freeze` reaches
  DONE and `LoopState.Round` resets to 0. (A churned plan parks on
  `plan_approval` first by Task-325 design, so the approve edge is
  exercised on the non-churned shape where dispatch proceeds.)

`flowref_create_first_turn_test.go`:

- `TestFlowRefAtRunCreateCpIngestRejectsMissingSource` — creation-time
  `flowRef=vibe-cp-ingest` (workingMode vibe, X-Client tui): the first turn
  carries no flowRef and no CP-shaped source, so `resolveWorkflowFlowRef`
  copies `chatFlowRef` into `TurnInput.FlowRef` and
  `validateVibeCpIngestSource` (BUG-399) must reject 422
  `invalid_cp_source` — with zero provider sends.

## Verification

```
go test -count=1 -run 'TestTaskHarnessCap5|TestFlowRefAtRunCreate|TestBug520' \
  -v ./internal/runner/
```

All green. Regression run over Resume/Continue/Reinvoke/Park/PlanApproval/
GateReprompt/Stall patterns: only pre-existing environment failures
(`codex`/`agy` binaries absent from PATH) — same baseline as recorded in
CA-1036.
