# CA-1087: vibe-tasks sprint handoff raced — stale hub "done" resolved against swapped topology, audit ran with empty validation

Date: 2026-10-01
Refs: user-reported live log — `Audit blocked: validation was not
positively verified (status=blocked_validation_failed, validation=)` on
vibe-tasks run-2302. Expected: task_plan_reader hands off to the sprint
chain deterministically; audit only ever runs after validate.

## Symptom (live run-2302, 15:28–15:29)

- `task_plan_reader` (run-3190) completed, reporting 5 CP-01-scoped Tasks.
- `contract-planner` (run-3339, grok-4.7) spawned at 15:28:56, reached a
  tool-permission `pending_interaction`, then received `session/cancel`
  at 15:29:07 (`MidTurnAbort`, `rejectionNote: "interrupted by user"`).
- Step timeline: `preflight_contract_plan` FAILED, `synthesis` DONE at
  15:29:07.183 (7 ms after the planner failure), `context`/`tdd`/`coder`/
  `validate` all PENDING — then the audit node ran with `validation=""`
  and escalated `blocked_validation_failed` → `flow_awaiting_user` card.

## Root cause — two stacked defects

1. **The chain handoff was not claimed.** `tryAdvanceFlowFromNode` calls
   `onVibeCpNodeDone` *before* reading the topology — which arms the sprint
   plan and `startResolvedFlow(vibe-sprint)` swaps `activeFlowNodes`. Back
   in the caller, `forwardDoneTargets(sprintEdges, "task_plan_reader")` is
   legitimately empty (the node does not exist in the new topology), so it
   returned `false` → `advanceOrNotifyHub` fell back to the note +
   `maybeAutoReinvokeHub` path → a generic "synthesize the outcome" hub
   turn was scheduled for a transition that no longer exists. (The
   `vibeHubSealed` suppression could not fire — it resolves the hub against
   the *new* topology, where `synthesis` is not a validator.)

2. **A stale hub `done` resolved against a never-dispatched node.** The
   reinvoked hub submitted `submit_review_outcome(done)` →
   `advanceHubDoneThroughEdge` → `activeHubNodeID` was `""` (cleared at the
   cp_lock seal; only `dispatchHubNotifyNode` ever re-stamps it) → fallback
   `hubInlineNodeID(activeFlowNodes)` picked `synthesis`, the sprint
   topology's first `hub.inline` — still PENDING, never dispatched → its
   `--done--> audit` edge dispatched `audit` with zero validation →
   escalate → `parkFlowForAwaitingUser` → `session/cancel` killed the
   contract-planner mid-turn. The audit's `blocked_validation_failed`
   verdict was correct fail-closed behavior — the dispatch to it was not.

   Note: the pre-existing `hubID == vibeSprintSlicerNodeID ||
   vibeTaskSlicerNodeID || vibeTaskPlanReaderNodeID` special case in
   `advanceHubDoneThroughEdge` was unreachable — neither `activeHubNodeID`
   nor `hubInlineNodeID` can ever hold a delegate node id (the CA-783 test
   for it set `activeHubNodeID` manually).

## Fix (`internal/runner` only)

- `flow_executor.go` `tryAdvanceFlowFromNode`: when the completed node is
  a vibe chain-handoff node (`vibeChainHandoffNode`) that is absent from
  the now-active topology, claim the completion (`true`) instead of
  falling back to the hub reinvoke — the handoff already dispatched the
  next chain deterministically. Same claim set also extended to
  `sprint_slicer`/`task_plan_reader` in the existing terminal-done claim
  (CP-90 added the reader but missed that list — the un-swapped fallback
  could re-park cp_lock, the run-214743 class).
- `vibe_cp.go`: new `vibeChainHandoffNode` — the node set whose completion
  `onVibeCpNodeDone` consumes into a topology-swapping dispatch
  (`cp_writer`, `debate_synthesis`, `task_slicer`, `sprint_slicer`,
  `task_plan_reader`).
- `interactive_service.go` `advanceHubDoneThroughEdge`: safety net for any
  residual stale source (deferred `pendingHubReinvoke`, restart replay).
  An agent-initiated `done` resolving via the fallback (`activeHubNodeID`
  empty) onto a hub node whose step is not RUNNING/WAITING_USER_APPROVAL
  is unattributable — every legitimate reach stamps the hub RUNNING before
  its turn. Such outcomes are consumed as `done`/`advancing` (one-decision
  guard stamped) instead of dispatching a successor the run never reached.
  Operator settles (`agentInitiated=false`) and missing step rows (`""`,
  legacy/unmirrored shapes) keep legacy behavior.

## Tests

`ca1087_sprint_handoff_stale_done_test.go` (reproduce-first red → green;
the red run reproduced the live shape exactly —
`handled=true {Status:blocked ... NextAction:awaiting_user}` with
`synthesis` stamped DONE while `audit` escalated):

- `TestCA1087_TaskPlanReaderCompletionConsumedByChainIsClaimed`: the
  reader completion returns claimed, plan armed (2 CP-02 tasks), topology
  swapped, no `pendingHubReinvoke` armed.
- `TestCA1087_StaleHubDoneOnUndispatchedSprintHubDoesNotDispatchAudit`:
  an agent `done` on PENDING fallback `synthesis` is consumed as
  `advancing`; `synthesis` stays PENDING, `audit` never dispatched, loop
  stays running.
- `TestCA1087_LegitDispatchedSynthesisDoneStillAdvances`: a real
  `dispatchHubNotifyNode`-dispatched `synthesis` (RUNNING +
  `activeHubNodeID`) still resolves its own `--done--> audit` edge.

Existing coverage unchanged and green: `TestVibeTasks_*` (CP-90 chain),
`TestCodePhaseHubDoesNotResetRound` (operator-shape `done`, agentInitiated
unset), all `advanceHubDoneThroughEdge` verdict/park tests, `go vet` clean.
Full `./internal/runner` suite deltas vs the recorded baseline noted in
the commit body.
