# BUG-414: Tie-card resolution accepted but discarded — `continue` with `candidate-b` re-parks "no task artifact…"

## Metadata

- Document ID: `BUG-414`
- Title: `Tournament tie decision card accepts candidate-b then silently drops the choice — hub re-escalates, winner/merge never routed`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-23`
- Parent Documents: [CP-65-Test-Steps](../../07-Coding-Plan/done/CP-65-Test-Steps.md), [CP-65-Multi-Candidate-Tournament-Harness](../../07-Coding-Plan/done/CP-65-Multi-Candidate-Tournament-Harness.md)
- Feature Keys: `tournament`, `decision-card`, `flow-engine`

## AI Quick View

### Summary

- CP65-4 (run-1): a `user_decision_card_requested` tie card was emitted (options `candidate-a`/`candidate-b`/`retry`, `recommended: candidate-b`, evidence `percent.go:10`, `NeedsHumanDecision=true`, `total=0.5 blast=0.7 each`); the loop parked `blocked`/`escalate`. `POST /agent-loop/continue` with `candidate-b` flipped the loop `running`, re-invoked the hub **with no tournament payload**, and the hub re-escalated `Cannot render a review verdict: no task artifact, acceptance criteria, diff, or candidate contents…` → parked `blocked`/`escalate` again.
- The API accepts the human's tie-break choice but never routes it to a winner/merge path — the answer is silently discarded.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom:** answering the tournament tie card via `continue` produces a second `flow_control_escalate` — the hub receives no candidate selection, no candidate contents, and no task artifact, so it re-parks on the same escalate reason family; `candidate-b` is never applied.
- **Expected:** choosing `candidate-b` routes the selection into the tournament winner/merge path (`tournament.merge`/`WorktreeManager.MergeWinner` or the arbiter retry), or the card should refuse the answer if routing is impossible.
- **Actual:** choice accepted and dropped; the loop oscillates `blocked(escalate)` → `running` → `blocked(escalate)` forever.
- **Impact:** the human-decision escape hatch for tied tournaments is a dead end — same acceptance-without-effect shape as BUG-411 (gate-decision) and BUG-413's unhandled continue. Note the live path is additionally constrained: the real `tournament.arbiter` node is unreachable (tournament-harness entry spawn fails `workflow_has_no_steps`, `interactive_handlers.go:752` — CP65-3 family), so the escalate card is currently emitted via operator `flow-control escalate`.

## Reproduction

1. Emit a tie card (live: parked escalate on run-1 with card payload — `l65-3-run1-events.sse` evt-209; options candidate-a/candidate-b/retry, recommended candidate-b).
2. `POST /agent-loop/continue` with the chosen option (`candidate-b`).
3. Observe loop `running` → hub re-invoked without tournament payload → second `flow_control_escalate` → `blocked`/`escalate` with "no task artifact…" reason.

## Root cause

- The `continue`/gate-answer path does not carry the decision-card selection into the tournament state: the hub is re-invoked with no candidate payload (no selected winner, no candidate contents/diff), so its verdict render fails and it re-escalates. No routing exists from the card's option id to `DecideTournamentAction`'s winner/merge outcome (`tournament_behavior.go:362+,391-397`) or `tournament.merge`.

## Evidence

- `~/fp-beds/lt-evidence/cp65/l65-3-run1-events.sse` evt-209 — full `user_decision_card_requested` payload (`recommended: candidate-b`, `percent.go:10`, tied ranking detail).
- `~/fp-beds/lt-evidence/cp65/l65-3-continue-candidate-b.json` — the accepted answer.
- `~/fp-beds/lt-evidence/cp65/flow-diag-run-1.ndjson` — second `flow_control_escalate` after the answer.
- `~/fp-beds/lt-evidence/cp65/l65-3-final-graph.json`, `l65-3-tie-card-response.json` — terminal `blocked`/`escalate`, gateReason `tournament needs a human: tied ranking…`.
- `~/fp-beds/lt-evidence/cp65/RESULT.md` (BUG-LIVE-CP65-4, L-65-3).

## Severity

`medium` — silent discard of an explicit operator decision; tied tournaments cannot be resolved live even when the card renders.

## Completion Notes (implemented 2026-09-23, CA-923b)

- Root cause (three layers, all live-verified): (1) `decisionCardChosen` was
  captured but never consumed — resume fell through to a generic hub
  reinvoke; (2) the card itself was rejected by `parseUserDecisionCard`
  (`options` built as `[]map[string]any`, parser asserts `[]any` →
  `decision_card_invalid`, live run-2500) so no typed choice could exist;
  (3) the arbiter's escalate cleaned all candidate worktrees at park, so a
  picked candidate hit `no worktree for owner` in merge (live run-3077).
- Fix: `resumeFlowWithFeedback` routes a captured tournament-card choice to
  `resumeTournamentChoice` (candidate → merge, retry → fresh cohort with
  stale-dir cleanup, ask → stays parked, unknown → rejected). Card options
  emit `[]any`. The arbiter snapshots each candidate's patch into the
  escalate payload (`patches`), the run stashes them, and merge applies the
  stored diff via `worktree.Manager.ApplyPatch` — escalate keeps its
  no-orphan cleanup (`TestTournamentTieRequiresHumanDecision` still green).
  Merge-done sweeps loser worktrees. `.flowpilot/` is excluded from
  candidate diffs so runner metadata can't enter a merge patch.
- Tests: `TestBug414_TieCardCandidateChoiceMerges`,
  `TestBug414_TieCardRetryChoiceReattempts`,
  `TestBug414_UnknownTournamentChoiceRejected`,
  `TestBug414_TournamentCardParsesThroughFlowControlValidator`,
  `TestBug414_EscalateCarriesMergeablePatches`,
  `TestBug414_RetryChoiceCleansStaleWorktrees`.
- Live: run-3589/run-4262 — card validated (no `decision_card_invalid`),
  `candidate-b` choice routed into `merge_and_audit`; merge executed and
  escalated with conflict evidence when the patch could not land (empty
  candidate diffs in this bed — see CA-923b caveats).
