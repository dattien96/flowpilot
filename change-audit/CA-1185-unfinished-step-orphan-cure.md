# CA-1185 — orphan-cure extended to unfinished-step parked children (BUG-1185)

## Defect

`resumeFlowWithFeedback`'s orphan-cure only re-drove parked
`waiting_user_approval` children carrying `pendingGateCodePaths` (the BUG-520
gate-debt seed). A child the park froze mid-step with no gate violation — and
therefore no code-paths seed — was skipped and stayed parked forever, its flow
step stuck `RUNNING`/`WAITING_USER_APPROVAL` (live CP-03: hub-stall parks
froze members that had no gate debt at all).

## Fix

The orphan filter now accepts two evidence channels:

1. `pendingGateCodePaths` non-empty — the original gate-debt seed.
2. An unfinished flow step for the child's label — the parent's step rows are
   loaded once and `RUNNING`/`WAITING_USER_APPROVAL` count as owed work.
   `PENDING` steps are excluded (they belong to the flow's own dispatch);
   terminal rows mean nothing is owed.

Non-gate orphans get a plain resume prompt ("continue your node's work")
instead of the gate re-check prompt; the gate-debt prompt is unchanged.

## Regression tests

`bug1185_unfinished_step_orphan_test.go` — a parked child with a RUNNING step
and no gate paths is re-driven on Continue (turn reaches its provider); a
parked child whose step is DONE stays parked (guard).
