# CA-956 — BUG-459: stash tournament patch snapshots on every arbiter verdict

## What changed

- `internal/runner/tournament_dispatch.go`: the `rs.tournamentPatches`
  stash moved above the verdict switch in `runTournamentArbiterNode` —
  previously it ran only in the escalate/ask branch. Auto-pick (`done`)
  now records snapshots too, so `runTournamentMergeNode` sees
  `patchRecorded=true` and an empty winner patch escalates explicitly
  (BUG-453 contract) instead of falling back to the live-worktree
  `MergeWinner` path where an empty diff returns success.
- `internal/runner/bug446_453_tournament_test.go`: new
  `TestBug459AutoPickedEmptyWinnerPatchEscalates` — real repo, real
  worktrees, stubbed probes; asserts the stash and that the merge parks
  rather than settling done.
- `internal/runner/tournament_e2e_test.go`: new `tournamentE2EGreenRepo`
  helper (green-baseline twin of `tournamentE2EBugRepo`).

## Why

Live `run-20041` (post-BUG-458 build): arbiter auto-picked `candidate-b`
whose worktree was untouched — merge reported "tournament winner
candidate-b merged" and the flow completed, but no code ever landed in the
main workspace. The escalate-on-empty-patch path existed (BUG-453) but was
only reachable via the human decision card because the auto-pick branch
never stashed the snapshots.

## Scope / parity

- In-memory run-state field only; no schema or payload shape changes.
- Provider-agnostic: dispatch-layer bookkeeping, no adapter touched.
- Retry safety: a retry's next verdict overwrites the map.

## Evidence

- RED: `TestBug459AutoPickedEmptyWinnerPatchEscalates` failed on HEAD
  (`tournamentPatches` absent after auto-pick; merge settled done).
- GREEN: same test passes post-fix.
- `go test -count=1 ./internal/runner -run 'Tournament|Bug44|Bug45|Bug41[0-9]'`
  — 16s, all green.
