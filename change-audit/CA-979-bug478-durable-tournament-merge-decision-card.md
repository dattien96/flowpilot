# CA-979: BUG-478 — durable tournament merge decision card with alternate patches

## Why

Live run-23455: the merge-stage escalate carried only a summary string —
no structured decision card — even though `rs.tournamentPatches` still held
non-empty alternate diffs. The operator had no reachable way to apply an
alternate candidate, and `resumeTournamentChoice("discard")` returned
`unhandled decision choice`. Worse, `tournamentPatches`/`tournamentWinner`/
`decisionCard`/`decisionCardChosen` lived only on `interactiveRun` (RAM): a
restart while parked on the merge card dropped every alternate patch and
the card itself, stranding the run.

## What changed

### Durable state

- `ProviderSessionState` gained `TournamentWinner`, `TournamentPatches`,
  `TournamentAttempt`, `DecisionCard`, `DecisionCardChosen`; the NDJSON
  session record mirrors them as `tournament_winner`, `tournament_patches`,
  `tournament_attempt`, `decision_card`, `decision_card_chosen`.
- `sessionStateOf` and `reconstructRunInternal` round-trip the new fields,
  so a restart rehydrates the parked card and every patch snapshot.

### Merge-stage escalation

- `behaviorTournamentMerge` (escalate path) now renders
  `tournamentMergeDecisionCard`: options are derived ONLY from recorded
  non-empty patches (empty/missing diffs are never offered — they cannot
  merge), plus `retry`, `discard`, `ask`. The card is parked through
  `applyFlowControl{Status:"escalate"}` as `payload.decision_card`, which
  the existing durable path persists and re-emits on resume.

### Choice routing

- `resumeTournamentChoice` gained an explicit `discard` case routed to
  `discardTournamentMerge`: sweeps candidate worktrees, clears stored
  snapshots, terminalizes the loop with an honest "no patch merged"
  summary, and returns an escalation child's parent to manual escalation —
  never a false winner resume.

## Tests

- `bug478_merge_card_alternates_test.go`
  - `TestBUG478_MergeEscalateOffersValidAlternatePatches` — card lists only
    valid alternates plus retry/discard/ask.
  - `TestBUG478_AlternatePatchChoiceAppliesCandidatePatch` — choosing an
    alternate applies its snapshot through the worktree patch seam.
  - `TestBUG478_DiscardTerminalizesHonestly` — discard settles without a
    fake merge.
  - `TestBUG478_RestartPreservesMergeCardAndAppliesOnce` — restart
    rehydrates card + patches; a choice applies the patch exactly once.

## Verification

- `go test ./internal/runner -run 'TestBUG478'` — 4/4 green.
- Tournament/session surface and full-suite results recorded in the BUG doc.
