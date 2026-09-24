# CA-953 — KR-005 CP-71: recreate_empty dead on real lost bindings (stale branch)

## Summary

New HTTP-level coverage (`cp71_worktree_resolve_modes_test.go`) caught a real
production bug: `recreate_empty` on a `lost` binding always failed with
`worktree_recreate_failed` — `git worktree remove` deletes the worktree but
leaves branch `fp/<slug>-<owner>`, and `Manager.Create` always runs
`git worktree add -b <branch>`, which Git rejects when the branch exists.

Fix in `resolveWorktree`: for `recreate_empty`, delete the stale branch
(`git -C <repo> branch -D <b.Branch>`, error tolerated — branch may already be
gone) before calling `Manager.Create`. Correct per contract: `recreate_empty`
explicitly reports `priorChangesLost`, so the old branch is disposable.

New test file also covers: `worktree_unavailable` on non-git workspace,
`keep_branch`, `discard` untracked-file confirmation gate, `archive` on lost
binding, `invalid_mode`.

## Files

- `apps/local-runner/internal/runner/run_worktree_merge.go` — branch delete
  before recreate
- `apps/local-runner/internal/runner/cp71_worktree_resolve_modes_test.go` — NEW

## Verified

- `go test ./internal/runner` — 19.7s, all pass (incl. new E2E resolution tests).

## Provider parity

Provider-agnostic: git/worktree plumbing; no provider session interaction.
