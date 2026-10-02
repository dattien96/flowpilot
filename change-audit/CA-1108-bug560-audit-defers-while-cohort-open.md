# CA-1108 — BUG-560: audit defers a not-ready verdict while a cohort is open

## What
`runAuditNode`'s `draft.Status != "ready"` path now checks
`agentOrchestrator.hasOpenCohort(parentRunID)` before escalating: with an
upstream member still in flight the verdict is provisional, so the audit step
resets to PENDING and the loop keeps running. The cohort join re-dispatches
the audit through the normal hub → done-edge path once the barrier resolves.

## Why
Live run-60899: reviewer `run-67320` spawned into `flow-auto-validate-round-1`
at 10:31:52; audit ran at 10:32:09, hit `blocked_missing_feature_key` on stale
`openIssues=2`, escalated, and `parkFlowForAwaitingUser` cancelled the
reviewer's turn mid-flight (`cohort_member_failed "interrupted by user"`), then
drained the barrier on the released seat — nothing could rejoin on resume.

## Guarantees kept
- Infra escalates (git observe failure, draft persist failure) still escalate —
  only the verdict-driven not-ready path defers.
- `ready → done` was already guarded by `applyFlowControl`'s open-cohort
  soft-defer; this closes the asymmetric hole on the escalate side.
- Generic, not vibe-only: a premature audit verdict is wrong for every
  topology, and deferral is strictly safer than cancelling live member work.
- `applyFlowControl` still refuses done/continue while cohorts are open —
  deferral cannot advance the flow early.

## Tests
`bug560_audit_open_cohort_defer_test.go` (3 cases): defer while open (loop
stays running, audit → PENDING), escalate after the barrier drains, no defer
when no cohort is open. CA-1096/CA-1074/BUG-356 audit suites green, unedited.
