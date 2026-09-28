# CA-643 — CP-89 Task-452: `forwardFlow` Turn Seam

**Date**: 2026-09-28
**Author**: Devin
**Ticket**: CP-89 / Task-452

---

## Problem

`flowArm=pending` (Task-451) pins a flow without starting it. CP-89
requires an explicit, deterministic signal — never model inference or
prompt wording — to flip the latch and launch the pinned flow. Before
this change no such signal existed: any first-turn flowRef fired the
immediate path, and a bare `{"forwardFlow":true}` body was rejected by
BUG-509's empty-body guard.

## Change

- `TurnInput.ForwardFlow` + `turnBody.forwardFlow`; the admission guard
  counts `forwardFlow` as actionable content so a bare forward passes.
- Dedicated forward path in `startTurn` BEFORE the ordinary
  `turnCount==0` flow-start block — `resolveWorkflowFlowRef`'s
  `turnCount!=0` guard untouched; the immediate auto-start block is now
  gated on `rs.flowArm == FlowArmImmediate`, so a pending run can never
  start on a plain turn even if a flowRef arrives.
- `resolvePinnedFlowRefForForward`: pin resolution order —
  `rs.chatFlowRef` → `rs.workflowID` mount → 422
  `forward_requires_flow_pin`.
- `runFirstTurnFences`: re-runs the first-turn fences at forward time —
  working-mode family gate (`FlowAllowedForWorkingMode`), vibe-cp-ingest
  source contract (create-pinned `SourceDocID` counts; turn value wins),
  flow-definition validity (`invalid_flow_definition` via the resolver,
  read directly under `s.mu` — `explicitFlowRefResolves` self-locks and
  would deadlock).
- `forwardPinnedFlow`: fence failure returns BEFORE the latch flip —
  pending is never consumed by a failed forward (user fixes and retries,
  `turnInFlight` released). Success flips `pending→started` under `s.mu`
  + sets `flowEngineDriven` + `yolo` + vibe start markers (the forward IS
  the arm moment a pending run deferred at create), then the caller
  spawns the entry through the existing `startResolvedFlow` +
  `flowStartOnly` path.
- `forwardFlow` on `immediate`/`started` → 422 `flow_already_started`;
  double-forward cannot spawn a second entry.

## Files

- `internal/runner/provider_event.go` — `TurnInput.ForwardFlow`,
  `StartRunInput.SourceDocID`.
- `internal/runner/interactive_handlers.go` — `turnBody.forwardFlow`,
  admission, `TurnInput` wiring.
- `internal/runner/interactive_service.go` — forward seam in `startTurn`,
  `forwardPinnedFlow`, `resolvePinnedFlowRefForForward`,
  `runFirstTurnFences`, immediate-gate on the first-turn block.
- `internal/runner/task452_forwardflow_test.go` — 10 additive tests.

## Test evidence

- `TestTask452_*` ×10 green: bare-forward admission, forward-starts-from-
  pin, no-pin 422, started 422, immediate 422, failed-fence-keeps-pending,
  corrupt-def fails closed, workflowID pin resolves, turnCount guard
  unchanged, pinned SourceDocID satisfies ingest fence.
- `-race` on `TestTask45[123]` green.

## Risk / blast radius

- `forwardFlow` is a new opt-in field; turns without it hit byte-
  identical paths. The `rs.flowArm == FlowArmImmediate` gate on the
  first-turn block is true for every pre-CP-89 run (empty arm resolves
  immediate), so BUG-261/BUG-315/run-63960 protections are unchanged.

## Addendum — BUG-399 admission fence vs pending (found by live L-7)

The pre-existing BUG-399 admission fence fired on `turnCount==0` whenever
the adopted `in.FlowRef` resolved to vibe-cp-ingest — including the first
PLAIN chat turn of a `flowArm=pending` run, which is not a flow start.
It also read `in.SourceDocID` only, ignoring the create-pinned
`rs.sourceDocID`. Effect: pending vibe-cp-ingest chat was dead on turn 1
(422 `invalid_cp_source`) and pinned-source forwards could never reach
`runFirstTurnFences`. Live-verified 422s before the fix.

Fix: the admission fence now requires `rs.flowArm == FlowArmImmediate` —
pending runs keep chat plain; the ingest contract is enforced at forward
time by `runFirstTurnFences` (which does honor the create pin). Covered
by `TestTask452_PendingVibeChatTurnNotIngestFenced`.
