# Task-328: Vibe CP resume gate + missing-CP rewrite (skip ss_lock)

## Metadata

- Document ID: `Task-328`
- Title: `After cp_writer — CP present → resume to task_slicer; CP deleted + SS remains → restart cp_writer`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-11`
- Last Updated: `2026-09-11`
- Parent Documents: [CP-60-Test-Steps](../../07-Coding-Plan/done/CP-60-Test-Steps.md), [Task-327](./Task-327-Vibe-Missing-SS-Auto-Resume-Ingest.md), [CA-793](../../../change-audit/CA-793-Vibe-Checkpoint-File-Exist.md), [CA-791](../../../change-audit/CA-791-Ingest-Joins-Task-Slicer.md)
- Child Documents: `None`
- Related Documents: [CA-827](../../../change-audit/CA-827-Vibe-Missing-SS-Auto-Resume.md)
- Replaces: `None`
- Tags: `vibe-mode, resume, cp_writer, task_slicer, O-6, R-CP-K, R-CP-D1`
- Feature Keys: `vibe-mode`

## AI Quick View

### Summary

- Close CP-60 §11 **R-CP-K** / **R-CP-D1** live gap (same O-6 family as Task-327, CP layer).
- **CP present**, ingest `cp_writer` done, no Tasks yet → reopen parks **Resume confirmation**; OK → `maybeStartVibeCpIngest` (`task_slicer`), not idle.
- **CP deleted**, SS still on disk → restart from **`cp_writer`** only (do **not** re-park `ss_lock` / do not full ingest).

### Current Ask

- Landed CA-828. Rebuild TUI; live re-prove R-CP-K (CP keep → Resume → task_slicer) and R-CP-D1 (CP delete → cp_writer, no SS lock).

### Key Decisions

- `T-1` `vibe-ingest` ends `cp_writer → done`; join to `task_slicer` is CA-791 `maybeStartVibeCpIngest`, not a forward edge — reopen must re-arm that join via resume gate.
- `T-2` Missing CP + SS present → `startResolvedFlowFromNode(..., cp_writer)`, never `ss_lock` card.
- `T-3` Empty cwd = unknown (same as Task-327 T-1); do not false-trigger.

### Constraints

- Additive tests only. Provider-agnostic. Do not undo Task-327 SS recover or CA-770.
- Out of scope: Task-layer missing-Task auto-spawn (R-TK-*); N-* new-chat probes.

### Open Questions

- None — live idle screenshots on run after cp_writer + CP delete.

### Source Refs

- CP-60-Test-Steps R-CP-K / R-CP-D1; CA-791 join; CA-793 demote; Task-327 O-6 pattern.

## 1. Goal

Operator stops after `cp_writer` writes CP, reopens: sees Resume gate → Continue joins `task_slicer`. Operator deletes CP (SS remains), reopens: `cp_writer` runs again without SS lock card.

## 2. Parent Links

- coding plan: CP-60 §11 O-6 residual (CP layer)
- tech design: SD-24 vibe ingest / CA-791 join
- system spec: SS-18 lock-before-sprint (SS already locked)

## 3. Trigger

Live Windows gate-sandbox: after cp_writer DONE, `/exit` reopen → `running · blocked`, no card. Delete CP, reopen → same idle. Operator expect matches R-CP-K / R-CP-D1.

## 4. Exact Change

- `T-1` `vibeCPArtifactsPresent` + `restartVibeCpWriterForMissingCP`
- `T-2` `maybeParkVibeCpJoinResume` when CP present, no Task plan, not already on vibe-cp-ingest
- `T-3` Reconstruct order: SS-missing recover → CP-missing rewrite → generic resume confirm → CP-join resume
- `T-4` Resume OK with `from=cp_writer` → `maybeStartVibeCpIngest` (CP present) or rewrite (CP missing)

## 5. Touched Areas

- files: `vibe_checkpoint.go` / `vibe_cp.go`, `interactive_resume.go`, `gate_hook.go`, new `task328_*_test.go`, CA-828, Task-328, CP-60-Test-Steps

## 6. Acceptance Check

- New tests green; Task-327 / CA-770 / CA-793 / CA-801 tests untouched green.
- Live: CP keep → Resume → task_slicer; CP delete → cp_writer rerun, no SS Preview.

## 7. Out of Scope

- R-TK-* missing Task auto-spawn; Branch C happy path.

## 8. Completion Notes

- result: `done` — CA-828; `TestTask328_*` green; Task-327 suite still green
- follow-ups: live tick R-CP-K / R-CP-D1; R-TK O-6 if needed
- upstream docs updated: CP-60 Current Ask
