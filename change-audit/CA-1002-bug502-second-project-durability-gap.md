# CA-1002 — BUG-502 live-found: second-project run durable rows lost across restart

## What

Live evidence collection + documentation only — NO production fix (mechanism
unproven; safe-fix contract forbids blind fixes).

- `requirements/09-BugFix/todo/BUG-502-Second-Project-Run-Durable-Rows-Lost-Across-Restart.md`
  — full live evidence, eliminated mechanisms, instrumented-repro plan.
- `internal/runner/bug502_cross_project_session_persist_test.go` —
  regression guard asserting a non-workspace-project chat run leaves a
  durable session row through the real HTTP surface + real
  `localFileSessionStore`. PASSES on current build (locks in the verified
  good path).
- Runbook `CP-Full-Live-Test.md` §G rows for the 2026-09-25 b501 live
  matrix: CP-84 L-1..L-7, CP-82 multi-project + worktree uniqueness,
  CP-71 L-1..L-8, B-51/B-59 kill-9 reconcile, BUG-502 finding, and the
  partial A-1/A-60 flow-arm evidence.

## Live evidence summary (build b501, bed /Users/tiendat/fp-beds/full)

- `run-260777` (project db51ec26) + leg `run-260889`: real grok turn +
  gate park (`waiting_approval`, `appr-260887`) — zero session/approval
  rows; post-restart `run_not_found`/`chat_not_found`.
- Control `run-260775` (workspace) persisted + rehydrated; fresh
  `run-266599` (db51ec26) on the same build wrote 3 rows and survived a
  graceful restart — write path is not project-gated.
- Concurrent second runner (`:19500`, build e85776b3) shares the same
  `sessions.ndjson` — a real cross-process rewrite hazard, though that
  instance was idle during the window.

## Why no code fix yet

Every plausible static mechanism was checked and eliminated (no project
gate in persist callsites, store is single-instance, delete-rewrite only
drops malformed/target lines, boot recovery never deletes). The fix must
wait for an instrumented reproducer per BUG-502 doc.

## Verification

- `go test ./internal/runner/ -run TestBug502SessionRowPersistsForSecondProjectRun` — PASS.
- Gate restored `observe` → `enforce` on bed after drills.
