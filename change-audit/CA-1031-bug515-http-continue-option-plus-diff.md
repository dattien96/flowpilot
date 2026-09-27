# CA-1031 — BUG-515 round 2: HTTP Continue carries both the option pick and the operator diff

Date: 2026-09-26 — review follow-up on BUG-515 (CA-1021 fixed the direct
`resumeTournamentChoice` path; the HTTP entry still dropped the payload)

## Change

`apps/local-runner/internal/runner/sprint_handoff.go`:

- `captureDecisionChoice` now matches a parked decision-card option against
  the **first line** of the continue feedback (id or label, case-insensitive)
  instead of requiring the entire body to equal the option. The remainder of
  the body still flows verbatim into `resumeFlowWithFeedback` →
  `resumeTournamentChoice` → `runTournamentMergeNode` →
  `extractOperatorPatch`, which already tolerates prose before a `diff --git`
  header.

## Why

`handleContinueFlow` has exactly one free-form channel (`feedback`). When the
operator submits the merge-card pick *and* their hand-resolved unified diff
in one message — `"candidate-a\n\n<diff>"` — the old full-string `EqualFold`
match never captured the pick, so `decisionCardChosen` stayed empty and
`resumeFlowWithFeedback` fell through to the generic blocked-resume: neither
the option nor the diff reached the merge node, and the parked card simply
re-parked. The operator's only real way out stayed "discard" — the exact
BUG-515 trap, still reachable through the shipped HTTP surface.

First-line matching keeps the no-guess fallback: prose ("candidate-a is
risky, retry instead") matches nothing, and a bare single-line pick behaves
exactly as before.

## Tests

- `TestBug515_HTTPContinueCarriesOptionAndOperatorDiff` — real
  `POST /client/workflow-runs/{id}/agent-loop/continue` with
  `{"feedback":"candidate-a\n\n<resolved diff>"}` on a conflict-parked merge
  card; asserts the operator content lands in the workspace and the loop
  settles `done`. Verified RED pre-fix (diff never applied).
- `TestBug515_OperatorResolvedDiffOverridesStoredPatch`,
  `TestBug515_ProseFeedbackKeepsStoredPatch` — unchanged, still green.

## Risk

Low: the only new match region is a first line equal to an option id/label —
messages that previously captured still capture; prose-only submissions are
unchanged. A first line that is exactly an option id followed by free text is
now treated as a pick (the intended desktop submit shape).
