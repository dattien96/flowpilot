# CA-828 — CP present → resume join task_slicer; CP missing → rewrite cp_writer

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: Task-328
change_type: feature
summary: After cp_writer, reopen parks Resume to join task_slicer when CP exists; CP deleted with SS remaining restarts cp_writer only (no ss_lock)
# --->8---

## Why

Live R-CP-K / R-CP-D1: after `cp_writer` DONE, `/exit` reopen showed `running · blocked` with no card (`cp_writer → done` has no forward successor for `pendingVibeResumeFromNode`). Delete CP, reopen stayed idle. Operator expect: Resume → `task_slicer` when CP remains; rewrite `cp_writer` when CP gone (SS already locked — no SS Preview).

## Change

- `vibeCPArtifactsPresent`, `restartVibeCpWriterForMissingCP`, `maybeParkVibeCpJoinResume`
- Reconstruct order: SS-missing (Task-327) → CP-missing rewrite → generic resume → CP-join resume
- Resume OK `from=cp_writer` → `maybeStartVibeCpIngest` or CP rewrite
- Bail CP rewrite while `vibeAwaitingLock` on `ss_lock` (keep R-SS-K)

## Tests

New `task328_cp_resume_missing_rewrite_test.go`. Task-327 / CA-770 / CA-793 / CA-801 untouched green.

## Providers

Case 1 agnostic (no `providerKey` branch).

## Will not undo

CA-827 / Task-327 SS recover. CA-791 join. CA-793 demote. CA-770 lock restore.
