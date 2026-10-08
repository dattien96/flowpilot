# BUG-653 — The dead-dispatch sweep skips any run with `pendingFlowGateSettle` armed, so a parked hub masks a wedged RUNNING step underneath it: the park is the *symptom* of the deadlock but acts as its *shield*

- **ID:** BUG-653
- **Severity:** High — every "parked card + stale RUNNING step" shape
  self-hides from the only mechanism that could heal it; observed wedge
  persisted ~4h until manual `flow-control`.
- **Status:** FIXED — CA-1236 (2026-10-08): pendingFlowGateSettle removed from the run-level sweep filter; hub node skipped while parked, dead delegate steps settle via tryAdvanceFlowFromNode

## Evidence chain (all live)

1. run-523131 sprint-4: `tdd` step RUNNING-dead (leg completed,
   BUG-647), `synthesis` parked WAITING_USER_APPROVAL on
   "missing verdict" — correct park, since cohort never ran.
2. `maybeRedriveDeadDispatchedSteps` bails at the top when
   `pendingFlowGateSettle` is armed → the dead `tdd` was never settled
   or redriven → the park's missing-verdict condition never cleared →
   stable deadlock (park guarding the wedge that caused the park).
3. Operator `flow-control {status:"continue"}` fired the intended
   back-edge and unblocked — the heal path existed but was unreachable.

## Root cause

The sweep's guard conflates "run has a live question for the user" with
"run has no dead dispatch". A park card is a surface state; the sweep
should still heal dead steps beneath it.

## Fix direction

- `F-1` Split the guard: pending settle suppresses re-dispatch of the
  PARKED node, not the sweep of sibling dead steps.
- `F-2` When a parked node's missing-input names a dead-step id, prefer
  healing that step over re-parking the hub.

## Regression coverage

- `TestBug653_ParkedHubDoesNotShieldDeadStep` — armed settle + dead
  RUNNING step → step still healed/redriven.
- `TestBug653_ParkedNodeNotRedispatched` — the parked node itself still
  waits for its answer (no double-dispatch).
