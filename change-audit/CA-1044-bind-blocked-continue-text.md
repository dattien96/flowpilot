# CA-1044 — Tournament binding skips ledger-blocked accounts; continue accepts `text` feedback (BUG-536/537)

## Summary

Two R6 residuals from the CP-Full-Live-Test review of the run-2634
tournament exercises:

- Tournament candidate binding re-picked a provider whose active account
  carried a durable hard quota block, so every arbiter retry attempt
  spawned a leg that immediately vetoed on admission and re-fired the
  same quota card.
- `agent-loop/continue` decoded only `feedback`; `{"text":"..."}` bodies
  (the option-card contract) were silently dropped and the loop resumed
  empty.

## Changes

- `internal/runner/tournament_escalation.go`
  (`bindTournamentCandidatesToAvailableProviders`): after the
  selectability/connectivity filter, resolve each provider's active
  account and read the durable quota ledger under `quotaMu`; skip
  providers whose account carries a hard block. `quota_exhausted`
  remains admissible — admission telemetry may lift it
  (`quotaBlockLiftedOnHeadroom`), mirroring the pin-veto carve-out.
  Corrupt ledger (`loadErr`) resolves to the fail-closed reason and is
  excluded. Single-provider duplication (BUG-526) unchanged.
- `internal/runner/interactive_handlers.go` (`handleContinueFlow`): body
  gains `Text`; `text` aliases `feedback` when `feedback` is empty.
  `captureDecisionChoice` and `resumeFlowWithFeedback` both consume the
  resolved value.

## Tests

- `TestBug536_BindingSkipsLedgerBlockedProviderAccount` (RED→GREEN):
  blocked grok account + live devin → both candidates bind devin.
- `TestContinueFlowTextFieldAliasesFeedback` (RED→GREEN): `{"text":...}`
  rides into the resumed turn's prompt.

## Verification

- Focused batch `TestBug5xx`/`TestBug430`/`TestBug521`: PASS.
- `-race` on the new tests: PASS. `go vet` + `go build ./...`: clean.
- Broad suite baseline unchanged (env-missing-binary fails only).
