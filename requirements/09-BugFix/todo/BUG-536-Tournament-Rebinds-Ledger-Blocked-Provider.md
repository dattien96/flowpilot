# BUG-536 — Tournament binding re-picks a ledger-blocked provider every retry round

Status: FIXED (unit-verified red→green; live-verified 2026-09-28 — grok
ledger-blocked tournament spawned devin×2 candidates, evidence in the
BUG-538 doc's live section and CP-Full-Live-Test R14)
Filed: 2026-09-27 (R6 residual — review of run-2634 tournament retries)
CA: CA-1044

## Symptom (live run-2634, /tmp/fp-live3)

Grok's active account carried a durable `billing_required` ledger block.
Every tournament retry attempt still bound `grok-4.5` to candidate-a:
`bindTournamentCandidatesToAvailableProviders` selected providers on
registry selectability + account connectivity only — it never consulted
the quota ledger. The spawned leg then vetoed on admission
(`pinnedAccountHardVeto`) → parked with a fresh quota card → arbiter
retry → re-bind → same card again. One wasted veto card per attempt,
fail-closed but useless work and a noisy surface.

## Root cause

`bindTournamentCandidatesToAvailableProviders`
(tournament_escalation.go) enumerated `registry.Selectable` providers
with a connected account, then mapped candidates over `available` in
order. The durable quota ledger (`blockedAccounts`) was only consulted
later at per-leg admission (`quota_gate.go`), so binding was blind to
blocks it would certainly trip on.

## Fix (CA-1044)

After selectability/connectivity, binding now resolves each provider's
active account (`activeAccountForProvider`) and reads the durable ledger
under `quotaMu`; a provider whose active account carries a hard block
(`billing_required`, `credits_exhausted`, corrupt-ledger fail-closed
reason) is skipped. `quota_exhausted` is deliberately still admissible:
telemetry can lift that block at admission (`quotaBlockLiftedOnHeadroom`),
same carve-out as the pin veto. Single-provider fallback (BUG-526
diversity-preferred-not-required) is preserved — both candidates land on
the surviving provider instead of re-binding the blocked one.

## Tests

`internal/runner/review_followup_tournament_test.go` —
`TestBug536_BindingSkipsLedgerBlockedProviderAccount`: grok account
connected but ledger-blocked (`billing_required`), devin connected —
binding must put both candidates on devin and bind nothing to grok.
Verified RED pre-fix (candidate-a bound `grok-4.5`), GREEN after.
