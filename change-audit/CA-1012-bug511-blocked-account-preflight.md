# CA-1012 — BUG-511: blocked pinned account gated at turn admission

## Context

Deep review B-1: `noteAccountBlockedLocked` writes the durable blocked
ledger and the rotation claim path reads it, but nothing consulted it
before provider dispatch — a run pinned to a blocked account burned a
turn that could only fail the same way before the post-failure
`enterQuotaGate` ran (live R2: three concurrent Devin runs dispatched
onto the sole account, zero preflight).

## Changes

- `startTurn` (`interactive_service.go`): at the admission boundary —
  after idempotent replay, after the pending-card guard, before dispatch
  — a run whose pinned `providerAccountID` is in the durable blocked
  ledger enters `enterQuotaGate(..., "account_blocked")` and returns
  `quota_route_required` (409). Child turns ride the same seam.
- `bug511_blocked_account_preflight_test.go`: blocked pin → quota card +
  409, no dispatch; unblocked pin → no card; unpinned → no card.

## Verification

`go test -count=1 -run TestBug511 ./internal/runner/` — green.
