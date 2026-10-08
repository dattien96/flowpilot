# BUG-654 — `agent-loop/continue` on a run parked at a routing node re-dispatches the PARKED hub turn (burning a round to re-submit the same outcome and re-block), while only `flow-control` can traverse the intended edge — same verb, two semantics

- **ID:** BUG-654
- **Severity:** Medium-High — the operator-visible "continue" surface
  does the wrong thing exactly when the run needs routing; burns quota
  and re-parks in the same shape.
- **Status:** FIXED — CA-1236 (2026-10-08): routing park continue routes through applyFlowControl edge traversal instead of re-dispatching the parked hub node

## Evidence chain (all live)

1. run-523131 sprint-4 `synthesis` park: `POST /agent-loop/continue`
   re-invoked the synthesis hub → it re-submitted the same
   `submit_review_outcome` → gate re-blocked "missing verdict" →
   re-park. Round 3 burned, zero progress.
2. `POST /flow-control {status:"continue"}` on the SAME park fired the
   `synthesis → coder` back-edge and dispatched a real coder leg — the
   designed route for this state.
3. From the operator seat there is no signal distinguishing "turn was
   interrupted, resume it" from "node awaits routing, advance the edge".

## Root cause (hypothesis)

`handleAgentLoopContinue` re-drives the run's last turn irrespective of
node type; for a flow node parked on a gate/hub outcome the meaningful
action is edge traversal, which lives behind `flow-control` only.

## Fix direction

- `F-1` `agent-loop/continue` on a flow-parked run should resolve to the
  node's pending edge decision (or surface a card) — re-running the hub
  outcome is never useful once already submitted.
- `F-2` If keep both routes: document and surface which verb applies in
  the park card payload (`recoveryHint`), so automation picks
  `flow-control` for routing parks.

## Regression coverage

- `TestBug654_ContinueOnRoutingParkAdvancesEdge` — parked synthesis +
  continue → edge fires, no hub re-dispatch.
- `TestBug654_ContinueOnInterruptedTurnResumes` — non-flow interrupt
  still resumes the turn (no regression of the true resume path).
