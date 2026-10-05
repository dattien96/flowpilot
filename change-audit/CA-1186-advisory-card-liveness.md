# CA-1186 — advisory question cards held member/hub liveness (BUG-1186)

## Defect

`context_pressure_90` and `usage_budget_exceeded` cards set
`pendingQuestionID` without gating the run's work — advisory decisions whose
answer can never produce a machine verdict or unblock the member. Every
liveness/quiet check counted `pendingQuestionID != ""` as real gated work:

- `memberVerdictStillLive` → done-edge missing-verdict escalation deferred
  forever on a member whose turn had already ended (live `run-204891` —
  the reviewer member held an unanswered context-pressure card while the
  sprint needed its verdict resolution);
- `hasActiveFlowChild` → hub watchdog never reached the wedge;
- cohort stall sweep `hasGate` → member skipped as "awaiting user";
- `redriveQuietFlowLoop` quiet checks → armed intents/orphans never ran.

## Fix

New `pendingQuestionGatesWorkLocked` — returns true only when the run's
pending question is a real gate (any kind other than the two advisory kinds;
a dangling `pendingQuestionID` fails closed per BUG-565). Also scans all
records for the run so an advisory card overwriting `pendingQuestionID`
cannot hide an earlier still-open gating question.

Converted the liveness sites: `memberVerdictStillLive` (now a method),
`hasActiveFlowChild` ghost/active checks, cohort stall `hasGate` (both),
hub `busy`/`hasCard` checks, `redriveQuietFlowLoop` quiet checks, and the
orphan-cure gate. Visibility surfaces (`decision_payload`, chat-switch and
handoff guards, resume reconstruction) still count advisory cards — the card
remains user-visible and answerable; it just stops pretending to be work.

## Regression tests

`bug1186_advisory_card_liveness_test.go` — advisory card does not defer the
missing-verdict escalation, does not count as an active flow child; gating
quota card still defers; dangling question id fails closed.
