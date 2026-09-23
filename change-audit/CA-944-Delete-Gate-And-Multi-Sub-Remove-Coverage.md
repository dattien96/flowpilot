# CA-944 — Coverage: worktree delete-gate + multi-subscriber remove

## Summary

Review of the three CP-81/83/84 seams found two test-coverage gaps (no code
defects). Additive tests only — no production change.

- `TestE2EWorktree_DeleteGateBlocksAndInlineResolves`: DELETE on a run whose
  binding is `merge_pending` must 409 `worktree_merge_pending`; DELETE with
  `?worktree=discard` resolves inline then deletes (SS-23 E-3 / AGENTS §4 —
  the data-loss guard previously had zero coverage).
- `TestRunUpdates_TerminalRemoveReachesEverySubscriber`: remove bookkeeping
  is per-subscriber (`sub.removed`); both subscribers of a terminal lane must
  each receive exactly one remove (live-verified earlier, now pinned).

## Verified

`go test ./internal/runner/ -run 'TestE2EWorktree_DeleteGateBlocksAndInlineResolves|TestRunUpdates_TerminalRemoveReachesEverySubscriber'` — 2/2 pass.
