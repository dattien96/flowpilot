# CA-831 — reset stale vibeSprintIndex on CP rewrite / slicer reload

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: BUG-372
change_type: bugfix
summary: Clear vibeSprintIndex + plan on CP rewrite and reload plan with index=0 after task_slicer so chip cannot show task 2/3 while Task-904 is still draft
# --->8---

## Why

Live run-225468 after R-TK-D2: Tasks regenerated, chip showed **task 2/3**, Task-905 `in_progress`, Task-904 still `draft`, almost no snake code. Session `vibe_sprint_index=2`. Root cause: CP rewrite kept stale plan/index; slicer-done only loaded disk plan when `len(vibeTaskPlan)==0`.

## Change

- `clearVibeSprintCursor` — plan + index + boundary flags
- Call from `restartVibeCpWriterForMissingCP`, `restartVibeTaskSlicerForMissingTasks`, `forceStartVibeTaskSlicer`
- `onVibeCpNodeDone(task_slicer)`: always reload disk plan and set `vibeSprintIndex=0` before first `maybeStartNextVibeSprint`

## Tests

New `bug372_stale_sprint_index_test.go`. CA-783/CA-791/CA-770 untouched green.

## Providers

Case 1 agnostic.

## Will not undo

CA-829/828 recover paths; chip formula BUG-367 (index after takeNext still 1-based display for first sprint).
