# BUG-646 — `agent-loop/amend` mints a new contract and resumes the run but never unparks the leg's `pendingFlowGateSettle`, so the amended leg stays parked until a manual `continue`

- **ID:** BUG-646
- **Severity:** Medium — amend looks successful but silently leaves the
  flow parked; the run shows "running" with no leg activity until an
  operator notices.
- **Status:** OPEN (captured live, run-523131, leg run-525391)

## Evidence chain (all live)

1. run-523131, leg `run-525391` parked on a scope gate. Operator called
   `POST /agent-loop/amend` with expanded scope → contract row written,
   run resumed — but `pendingFlowGateSettle` on the leg stayed armed.
2. The leg remained `waiting_user_approval`; only a separate
   `agent-loop/continue` (or `flow-control`) flushed the settle intent.
   Amend+unpark required two operator calls where one should suffice.

## Root cause (hypothesis)

`handleAmend` (interactive_service) writes the amended contract and
resumes the run, but does not clear the leg's `pendingFlowGateSettle`
intent nor arm `proposalTurnPending` — the same half-consumed gate
state class as BUG-649.

## Fix direction

- `F-1` After a successful amend, treat the leg like an accepted gate
  decision: clear `pendingFlowGateSettle`, arm the reprompt, dispatch.
- `F-2` If the leg's node is inside a nested debate, route the unpark
  through the cohort barrier instead of freeing the leg directly
  (interacts with BUG-637).

## Regression coverage

- `TestBug646_AmendUnparksLeg` — parked leg + amend → leg dispatches a
  new turn without a manual continue.
- `TestBug646_AmendInsideDebateKeepsBarrier` — cohort member stays
  parked; settle routed to the cohort join.
