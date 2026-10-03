# CA-1153 — BUG-622: persistedCompletedChildExists scoped to the sprint

Live trigger: run-150388 (continuation of the BUG-616/619/620 wedge). After
the owner-debate overlay unmounted and the diverted tdd completion drained,
the sprint chain had to spawn sprint-2's first `coder` leg — but:

- `pendingVibeResumeFromNode` iterated every node whose step was DONE **or**
  which had a persisted completed child of the same label. Sprint-1's
  `coder`/`reviewer` legs (vibe_task_index=1) stay `completed` in the session
  index forever (legs close only at run end), so they qualified — the
  last-write-wins loop produced `Resume from reviewer?` instead of
  `Resume from tdd?`. Answering that card would have run
  `tryAdvanceFlowFromNode(reviewer)` and skipped sprint-2's un-run
  coder/validate legs entirely.
- `maybeAdvancePendingValidateAfterCoder` had the same hole in the other
  direction: `persistedCompletedChildExists("coder")` returning true off a
  sprint-1 leg could fire `coder → validate` past a never-run coder.

Root cause: the same class as BUG-616 (session→node projection) and
BUG-620 (frozen-contract head) — a lookup that treats `parentRunID + label`
as unique when vibe sprints reuse node ids every task.

## Fix

`persistedCompletedChildExists` now filters sessions through
`sessionBelongsToVibeSprint(session, parent.vibeSprintIndex)` — the same
predicate BUG-616 applied to `resumedFlowStepRows`. Only sessions stamped
with the run's current sprint index satisfy the check; index-0
(unstamped/legacy) sessions keep fail-open behavior; non-sprint parents
(index 0) keep the unscoped scan byte-identical.

## Files

- `internal/runner/vibe_sprint.go` — scoped filter + doc note.
- `internal/runner/bug622_cross_sprint_completed_child_test.go` — red-verified
  repro: sprint-1 completed coder/reviewer legs no longer satisfy sprint-2's
  nodes; current-sprint and legacy-unstamped legs still count.

## Verification

- `go test -run TestBug622` red before fix (sprint-1 coder satisfied
  sprint-2's node), green after.
- Resume/sprint/stall families green: BUG-616..619, VibeResume/VibeSprint,
  PendingVibe, HubStall, ResumeVibe/ResumeFlow, ContinueReinvoke,
  VibeRequirement, AdvancePending.
- `TestVibeSprintFreezeSpawnsTddThenCoder` flaked once on
  `TempDir RemoveAll` cleanup under the broad run; green in isolation.

Live: after restart, `pendingVibeResumeFromNode` derives `from="tdd"`, the
resume card reads `Resume from tdd?`, and OK routes through
`resumeVibeAfterTddGate` → `maybeResumeVibeCoderAfterTdd` →
`tryAdvanceFlowFromNode(tdd)` — the designed first-round coder spawn.
