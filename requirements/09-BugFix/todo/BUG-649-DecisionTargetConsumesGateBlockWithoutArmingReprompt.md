# BUG-649 — The `decisionTarget` code path consumes `pendingGateBlock` but never arms `proposalTurnPending`, so a reprompt after `suggest-requirement-change` silently re-evaluates the SAME gate block instead of the new scope

- **ID:** BUG-649
- **Severity:** High — the adjudication loop accepts the decision but the
  next leg turn re-hits the old block; seen as repeated identical cards
  across rounds (adj-53 adjudicated "amend the contract" yet the leg
  still bound the stale scope).
- **Status:** FIXED — CA-1236 (2026-10-08): decisionTarget arms proposalTurnPending + clearedAt so the target run re-prompts instead of re-blocking

## Evidence chain (all live)

1. run-523131, Task-113 TDD leg: gate blocked on scope; operator chose
   the `decisionTarget` path (suggest-requirement-change / custom with a
   declaration). `pendingGateBlock` was consumed (cleared), but
   `proposalTurnPending` — set only in the direct-block branch — stayed
   false.
2. The reprompted turn ran under the OLD gate context: contract binding
   still evaluated the pre-amend scope, producing the same violation
   card again (3 rounds observed for task 3).

## Root cause

Two asymmetric branches: direct-block response arms
`proposalTurnPending`; `decisionTarget` clears the pending gate state
without arming it — half the state machine transitions.

## Fix direction

- `F-1` Factor a single `consumeGateBlock` that always pairs
  `pendingGateBlock`-clear with `proposalTurnPending`-arm, and call it
  from both branches.
- `F-2` Assert in the reprompt dispatch that a consumed block implies an
  armed proposal turn — missing arm = defect event, not silent reblock.

## Regression coverage

- `TestBug649_DecisionTargetArmsProposalTurn` — decisionTarget answer →
  reprompt runs with amended contract, old block not re-evaluated.
- `TestBug649_BothBranchesSymmetric` — table-test both paths leave the
  same gate-intent shape.
