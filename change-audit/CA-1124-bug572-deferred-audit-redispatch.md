# CA-1124 — BUG-572: deferred audit node never re-dispatches

## Why

Live run-100368: `flow_validate_audit_dispatch` defers a not-ready
`artifact.audit_draft` node (open cohort / RUNNING sprint step) by stamping
it back to `PENDING` and returning — on the assumption that the next cohort
join or flow edge re-dispatches it. When the upstream `synthesis` done-edge
had already been consumed before the defer, no later event owned the
dispatch: the audit sat `PENDING` forever while every evidence node was
`DONE` and the run showed `running`. Only an operator
`flow-control {"status":"done"}` re-walked the flow.

## What changed

`apps/local-runner/internal/runner/wedge_sweep.go`:

- `maybeRedispatchDeferredAudit` re-drives a PENDING
  `artifact.audit_draft` node through the normal `runAuditNode` inline path
  — but only when the defer's own conditions are gone: no open cohort, no
  other RUNNING sprint step, and every transitive forward-done predecessor
  terminal (DONE/SKIPPED), plus the loop still advancing. Any of those
  still true → stays parked, same as the defer.

The sweep runs on the existing `StartSettleSweep` tick.

## Invariant

A deferred dispatch must always have a converger. Re-dispatch goes through
the same `runAuditNode` the edge-driven path uses — audit still evaluates
its own readiness and fails closed (blocked drafts escalate; never a silent
pass). Fail-closed ordering is preserved: a PENDING/CANCELED predecessor
keeps the node parked.

## Tests

`bug571_wedge_sweep_test.go` (red → green):

- `TestBug572_SweepRedispatchesDeferredAudit` — PENDING audit with DONE
  predecessors and no running steps leaves PENDING after the sweep.
- `TestBug572_SweepKeepsAuditDeferredWhileSprintStepRuns` — audit stays
  PENDING while another sprint step is RUNNING.
