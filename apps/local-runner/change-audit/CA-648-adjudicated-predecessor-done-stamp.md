# CA-648 — Adjudicated predecessor stamped DONE on advance (BUG-1197)

## Evidence

Live run-225691 (`flow|adjudicated-node-never-stamped-done`): the hub
adjudicated `tdd` approved; the engine then advanced `coder → DONE → validate
→ DONE → spec_align → DONE`, but the `tdd` step row stayed `RUNNING` forever —
a mid-spine RUNNING badge while three downstream nodes completed.

`tryAdvanceFlowFromNode` stamps `completedNodeID` DONE and dispatches its
successors, but never reconciles strict done-edge *predecessors*. A
predecessor's leg can go terminally Completed while the settle that would
have stamped its step is consumed by a gate divert, a debate stash, or a
stale-completion claim — leaving the node RUNNING with nobody left to stamp
it.

## Fix

`settleAdjudicatedDonePredecessors` (flow_executor.go) runs inside
`tryAdvanceFlowFromNode` once flow-engine-driven topology is confirmed —
before any consume branch (negotiation hub, terminal done, inline, cohort
spawn):

- walk `flowForwardDonePredecessors(edges, completedNodeID)`
- skip already-terminal rows (DONE/SKIPPED/FAILED/CANCELED)
- require `persistedCompletedChildExists(parentRunID, pred)` — durable
  evidence the leg's outcome was adjudicated (fail-closed: no completed leg
  → no stamp; `markForwardDonePredecessorsSkipped` owns never-ran PENDING)
- require `!vibeNodeHasLiveWork(parentRunID, pred)` — round re-entry
  (`synthesis_negotiation → tdd` on continue) reuses node ids; a prior
  leg's Completed session must not stamp over a live current leg
- stamp DONE + `flow_advance_settles_adjudicated_predecessor` diag

`flowDriven` was hoisted above the negotiation-hub early return so the
settle covers every consume branch, and reused at the existing stamp site.

## Tests

`bug1197_adjudicated_predecessor_stamp_test.go`:

- `TestBug1197_AdvanceSettlesAdjudicatedPredecessor` — red pre-fix: `tdd`
  stayed RUNNING after `coder` advanced with a durably-Completed `tdd` leg;
  green post-fix (DONE).
- `TestBug1197_AdvanceLeavesUnfinishedPredecessor` — guard: a `tdd` leg
  still in flight (live child + prior-round Completed session) stays
  RUNNING.

Regression: `TestBug(1194–1197|174|234|318|353|363–365|520|539|585|616|618|622)`
+ Advance/Settle/Cohort/Resume families — green except 2 environmental
failures (`codex`/`agy` provider binaries absent from PATH, pre-existing).
