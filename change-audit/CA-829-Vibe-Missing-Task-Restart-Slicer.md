# CA-829 — missing Task files restart task_slicer (not Resume→tdd)

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: Task-329
change_type: feature
summary: When Task-*.md deleted but CP remains, clear stale vibeTaskPlan and restart task_slicer before Resume-from-tdd can park
# --->8---

## Why

Live R-TK-D1 (run-225468): operator deleted Task-904/905/906 after stop mid-sprint. Reopen parked **Resume from tdd** (stale `vibe_task_plan` + `vibe-sprint` graph). Continue ran tdd. Expected: re-run `task_slicer` from CP (R-TK-D1). Resume→tdd is the **Tasks still present** path (R-TK-K).

## Change

- `restartVibeTaskSlicerForMissingTasks` — CP present, Tasks absent, expected via stale plan / vibe-sprint / checkpoint tdd|task_slicer
- Reconstruct + Resume OK: run before `maybeParkVibeResumeConfirm` / tryAdvance
- Clear `vibeTaskPlan` then `forceStartVibeTaskSlicer`

## Tests

New `task329_missing_task_restart_slicer_test.go`. Task-327/328 + CA-801 untouched green.

## Providers

Case 1 agnostic.

## Will not undo

CA-827/828 SS/CP recover. CA-793 demote. R-TK-K when Tasks remain on disk.
