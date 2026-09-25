# CA-970 — BUG-472: worktree dirty-scan errors fail closed before destructive resolve

- type: bugfix
- bug: BUG-472 (code-confirmed; live fault-injection pending)
- feature: run-worktree
- follows: CA-969

## Change

`internal/runner/run_worktree_merge.go` `resolveWorktree`:

- `keep_branch` and `discard` previously discarded the Git inspection error
  (`uncommitted, _ :=` / `untracked, _ :=`), reading a failed `git status` /
  `git ls-files` as an empty list and proceeding to `Cleanup` + terminal
  state — deleting work the scan never proved was safe to lose.
- Both guards now retain the error and return `503
  worktree_inspection_failed` before any cleanup, state transition, event
  emission, or artifact removal runs. The error is retryable: the same call
  resolves once Git is healthy.
- Confirmation wire shapes (`requiresConfirm` + `uncommitted`/`untracked`
  lists, 409 codes) are unchanged when inspection succeeds.

`internal/runner/interactive_service.go` + `run_worktree_merge.go`:

- Added `worktreeOps` — the narrow slice of `worktree.Manager` that
  `resolveWorktree` uses — and an `InteractiveService.wtOps` seam (nil =
  real manager) so tests inject Git faults without breaking the filesystem
  under test.

## Tests (additive only)

- `bug472_worktree_dirty_scan_fail_closed_test.go`:
  - `TestE2EWorktree_KeepBranchInspectionFailureFailsClosed` — RED first
    (pre-fix: injected `Uncommitted` error still produced 200
    `kept_branch`); post-fix: 503 typed error, zero Cleanup calls, files +
    binding state untouched, retry succeeds.
  - `TestE2EWorktree_DiscardInspectionFailureFailsClosed` — same contract
    for `Untracked`/`discard`.

## Provider parity

Provider-agnostic — the seam and the fail-closed guard sit in the shared
resolve path; no adapter/provider branching. Desktop/TUI surface the typed
error through the existing `error.code` channel; the merge card stays
actionable because no state transition occurs.

## Invariant

CP-71/SD-27: inability to prove a worktree is clean blocks destructive
resolution — "could not inspect" is never "clean".
