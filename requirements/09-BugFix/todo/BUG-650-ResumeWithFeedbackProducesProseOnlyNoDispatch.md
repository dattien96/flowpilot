# BUG-650 — A resume-with-feedback turn can complete having produced only prose: no `submit_review_outcome`, no dispatch, no record — the decision is "accepted" but nothing acts on it

- **ID:** BUG-650
- **Severity:** High — the operator's feedback is acknowledged by the
  ledger yet no edge fires; the run sits "running" with no work, looking
  alive but dead.
- **Status:** OPEN (captured live, run-523131, parent turn 14:30–14:31Z)

## Evidence chain (all live)

1. run-523131: operator resumed with feedback ("continue past the card").
   The resumed hub turn ran to `turn_completed` having written only a
   prose reply — no tool call, no `submit_review_outcome`, no edge
   traversal.
2. The decision surface recorded the resume as accepted while the
   underlying parked node never received an outcome — the run stayed
   parked in the same shape, indistinguishable from "thinking".

## Root cause (hypothesis)

The resume-turn prompt does not constrain the model to a routing tool
face (schema-first T1), and nothing validates that a turn answering a
parked decision produced a machine verdict — prose completion is
accepted as a finished turn.

## Fix direction

- `F-1` Turns resumed with feedback into a parked decision must be
  prompted with the tool face and validated: a turn ending without a
  routing call = `turn_invalid`, auto-reprompt once, then park.
- `F-2` Persist "decision consumed but no outcome recorded" as a drift
  event so the wedge is diagnosable without log archaeology.

## Regression coverage

- `TestBug650_ResumeFeedbackRequiresOutcome` — prose-only turn on a
  parked decision → reprompt or park, never silent accept.
- `TestBug650_OutcomeRecordedFiresEdge` — same resume with a verdict
  call → edge traverses.
