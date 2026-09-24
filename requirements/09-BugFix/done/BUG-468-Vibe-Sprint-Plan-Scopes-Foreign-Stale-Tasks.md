# BUG-468: Vibe sprint plan globbed foreign/stale Task files — sprints ran wrong-feature work

- status: done
- found: live run-91517 + run-91606 (fp-beds/full, A-60-4 resume-matrix drill)
- fixed_by: CA-965
- tests: internal/runner/bug468_vibe_task_plan_cp_scope_test.go

## Symptom (live)

On the full bed, `requirements/08-Task/todo/` held nine Task files from three
different producers:

- `Task-1/2/8` — stale calc/strutil tasks, `Parent Documents: none`
- `Task-9/10/11` — earlier CP-01 slicer run, `Parent Documents: CP-01`
- `Task-12/13/14` — run-91517's slicer for CP-02, `Parent Documents: CP-02`

Both vibe runs entered sprint and stamped
`vibe_task_name: Task-1-calc-gcd-lcm.md`, `vibe_task_index: 1`,
`vibe_task_total: 9` — i.e. a snake vibe flow sprinted a **calc** task file,
and Task-1 was bumped to `Status: in_progress`. Wrong-feature work, twice,
on one bed.

## Root cause

`collectVibeTaskPlan` is a bare glob — `requirements/08-Task/todo/Task-*.md`
sorted — with no notion of which CP produced each file. After `task_slicer`
completed, `onVibeCpNodeDone` loaded the whole directory as the sprint plan,
so index 1/N was always the lexically-first stale file. The same unscoped
glob fed every "did this run's slicer already produce tasks" presence check
(join-resume park, slicer-restart, checkpoint artifact detection, sprint
resume fallback), so foreign tasks also masked missing-work detection.

## Fix

- `rs.vibeCpDocID` (durable, `sessions.ndjson`) pins the CP document this
  run's task set belongs to. Stamped at cp-ingest turn admission
  (`validateVibeCpIngestSource`, from the validated source's `Document ID`)
  and at `forceStartVibeTaskSlicer` (from the CP the slicer consumes).
- `collectVibeTaskPlanForCP(cwd, cpID)` keeps only tasks whose
  `Parent Documents` metadata line names cpID. The Parent-Documents line
  alone is parsed — `Related Documents` mentions of another CP (Task-12
  cites CP-01 as prior plan) must not pull the task in. Empty cpID returns
  the legacy unscoped glob so pre-fix runs and fixtures are untouched.
- Scoped at every "this run's tasks" site: slicer-done plan build +
  fail-closed gate (park reason names the CP), `maybeParkVibeCpJoinResume`,
  `restartVibeTaskSlicerForMissingTasks`, checkpoint artifact detection,
  sprint-resume fallback.
- Scoped run whose slicer produced no matching tasks parks fail-closed
  ("produced no Task files parented to CP-XX … refusing to sprint
  foreign/stale tasks") — never silently widens to unrelated work.
- `workingmode.CPDocumentID` extracts the normalized `CP-NN` value from
  `Document ID: CP-*` frontmatter.

The task-splitter prompt already mandates `Parent Documents` linking the
CP, so conformant slicer output is always attributable; tasks that name no
parent can never be claimed by a scoped run.
