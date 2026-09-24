# CA-958 — Live-found: keep_branch silently dropped uncommitted work

## Summary

Live SWE-2 validation showed `keep_branch` returned success
(`worktreeState: kept_branch`, branch retained) while the worktree's
uncommitted artifacts — e.g. an agent-written `WT_MARKER.txt` — were deleted
with the worktree and never reachable from the kept branch. The spec intent
(SS-23 R-3) is that keep_branch preserves the branch's work; uncommitted
changes are not on the branch, so the mode silently lost data.

Fix mirrors the existing discard guard (SD-27 Q-2): `keep_branch` now checks
`Manager.Uncommitted` (`git status --porcelain` — covers untracked AND
modified-tracked, a superset of discard's `ls-files --others`) and returns
`409 worktree_keep_branch_confirm` + `requiresConfirm:true` + the
`uncommitted` path list. Resending with `confirm=true` proceeds. The
delete-gate path already passes `confirm=true` (explicit user decision), so
delete flows are unaffected. Clean worktrees still resolve immediately —
`TestE2EWorktree_KeepBranchHTTP` unchanged and green.

The evidence-payload passthrough in `handleWorktreeResolve` was extended to
emit the new code's conflict body (same shape as `worktree_discard_confirm`).

## Repro

`TestE2EWorktree_KeepBranchWithUncommittedRequiresConfirm` in
`cp71_worktree_resolve_modes_test.go`: uncommitted file in the worktree →
keep_branch without confirm returned `200 kept_branch` (red — file silently
lost); now `409` + `requiresConfirm`, worktree survives until confirmed.

## Files

- `apps/local-runner/internal/worktree/manager.go` — `Manager.Uncommitted`
  (new, additive)
- `apps/local-runner/internal/runner/run_worktree_merge.go` — keep_branch
  gate + handler passthrough for `worktree_keep_branch_confirm`
- `apps/local-runner/internal/runner/cp71_worktree_resolve_modes_test.go` —
  new test appended (existing tests untouched)

## Verified

- `go test ./internal/runner -run TestE2EWorktree -v` — 15/15 pass including
  the new red→green test and all pre-existing resolve-mode tests.
- `go test ./internal/worktree` — pass (Uncommitted covered via E2E).

## Provider parity

Provider-agnostic: git/worktree plumbing at resolve time; no provider session
interaction. The 409 contract matches the existing discard-confirm shape the
desktop already handles via `resolveWorktreeMerge(runId, mode, confirm)`.
