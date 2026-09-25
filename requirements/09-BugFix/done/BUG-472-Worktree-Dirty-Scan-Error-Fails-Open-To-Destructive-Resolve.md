# BUG-472: worktree dirty scan errors fail open — keep_branch/discard can destroy uncommitted work

## Metadata

- Document ID: `BUG-472`
- Phase: `bugfix`
- Status: `done`
- Severity: `critical`
- Evidence: `code-confirmed; fixed CA-970; live fault-injection pending`
- Fixed By: `CA-970`
- Tests: `internal/runner/bug472_worktree_dirty_scan_fail_closed_test.go`
- Feature Keys: `run-worktree`
- Parent Documents: `CP-71`, `SS-23`, `SD-27`
- Related Documents: `BUG-378-Worktree-Committed-Work-Dropped-On-Merge`
- Affected Areas: `internal/runner/run_worktree_merge.go`, `internal/worktree/manager.go`

## Summary

`resolveWorktree` intentionally guards destructive `keep_branch` and `discard`
operations by listing work that would be lost. Both guards discard the Git
inspection error:

```go
uncommitted, _ := mgr.Uncommitted(...)
untracked, _ := mgr.Untracked(...)
```

If `git status --porcelain` fails, each result is treated as an empty list and
the destructive cleanup proceeds without confirmation. This violates CP-71's
fail-closed merge decision contract and can cause data loss.

## Evidence

- `keep_branch`: `run_worktree_merge.go:209-218` ignores `Uncommitted` error,
  calls `Cleanup(..., keepBranch=true)`, then stamps `kept_branch`.
- `discard`: `run_worktree_merge.go:223-233` ignores `Untracked` error, removes
  the worktree/branch, then stamps `discarded`.
- `Manager.Uncommitted` and `Manager.Untracked` return real errors when Git
  cannot inspect the worktree. There is no regression test for inspection
  failure.
- Failure can be induced by missing/corrupt worktree registration, permission
  failure, transient filesystem error, or unavailable Git executable.

## Expected vs Actual

- Expected: inability to prove that a worktree is clean blocks destructive
  resolution with a typed retryable error; no cleanup or state transition.
- Actual: inspection failure is interpreted as clean and cleanup proceeds.

## Impact

- `keep_branch`: committed branch history remains, but staged, unstaged and
  untracked work can be deleted with the worktree.
- `discard`: untracked files can be deleted without the mandatory confirmation.
- API may report a successful terminal state after evidence collection failed.

## Required Fix Contract

1. Treat dirty-scan errors as fail-closed and return a typed error.
2. Do not call cleanup or mutate `worktreeState` after inspection failure.
3. Preserve current confirmation behavior when inspection succeeds.
4. Provider-agnostic by construction; verify Desktop and TUI error surfaces.

## Required Tests

- RED: `keep_branch` with `Uncommitted` inspection failure does not call cleanup
  and leaves state `active|merge_pending`.
- RED: `discard` with `Untracked` inspection failure does not call cleanup.
- E2E HTTP: typed non-2xx response is retryable and contains no success state.
- Live fault injection: break worktree registration or Git access and confirm
  files survive.

## Implementation Plan

### P-1 — Reproduce first

- Add `internal/runner/bug472_worktree_dirty_scan_fail_closed_test.go`.
- Introduce a test-only manager seam at `InteractiveService` or a narrow
  package-level constructor so tests can force `Uncommitted`/`Untracked`
  failures without replacing Git globally.
- Assert before production changes that `keep_branch` and `discard` currently
  reach cleanup and terminal state after inspection error.

### P-2 — Make inspection authoritative

- Change `resolveWorktree` in `run_worktree_merge.go` to retain the returned
  error from `mgr.Uncommitted` and `mgr.Untracked`.
- Map inspection failures to one typed API error such as
  `worktree_inspection_failed` with HTTP 503/409; do not expose command stderr
  containing sensitive paths beyond the bounded operator-safe message.
- Return before `Manager.Cleanup`, `setWorktreeState`, event emission or patch
  artifact removal.
- Keep existing `requiresConfirm` wire shapes byte-identical when inspection
  succeeds.

### P-3 — Surface parity

- Desktop `worktreeMergeConfirm` path: retain the original card and show a
  retryable error; never flip to success.
- TUI `/wt-merge`: print the typed error and preserve the pending merge state.
- No provider-specific branching is required.

### P-4 — Verification order

1. Demonstrate both new tests red against current behavior.
2. Land runner fail-closed handling; targeted tests green.
3. Run `go test -count=1 ./internal/worktree ./internal/runner -run 'Worktree|BUG472'`.
4. Run Desktop/TUI worktree suites.
5. Run a real Git fault drill and verify inode/file contents before and after.

## Definition of Done

- [ ] RED tests prove both destructive modes proceeded on inspection failure.
- [ ] Inspection failure performs zero cleanup and zero state transition.
- [ ] Dirty-scan success preserves current confirmation payloads exactly.
- [ ] Desktop and TUI keep the card actionable and expose Retry.
- [ ] Real Git fault drill proves staged, unstaged and untracked files survive.
- [ ] Existing CP-71 lifecycle, keep-branch and discard tests pass unchanged.
- [ ] CA entry records the fail-closed invariant and live evidence.
