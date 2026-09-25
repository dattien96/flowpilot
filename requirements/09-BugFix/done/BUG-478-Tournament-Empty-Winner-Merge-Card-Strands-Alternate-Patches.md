# BUG-478: empty auto-winner merge escalation strands valid alternate candidate patches

## Metadata

- Document ID: `BUG-478`
- Phase: `bugfix`
- Status: `done`
- Severity: `high`
- Evidence: `live-confirmed run-23455; code-confirmed`
- Feature Keys: `agent-flow-engine`
- Parent Documents: `CP-65`
- Related Documents: `BUG-414`, `BUG-459`, `BUG-460`
- Affected Areas: `internal/runner/tournament_dispatch.go`, `tournament_behavior.go`

## Summary

When the arbiter auto-picks a candidate whose recorded patch is empty,
`behaviorTournamentMerge` correctly escalates instead of reporting a false
merge. However the merge-stage escalation payload contains only
`{winner, reason: empty_patch}`. It does not rebuild a tournament decision card
with the other recorded candidate patches. The operator cannot select the valid
losing patch, retry the cohort, or discard from that card; the run strand-parks.

## Evidence

- Live run-23455: empty candidate-b was auto-picked; candidate-a had a non-empty
  patch; merge parked with “no mergeable diff”; operator had to abandon.
- `runTournamentMergeNode` wraps only `out.Payload` under
  `tournament_merge` when merge returns escalate.
- Candidate patches still exist in `rs.tournamentPatches`, but are not included
  as actionable options.
- `tournamentDecisionCard` is created for arbiter escalation, not merge-stage
  empty-patch escalation.

## Expected vs Actual

- Expected: merge-stage failure exposes every still-valid recorded patch plus
  retry/discard/ask choices.
- Actual: useful alternate patches are unreachable through the parked surface.

## Impact

A successful candidate implementation can become operationally unusable due to
a bad automatic winner. Work is not deleted immediately, but the flow cannot
complete without out-of-band recovery or abandonment.

## Required Fix Contract

1. Merge-stage escalation must carry a structured decision card.
2. Options must be derived only from successfully snapshotted patches.
3. Candidate choice applies the stored patch without requiring cleaned
   worktrees; retry remains bounded.
4. Replayed choices must be idempotent and durable across restart.

## Required Tests

- RED: empty winner + non-empty alternate renders alternate candidate option.
- Selecting alternate lands its stored patch and completes.
- Retry starts a bounded fresh cohort; discard terminalizes honestly.
- Restart while parked preserves options and accepts one choice once.
- Re-run the live run-23455 shape.

## Implementation Plan

### P-1 — RED decision-card contract

- Extend `bug446_453_tournament_test.go` or add `bug478_*` tests with an empty
  winner patch and one valid alternate patch in `rs.tournamentPatches`.
- Drive `runTournamentMergeNode`, not only `behaviorTournamentMerge`, and assert
  the current parked payload has no actionable alternate candidate.
- Add restart round-trip coverage for stored patches/card state.

### P-2 — Build a merge-recovery card

- Add one structured card builder for merge-stage outcomes. It receives the
  recorded patch map and filters to successfully captured, non-empty patches.
- Options: each valid alternate candidate, bounded `retry`, explicit `discard`,
  and optional `ask`; never offer the empty/failed snapshot as mergeable.
- Persist the card and patch references before parking; payload must be bounded
  and must not inline full patches into UI events.

### P-3 — Route each choice

- Candidate choice updates `tournamentWinner` and re-enters the existing
  worktree-free stored-patch merge path.
- Retry increments the existing bounded attempt state and creates a fresh
  cohort; it must not reuse old candidate worktrees.
- Discard terminalizes with an honest no-merge outcome and cleans snapshots.
- Duplicate choice/replay after restart returns the committed outcome rather
  than applying a patch twice.

### P-4 — Live verification

- Recreate run-23455 topology with one real patch and one empty higher-scored
  candidate. Pick the alternate from the merge card and verify only its diff
  lands, tests pass and worktrees clean up.

## Definition of Done

- [ ] RED test proves valid alternate patch was unreachable before the fix.
- [ ] Merge-stage card lists every and only valid recorded patch.
- [ ] Alternate selection applies once and reaches terminal DONE.
- [ ] Retry remains bounded and discard is explicit/audited.
- [ ] Card and choice survive restart without duplicate patch application.
- [ ] BUG-414/453/459/460 tournament suites remain green unchanged.
- [ ] Live run-23455 equivalent completes via alternate candidate selection.
- [ ] CA entry records decision ownership and cleanup ordering.
