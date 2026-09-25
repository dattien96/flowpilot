# CA-1001 — BUG-501 worktree git-scope confusion (live-found)

## Scope
`apps/local-runner/internal/worktree/manager.go` — new `requireWorktreeScope`
guard wired into `Untracked`, `Uncommitted`, `Diff`, `Cleanup` (branch probe),
`Validate`. New test `internal/worktree/bug501_scope_confusion_test.go`.

## Finding (live)
Re-running the BUG-472 discard drill on build b500: worktree `.git` file
renamed → `DELETE ?worktree=discard` succeeded silently — run deleted, dir
gone, no 503. Root cause: `git -C <broken-worktree>` walks up to the parent
repo; the worktrees root is gitignored, so `ls-files --others` / `status`
return empty error-free → the dirty-scan evidence gates pass on a wrong-scope
answer → destructive removal proceeds.

## Fix
`requireWorktreeScope` asserts `git rev-parse --show-toplevel` inside the
worktree resolves to the worktree's own path; anything else fails closed:
scans error → 503 `worktree_inspection_failed`; Cleanup never names the
parent's branch for `-D`; Validate reports the binding as lost.

## Verification
- `go test ./internal/worktree/` — green (5 new BUG-501 tests incl. healthy
  controls)
- `go test ./internal/runner/ -run 'Worktree|Merge|BUG47[23]'` — green
- Live re-drill on build b501: broken `.git` → 503 + dir preserved; restore →
  discard → dir removed (see BUG-501 doc).

## Risk
Low — one extra `git rev-parse` per scan; healthy paths unchanged.
