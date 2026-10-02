# CA-1114 — BUG-566: audit entry settles the sprint's Task doc todo→done

## Why

Live run-69320 (PrivateVault Task-024): the sprint converged — reviewer
cohort unanimously approved, validation green — but the audit gate parked
on `task_referenced`: the Task doc never appeared in the aggregate diff.
Nothing touches `requirements/08-Task/todo/Task-NNN-*.md` during a sprint
once `stampVibeTaskInProgress` has no-op'd on an unchanged status, and the
doc was committed pre-sprint — so the only thing that satisfied the gate
was an operator running `git mv todo/ done/` by hand. The lifecycle
transition the flow owns conceptually was operator work in practice.

## What changed

`apps/local-runner/internal/runner/vibe_task_stamp.go`:

- New `settleVibeTaskDocDone(cwd, rel)`: pure `os.Rename` of the sprint's
  Task doc `requirements/08-Task/todo/<name>` → `.../done/<name>`, contents
  preserved byte-for-byte and status untouched (BUG-367: `done` stays off
  doc status — the directory carries the signal). Idempotent, never
  clobbers an existing done/ file, no-ops on non-todo/missing input.
- New `resolveVibeTaskDocPath(cwd, rel)`: resolves a plan task path to
  todo/ first, then the same basename under done/ — so bookkeeping written
  after the move still lands on the settled file.
- `stampVibeTaskDoDChecked` resolves through the new helper (boundary DoD
  ticking follows the moved doc); `readVibeDocStatus` falls back to done/
  for paths whose todo/ file is gone (a settled doc reports its real
  status, and `reconcileVibeSprintCursor` keeps treating it as started).

`apps/local-runner/internal/runner/flow_validate_audit_dispatch.go`:

- `runAuditNode`: when a vibe sprint is in flight (`vibeSprintIndex>0`,
  vibe topology), the sprint's plan task settles todo→done BEFORE
  `changedFilesSince`/`ObserveGitDiffSince` run — so the rename lands in
  the same aggregate diff the `task_referenced` gate reads. The plan entry
  is re-pointed at the done/ path and the move is diag-logged
  (`vibe_task_doc_settled`). Best-effort: a failed rename leaves the old
  gate behavior, never blocks the audit.
- Tradeoff noted: the move happens at audit entry, before the audit
  verdict lands — if audit escalates on bookkeeping (missing CA etc.) the
  doc already sits in done/ while remediation runs. Acceptable because
  audit entry means implementation + review already passed; the gate that
  motivated this can only be satisfied pre-observation.

`apps/local-runner/internal/runner/vibe_tasks_live_test.go`:

- `TestVibeTasks_ChainWalksFiveTaskPlan`: waits for the sprint-1 entry
  leg's cohort join before simulating the terminal audit — BUG-557 binds
  the read-only preflight leg to a one-member adhoc cohort whose join is
  asynchronous, and `hasOpenCohort` correctly refused the park while the
  leg was mid-flight. Pre-existing fixture race, now closed.

## Invariant

The flow owns the Task document lifecycle end-to-end: `in_progress` at
sprint start, `todo→done` at audit entry, DoD ticked at the boundary —
operator hands never touch the file.

## Tests

`bug566_task_doc_settle_test.go`:

- settle moves todo→done preserving bytes; idempotent; never clobbers an
  existing done/ doc; rejects non-todo/missing/empty input
- `runAuditNode` settles the current sprint's doc and the moved file is
  visible to `ObserveGitDiff`/`HasTaskDoc` (the gate's own observation)
- no settle before the first sprint (index 0)
- boundary `stampCompletedVibeTask` still ticks DoD on the moved file
