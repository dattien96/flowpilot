# BUG-522 — Tournament worktree failure falls back to the main workspace

## Severity

High — worktree isolation and merge correctness.

## Evidence

- `apps/local-runner/internal/runner/tournament_dispatch.go:54-81`
- `apps/local-runner/internal/runner/tournament_dispatch.go:341-368`
- Live `run-4416`: `thing.go` and `thing_test.go` appeared in `/tmp/fp-live-ws` before arbitration, causing the winner patch to conflict against files already present in the main workspace.

`tournamentCandidateWorktree` logs a `WorktreeManager.Create` error and returns an empty path. `spawnChildRun` treats an empty `WorkspaceCwd` as inheritance from the parent, so the candidate continues in the main workspace instead of failing closed. Candidate prompts are also composed using the parent `cwd` before the candidate worktree path is resolved.

## Impact

Parallel candidates can overwrite each other or modify the user's checkout before a winner is selected. Arbitration may score an empty/stale worktree and merge evidence no longer matches the edits on disk.

## Missing regression

Force candidate worktree creation to fail and assert:

1. No candidate provider turn is dispatched.
2. No child inherits the main workspace.
3. The flow parks with a typed, durable failure/card.
4. No file appears in the main workspace.

Also assert that the candidate prompt and provider turn cwd both reference the created candidate worktree, never the parent workspace.

## Scope

Capture only. No fix applied during the 2026-09-27 branch review.
