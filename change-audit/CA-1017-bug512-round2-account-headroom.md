# CA-1017 — BUG-512 round 2: account headroom in the reset callback

## Context

Round-2 review: the wired `contextResetHeadroomOK` checked the NODE's
declared `maxUsageTokens` budget — uncapped nodes (the common case)
always passed, so a context reset could mint a same-binding leg onto an
exhausted/blocked account that fails on dispatch. CP-87 P-3b/P-3c
requires the PINNED ACCOUNT's normalized headroom to afford the reseed.

## Changes

- `interactive_service.go`: the production callback now runs
  `pinnedAccountHardVeto` first (ledger-blocked or telemetry-exhausted
  pin → insufficient headroom → existing escalate-to-routing branch),
  then keeps the node-budget check as a second guard for capped nodes.

## Tests

3 new cases in `bug512_headroom_seam_wired_test.go` — ledger-blocked and
telemetry-exhausted pins reject reseed on an uncapped node; healthy pin
passes. Prior budget tests unchanged and green.
