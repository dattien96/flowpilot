# CA-1170 — run-2062497 D8+D10: drift signal quality

## What changed

`apps/local-runner/internal/driftdetect/`:

- `TurnSummary` gains `TestsGreen` — set only when the turn ran a suite with
  at least one pass and zero ordinary failures/regressions.
- `checkZeroDeltaProgress` now requires high token spend AND no changed
  files AND `TestsGreen == false`. Live: a coder turn burned tokens
  re-verifying already-complete work, produced no file delta, and the
  detector scored 100 → pause_for_human → reprompt loop — punishing a green
  turn.

`apps/local-runner/internal/runner/gate_hook.go`:

- `turnSummaryFromTurnResult` populates `TestsGreen` from
  `TurnResult.Tests`.
- The frozen-scope drift reason now splits drifted paths by membership in
  the turn's tool-call writes: paths absent from `fin.ChangedFiles`
  (external writes — a leg bash redirect or a direct operator edit, which
  are indistinguishable in the diff) are named with the hint "not written
  via this leg's tool calls — if these are operator edits, amend the
  contract to sanction them". The gate still fails closed; only the reason
  text distinguishes writer provenance (D10).

## Invariant

Zero file delta with a green suite is completion evidence, not drift; and
when drift does fire, the reason tells the operator which paths bypassed
leg tooling so amend — not another blind retry — is the sanction for their
own edits.

## Tests

`driftdetect_test.go`
(`TestRun2062497_ZeroDeltaExemptWhenSuiteGreen`,
`TestRun2062497_ZeroDeltaStillFiresWithoutGreenSuite`),
`run2062497_gitlink_drift_test.go`
(`TestRun2062497_ExternalWriteDriftHintsAmend`).
