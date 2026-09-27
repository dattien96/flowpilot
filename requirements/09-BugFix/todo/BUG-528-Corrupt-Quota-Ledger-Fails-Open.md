# BUG-528 — Corrupt quota ledger fails open as an empty ledger

## Severity

Important — durable routing safety is discarded on corruption.

## Evidence

- `apps/local-runner/internal/runner/quota_claim.go:227-239`
- `apps/local-runner/AGENTS.md:27-35`

`loadQuotaRuntimeState` ignores JSON unmarshal errors and returns a zero ledger. Active claims, cooldowns and blocked accounts therefore disappear from routing decisions when the file is partially written or corrupt. The runner contract requires corrupt durable state to fail closed into an explicit repair state, never zero-value resume.

A related crash window exists between the durable ledger write and `pinRunAccount`: the code comment says a non-resident run re-derives its pin from the ledger, but reconstruction only restores the session's `ProviderAccountID`, `AccountPinned` and `QuotaClaimID`; no ledger-based re-derivation was found.

## Impact

A restart can route back onto a blocked/exhausted account, overbook an active claim, or leave a durable claim that the owning run never uses.

## Missing regression

1. Start with malformed ledger JSON and assert typed repair/fail-closed behavior with zero provider dispatch.
2. Fault after `saveQuotaRuntimeState` but before session pin persistence, restart, and assert the run and claim reconcile to one consistent binding.

## Scope

Capture only. No fix applied during the 2026-09-27 branch review.
