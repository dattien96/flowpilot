# BUG-533 — tournament candidate worktrees orphaned on pre-spawn abort paths → permanent "already exists" wedge on retry

**Status:** fixed (CA-1042). Red test → minimal fix → focused/package/race/vet green → live-verified on `/tmp/fp-live3` (runner :4319, devin/grok).
**Found:** code review + live evidence shape (run-1504→run-1 on `/tmp/fp-live-devin`: orphaned `candidate-*` worktree wedged every subsequent spawn with "already exists" until manual `git worktree remove`).

## Reproduction

1. Tournament flow mounts; `spawnTournamentCandidates` iterates candidates.
2. Candidate-a's worktree is created and appended to `spawns`.
3. Candidate-b then fails EITHER the provider `Selectable` check OR the
   connected-account check (account disconnected mid-flow, binding TOCTOU
   after `bindTournamentCandidatesToAvailableProviders`), OR candidate-a's
   `spawnChildRun` fails in the spawn loop (`continue` leaves its worktree).
4. The function escalates and returns — `spawns` worktrees are never cleaned.
5. Operator answers the escalation → resume re-dispatches the rollout node →
   `spawnTournamentCandidates` runs again → `tournamentCandidateWorktree` →
   `WorktreeManager.Create` fails: `worktree for owner "candidate-a" already
   exists` → the worktree-failure branch cleans only THIS call's `spawns`
   (empty at the failure point) → escalate → **wedge repeats forever**.
   Only manual `git worktree remove` unblocks.

## Root cause

`spawnTournamentCandidates` in
`apps/local-runner/internal/runner/tournament_dispatch.go` (~lines 348-447)
cleans `spawns` only on the worktree-creation-failure branch. Three other
abort paths skip cleanup:

- `registry.Selectable(provider)` error → escalate + return
- no connected account for provider → escalate + return
- `spawnChildRun` error → `continue` (per-candidate worktree survives)

`WorktreeManager.Create` deliberately fails on an existing owner dir
(BUG-522's fail-closed fix), so an orphaned dir permanently blocks the same
candidate id. The decision-card retry path (`resumeTournamentChoice`,
~line 810) does clean candidate ids before re-spawn — but a plain escalate
park never produces that card, so that cleanup is unreachable from the wedge.

## Fix (CA-1042)

`spawnTournamentCandidates` in `tournament_dispatch.go` now tracks which
candidate ids actually spawned (`spawned` map). Every abort path runs
`cleanupCreated()`: provider `Selectable` failure, missing connected
account, and a post-loop orphan sweep over `created − spawned` covers
`spawnChildRun` refusals, non-advancing parent breaks, and unattempted tail
candidates. Stale on-disk dirs (planted or crashed leftovers) hit the
worktree-creation error branch, which already sweeps the owner dir and
retries `Create` once before escalating — a leftover that is not a
registered git worktree falls back to `os.RemoveAll` inside
`WorktreeManager.Cleanup`.

## Regression tests

- `TestBug533_AbortCleansCreatedCandidateWorktrees` — candidate-a worktree
  created, then candidate-b aborts on provider check → both dirs swept.
- `TestBug533_StaleDirIsSweptAndSpawnProceeds` — planted non-worktree dir
  at the owner path → sweep → `Create` retry → spawn proceeds (idempotent).

## Live evidence (/tmp/fp-live3, runner :4319)

- Planted `candidate-candidate-a/` stale dir (marker file, not a git
  worktree) before starting tournament-harness on `run-2634` → rollout
  spawn swept it and created a real registered worktree; candidate-a
  spawned and ran. Same sweep repeated for the retry cohort (attempt-1
  worktrees cleaned before re-spawn).
- Pre-fix shape: run-1 needed manual `git worktree remove` on
  `/tmp/fp-live-devin`.

## Evidence

- `tournament_dispatch.go` spawn loop: cleanup exists only in the
  `tournamentCandidateWorktree` error branch.
- `worktree_manager.go` Create doc: "fails when ... the worktree already
  exists".
- Live: run-1504's cancelled-cohort worktrees wedged run-1's spawn until
  manually removed — same wedge mechanism, different orphan source.
