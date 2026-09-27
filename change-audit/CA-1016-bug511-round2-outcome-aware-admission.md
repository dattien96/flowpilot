# CA-1016 — BUG-511 round 2: outcome-aware admission + telemetry veto

## Context

Round-2 review: the first fix (CA-1012) consulted only the durable block
ledger, returned `quota_route_required` for every outcome — including
`QuotaBlocked` which emits no card, so a single-account environment
looped a 409 referencing a card nobody could resolve — and auto-rotate
swallowed the prompt entirely (a committed rotation still returned an
error without dispatching it).

## Changes

- `quota_gate.go`: split `enterQuotaGate` into `resolveQuotaGate` (returns
  the `QuotaResolution` uncommitted) + the existing wrapper. Added
  `pinnedAccountHardVeto(ctx, rs)` — reports the ledger's stored block
  reason, else `quota_exhausted` when live telemetry shows
  `NormalizeAccountHeadroom.State == "exhausted"`. The trigger string is
  a real `ProviderLimitKind` so `ObservedLimit` is set and the pin
  hard-rejects in the candidate pass.
- `interactive_service.go` `startTurn`: the admission veto now unlocks
  `s.mu` before the (potentially slow) telemetry probe, then acts on the
  outcome — same-provider `QuotaRotate` commits the repin and the prompt
  continues onto the healthy account; cross-provider rotate relays the
  turn onto the minted active leg (same pattern as
  `consumePendingContextReset`); flow children return honest
  `quota_route_committed` since `respawnChildOnRoute` owns the
  replacement; `QuotaGate` → `quota_route_required` + card;
  `QuotaBlocked` → `quota_route_blocked` + durable event, no card.

## Tests

3 new cases in `bug511_blocked_account_preflight_test.go` covering
telemetry-only veto, single-account honest block (no card, repeatable),
and auto-rotate repin + prompt continuation. All green.

## Parity

Provider-agnostic — the veto/normalize path is shared; provider deltas
live in `quotaTelemetryFn` (injected).
