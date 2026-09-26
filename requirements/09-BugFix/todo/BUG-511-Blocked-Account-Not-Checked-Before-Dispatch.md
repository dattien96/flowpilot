# BUG-511 — Pinned account recorded as blocked is never consulted before provider dispatch

## Status
FIXED — 2026-09-26 (CA-1012).

## Found during
Deep review B-1 (`CP-Deep-Review-12CP-2026-09-26.md`), verified against
source. Nuance vs the review: `accountBlockReason` IS read on the
rotation claim path — the dead part is the thin `accountBlocked()`
wrapper plus the missing pre-dispatch check on the *initial* pin.

## Observed

`noteAccountBlockedLocked` records live-observed hard limits into the
durable quota ledger, and `SameProviderCandidates`/`ResolveQuotaPreflight`
consult it — but nothing consulted it *before dispatch*. A run already
pinned to a blocked account burned a turn that could only fail with the
same limit error before the post-failure `enterQuotaGate` ran. Live tell
in R2: three concurrent Devin runs all dispatched onto the sole account
with no preflight at all.

## Root cause

`startTurn` admitted turns straight to provider dispatch; the quota
preflight only existed downstream of a *newly observed* failure. The
durable blocked ledger had no admission-time reader.

## Fix (CA-1012)

At the turn-admission boundary in `startTurn` — after idempotent replay
and the pending-card guard, before dispatch — a run whose pinned
`providerAccountID` is recorded blocked enters the quota routing gate
(`enterQuotaGate`) and the caller gets a `quota_route_required` 409
instead of a provider call. Child turns ride the same seam. Unpinned
runs and unblocked pins are untouched; idempotent replays short-circuit
earlier.

## Tests

`bug511_blocked_account_preflight_test.go` — blocked pinned account →
quota card + `quota_route_required`, no dispatch; unblocked pin → no
quota card; unpinned run → no quota card. Green.

## Round 2 (2026-09-26, CA-1016) — review follow-up closed the claim

The first fix consulted only the durable block ledger, returned the same
409 for every outcome, and let auto-rotate swallow the prompt. Round 2:

- `pinnedAccountHardVeto` now reports a veto for ledger-blocked OR
  telemetry-exhausted pins (`NormalizeAccountHeadroom.State ==
  "exhausted"`), so a pin that never got `noteAccountBlockedLocked` is
  still caught pre-dispatch.
- `resolveQuotaGate` (the resolve half of `enterQuotaGate`) returns the
  `QuotaResolution` to the caller; `startTurn` now acts on the outcome:
  - `QuotaRotate` same-provider → committed repin rebinds THIS leg and
    the prompt still dispatches (no swallowed prompt);
  - `QuotaRotate` cross-provider → turn relays onto the freshly minted
    active leg (`startTurn` recursion, same pattern as
    `consumePendingContextReset`); flow children rely on
    `respawnChildOnRoute` and get an honest `quota_route_committed` 409;
  - `QuotaGate` → `quota_route_required` 409 + durable route card;
  - `QuotaBlocked` → `quota_route_blocked` 409 + `quota_route_blocked`
    event, NO invented card — a single-account environment fails honestly
    instead of looping a 409 that references an unresolvable card.
- The veto string is the ledger's stored block reason or
  `quota_exhausted` for telemetry — a real `ProviderLimitKind`, so
  `ObservedLimit` is set and candidates hard-reject the pin.

## Tests (round 2)

- `TestBug511_TelemetryExhaustedPinEntersGateWithoutLedger` — 0%
  remaining telemetry, no ledger entry → route card.
- `TestBug511_SingleAccountBlockedFailsHonestlyNoCard` — one account →
  `quota_route_blocked`, no card, `quota_route_blocked` event emitted;
  second turn repeats honestly.
- `TestBug511_AutoRotateRepinsAndContinues` — auto mode + healthy
  alternate → committed repin to `cx-1`, `quota_route_committed` event,
  dispatch proceeds (turn-2 logged) — the prompt is not swallowed.
