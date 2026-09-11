# Task-329: Missing Task files → restart task_slicer (not Resume→tdd)

## Metadata

- Document ID: `Task-329`
- Title: `Delete Task-*.md with CP remaining → re-run task_slicer; do not park Resume from tdd`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-11`
- Last Updated: `2026-09-11`
- Parent Documents: [CP-60-Test-Steps](../../07-Coding-Plan/done/CP-60-Test-Steps.md), [Task-328](./Task-328-Vibe-CP-Resume-And-Missing-CP-Rewrite.md), [CA-793](../../../change-audit/CA-793-Vibe-Checkpoint-File-Exist.md)
- Tags: `vibe-mode, resume, task_slicer, R-TK-D1, O-6`
- Feature Keys: `vibe-mode`

## AI Quick View

### Summary

- Close CP-60 **R-TK-D1**: Tasks deleted, CP remains → restart `task_slicer`.
- Must not park **Resume from tdd** using stale `vibe_task_plan` + `vibe-sprint` graph.

### Current Ask

- Implement + tests; live re-prove after rebuild.

### Key Decisions

- `T-1` Detect missing Tasks via `collectVibeTaskPlan` empty while CP present and (stale plan / vibe-sprint / checkpoint tdd|task_slicer).
- `T-2` Clear `vibeTaskPlan` then `forceStartVibeTaskSlicer` before `maybeParkVibeResumeConfirm`.

### Constraints

- Additive tests only. Provider-agnostic. Do not undo Task-327/328.

### Source Refs

- CP-60 R-TK-D1; live run-225468 Task-904/905/906 delete → Resume tdd (wrong).

## 1. Goal

Operator deletes `Task-*.md` after slicer/sprint stop; reopen re-runs slicer from CP, not tdd.

## 8. Completion Notes

- result: `done` — CA-829; `TestTask329_*`
