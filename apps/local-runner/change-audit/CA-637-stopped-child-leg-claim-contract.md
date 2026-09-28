# CA-637 — Cancelled/Stopped Child Leg-Claim Contract Pinned (BUG-543 assessment)

**Date**: 2026-09-28
**Author**: Devin
**Ticket**: BUG-543

---

## Problem

Live residue on `run-18660` (completed + `leg_state=active`), `run-23156`
and `run-28790` (park-cancelled + active) suggested a leg-close leak: a
settled child retaining its provider/worktree claim.

## Assessment — no production change

Closing the leg at child settle or at park/stop is **wrong**: the claim is
the child's re-drivable pin. `reinvokeMatchingFlowChild` re-drives
label-matched cancelled children, `resumeAgentLoop` revives stopped loops,
and `extend-cap` resumes cap-stopped loops — all need the live leg. A broad
close was tried and reverted after ~56 suite failures proved the contract.

Terminal ownership already transfers through the four seams that know the
member is done for good: `reconcileChildRunsOnFlowDone` (`flow_done`),
member-action skip (`member_skipped`), dispatch failure
(`dispatch_failed`), candidate sweep (`worktree_swept`).

run-18660's residue was a BUG-542 symptom (parent wedged before `done`, so
the reconcile never ran) — not an independent bug.

## Change

Additive contract-pinning tests only
(`internal/runner/bug543_stopped_flow_child_leg_test.go`):

- `TestBug543_StopKeepsChildLegClaim` — Stop terminalizes children to
  `cancelled`; leg claims must survive for resume/reinvoke.
- `TestBug543_FlowDoneReconcileClosesLegs` — the flow-done reconcile is the
  generic close seam (`completed` + `flow_done`).

## Residual

Orphaned cancelled children under a permanently-sealed `stopped` loop keep
an `active` leg row (durable garbage, no live session). Safe reclamation
needs a "never re-drivable" signal the model lacks today — future work,
documented in the BUG-543 ticket.
