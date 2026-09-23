# CA-940 — BUG-378: worktree merge-back diff anchored at the base commit

## Summary

`worktree.Manager.Diff` produced the merge patch with a bare `git diff`
(working-tree vs index) — work committed on the worktree branch was invisible
to it, so `apply_patch` could report `{"applied":true,"worktreeState":"merged"}`
while silently dropping committed provider work (live-verified on :4317).

Fix: when the base sidecar exists, anchor the diff at the recorded base
commit (`git diff <base>` covers committed + staged + unstaged +
intent-to-add — the full delta the contract promises). Missing sidecar keeps
the legacy bare diff. Conflict oracle (`git apply --check`) and StrictHead
tournament semantics unchanged.

## Verified

- 2 new tests red→green (`manager_test.go`): committed branch work appears in
  the patch and lands in the main workspace via `ApplyWithOptions`.
- `go test ./internal/worktree/` + runner worktree suite: all green, no
  existing test touched.

## Files

- `internal/worktree/manager.go`,
  `internal/worktree/manager_test.go` (additive),
  `requirements/09-BugFix/todo/BUG-378-…md` (new)

# ---8<--- flowpilot:change-ledger
feature_key: run-worktree
source_doc_id: BUG-378
change_type: bugfix
