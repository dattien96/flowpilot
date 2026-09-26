# BUG-512 — `contextResetHeadroomOK` callback nil in production; CP-87 P-3b branch dead

## Status
FIXED — 2026-09-26 (CA-1013).

## Found during
Deep review B-2 (`CP-Deep-Review-12CP-2026-09-26.md`), verified against
source: the only assignment to `contextResetHeadroomOK` was in test files.

## Observed

The context-reset path treats `contextResetHeadroomOK == nil` as
"sufficient headroom". Production never wired the callback, so the
CP-87 P-3b branch — reseeding onto the same binding only when the reset
won't blow the node budget, otherwise routing to the quota gate — could
never fire in production. Every context reset reseeded unconditionally,
even when the reseed would exceed the budget it was designed to respect.

## Fix (CA-1013)

`newInteractiveService` wires a default implementation: under `s.mu` it
reads the run's current node usage (`nodeUsageTokensLocked`) and a
cheap reseed estimate (`len(lastPrompt)/4`), then compares
`used + estimate` against `nodeUsageBudgetFor(rs)` — uncapped nodes
(budget ≤ 0) always pass; capped nodes must fit strictly under budget.

## Tests

`bug512_headroom_seam_wired_test.go` — production seam non-nil; uncapped
node → true; under-budget → true; over-budget → false (routes to quota
gate per P-3b). Green.

## Round 2 (2026-09-26, CA-1017) — account headroom, not node budget

The first fix wired the callback but checked `nodeUsageBudgetFor` —
nodes without `maxUsageTokens` (the common case) always passed, so an
exhausted pinned account still got a same-binding reseed that could only
fail on dispatch. CP-87 P-3b/P-3c requires the PINNED ACCOUNT's quota
state to afford the reseed.

Round 2: the default `contextResetHeadroomOK` now consults
`pinnedAccountHardVeto` first — ledger-blocked or telemetry-exhausted
pin → `false`, the reset escalates into the routing gate. The node-budget
check stays as a second guard for capped nodes; uncapped nodes now honor
the account check.

## Tests (round 2)

- `TestBug512_LedgerBlockedPinRejectsReseedOnUncappedNode`
- `TestBug512_TelemetryExhaustedPinRejectsReseedOnUncappedNode`
- `TestBug512_HealthyPinPassesOnUncappedNode`
