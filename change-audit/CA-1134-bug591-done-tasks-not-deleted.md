# CA-1134 — BUG-591: Resume-confirm `ok` re-sliced a foreign CP

## What changed

`apps/local-runner/internal/runner/vibe_cp.go`:

- `restartVibeTaskSlicerForMissingTasks` (R-TK-D1) judged "tasks missing" via
  `collectVibeTaskPlanForCP` — which scans `requirements/08-Task/todo/` only.
  After the last CP-02 task moved to `done/`, the scoped glob emptied and the
  check read a finished sprint as a deletion, force-starting `task_slicer`,
  which rebound the run to a foreign CP (CP-10) — live run-139670 morphed
  mid-resume.
- New `collectVibeDoneTasksForCP(cwd, cpID)` — the `done/` counterpart with
  the same `Parent Documents` scoping. Presence in done/ proves the CP's
  tasks exist; only absence from BOTH directories counts as deleted.
- The missing-tasks check now returns false when either directory holds
  scoped tasks. Genuine deletions (zero files in both) still re-slice.

## Invariant

A task in `done/` is completed, not deleted. Deletion recovery fires only
when planned files are absent from the whole `08-Task/` tree.

## Tests

`bug591_done_tasks_not_missing_test.go` — red-first: task in done/ + foreign
CP-10 task in todo/ → no slicer restart, plan preserved; all-deleted →
slicer restarts (R-TK-D1 preserved).
