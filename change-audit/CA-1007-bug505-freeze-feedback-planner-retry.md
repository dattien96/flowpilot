# CA-1007 — BUG-505: freeze-escalate feedback routes to planner retry, not freeze input

## Context

Live runs 3688/20370: after a `contract.freeze` escalation for a missing
planner draft, `resumeFlowWithFeedback` fed raw operator feedback straight
into `runContractFreezeNode` as `plannerResult`. Prose ("retry: emit the
JSON") was parse-failed verbatim (`invalid planner proposal: 'c'`), so the
freeze re-escalated forever; only an operator-pasted JSON draft unblocked
it — an undocumented escape hatch.

## Changes

`resumeFlowWithFeedback` (`interactive_service.go`), when the escalated
inline node is `contract.freeze`:

1. Feedback that parses as a valid preflight draft → feeds the freeze
   directly (live-verified escape hatch preserved).
2. Else, a draft retrievable from the planner child's events or the
   parent cache (parse-validated now, not just non-empty) → retries the
   freeze without re-running the planner.
3. Else, when the topology contains `preflight_contract_plan` → sets
   `failedDelegateNodeID` so Continue re-invokes the planner delegate
   with the note as guidance (the same retry path a failed writer takes).
4. No planner node → falls back to the existing inline handling.

## Tests

`bug505_freeze_feedback_retry_test.go` (new, additive): prose feedback
retries the planner; bare Continue retries the planner; operator-supplied
draft feeds freeze directly (no planner re-run); retrievable draft skips
the planner retry.

## Verification

`go test -count=1 -run TestBug505 ./internal/runner/` — 4/4 green.
