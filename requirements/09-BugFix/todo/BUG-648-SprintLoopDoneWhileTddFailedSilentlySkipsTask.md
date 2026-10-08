# BUG-648 — A sprint whose TDD leg FAILED can still close as `done`: the task-chain advances its checkpoint to the next task and the failed task is silently skipped with zero output files

- **ID:** BUG-648
- **Severity:** Critical — the sprint ledger records a task as done that
  produced nothing; every downstream task builds on a hole. This is the
  strongest silent-skip defect observed.
- **Status:** FIXED — CA-1236 (2026-10-08): sprint evidence completeness now requires tdd/coder/validate DONE; failed evidence escalates instead of sealing the boundary

## Evidence chain (all live)

1. run-523131, sprint for Task-112: TDD leg `run-554503` FAILED on
   provider limit mid-turn; the sprint loop still evaluated `done` and
   the task-chain checkpoint advanced to sprint 3 (Task-113).
2. The failed sprint left `coder`/`validate`/`reviewer`/`synthesis`
   skipped and zero implementation output — the failure was invisible
   in the step ledger (`audit=DONE`, `synthesis=DONE`, `tdd=FAILED`,
   rest SKIPPED → sprint closed).
3. Task-112's actual Kotlin bodies only landed later via adjudication
   adj-55 (`keep-test-fix-code`) re-drive — absent that manual repair,
   CP-11 would have shipped a silently empty task.

## Root cause (hypothesis)

The sprint-loop done predicate counts node terminal states (or the
audit node's verdict) without requiring `tdd`+`coder` to be DONE — a
FAILED upstream leg satisfies the exit condition and the chain advance
consumes the checkpoint unconditionally.

## Fix direction

- `F-1` Sprint done predicate must require the write-path steps
  (`tdd`, `coder`, `validate`) all DONE — a FAILED leg holds the sprint
  open and routes to the owner-debate/redrive path.
- `F-2` Task-chain advance asserts "task produced declared-path output
  delta" before checkpointing — zero-delta tasks never silently skip.

## Regression coverage

- `TestBug648_FailedTddKeepsSprintOpen` — tdd leg fails → sprint stays
  blocked, task chain does NOT advance.
- `TestBug648_ZeroDeltaTaskNotCheckpointed` — sprint with no declared-
  path writes cannot mark the task done.
