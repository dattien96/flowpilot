# CA-1067 — BUG-552: escalate park swallowed the routed gate-decision

Date: 2026-09-29
Type: BugFix
Area: local-runner / interactive resume + gate-decision routing

## Summary

`SubmitGateDecision` routed the operator's choice into
`resumeFlowWithFeedback`, which embedded the decision **only in the
dispatched turn's prompt** (and the turn's `pendingAgentContext` drain). On
the blocked-escalate path — the live run-15525 shape — the turn carrying
the decision was the exact turn the next escalate-repark cancelled, or an
orphan-redrive answered with its own generic prompt instead. The ACK
(`routed_to=…`) was emitted before delivery was ever proven, so the
operator's decision could vanish while the UI claimed acceptance.

## Root cause

The decision had no durable routing record: it existed as prompt text on a
single in-flight turn. Any cancellation between dispatch and provider start
(escalate re-park, restart, chat switch) lost it permanently; the loop kept
re-escalating on the same gate with no memory of the operator's answer.

## Fix — record the decision durably before any dispatch

`resumeFlowWithFeedback` now appends `"Operator feedback: <feedback>"` into
`pendingAgentContext` (persisted via `persistProviderSession`) **before**
`mutateLoop`, on every generic resume path — blocked escalate, quiet
running-loop redrive, and normal continue. `startTurn` drains the context
atomically into whichever turn next actually runs, so the decision is
delivered exactly once, across re-parks and restarts. The existing prompt
embedding is kept for immediacy; the durable note is the delivery
guarantee.

The pre-existing `!wasBlocked` (BUG-551 quiet-redrive) append is subsumed —
the note is now written once at the top instead of in that branch only.

## Files

- `apps/local-runner/internal/runner/interactive_service.go` — durable
  `pendingAgentContext` append in `resumeFlowWithFeedback`.

## Tests

`apps/local-runner/internal/runner/bug552553_park_decision_and_cohort_test.go`

- `TestBug552_RoutedDecisionRecordedDurably` — escalate-parked parent +
  parked orphan child (the early-return path that used to drop the
  decision): the decision note lands in `pendingAgentContext`.
- `TestBug552_DecisionSurvivesSubsequentPark` — a re-park after resume
  must not wipe the buffered note.

Both verified RED on the pre-fix code (`pendingAgentContext=[]`) and GREEN
after.
