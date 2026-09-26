# BUG-516 — Closed leg accepts new turns (committed route silently bypassed)

Status: FIXED — unit + LIVE-VERIFIED on the round-3 binary (:19400):
`POST /turns` on closed run-30802 → `turn-44192` dispatched on the
minted grok leg run-31122 → `PONG-516` settled on grok. The closed leg
no longer steals turns; the use_once card-resolve arm is proven
end-to-end.
Filed: 2026-09-26 (Grok deep-review round 3)
CA: CA-1022

## Symptom

`startTurn` never checked `rs.legState`. A run whose leg was CLOSED by a
committed route (provider switch, quota `use_once`, context reset) kept
accepting new turns and dispatched them on the stale binding — silently
bypassing the operator's routing decision.

## Live evidence

run-30802 (devin) pinned a ledger-blocked account → 409
`quota_route_required` + route card → operator answered `use_once` grok →
`switchChatLeg` minted run-31122 and durably closed run-30802's leg. A
turn then re-sent to run-30802 (the natural target — the card lived
there) completed `PONG-511` on devin: the closed leg ran the turn on the
old provider while the minted grok leg sat idle.

## Root cause

Leg lifecycle is enforced on the switch side (Phase C sets
`legState=LegStateClosed`) but turn admission had no leg guard at all —
any run in `s.runs` could dispatch regardless of leg state.

## Fix (CA-1022)

`startTurn` admission: `rs.legState == LegStateClosed` →

- chat leg with an active successor → relay the turn onto it
  (same pattern as the BUG-511 cross-provider rotate relay);
- closed leg with no active successor → 409 `leg_closed`;
- non-chat closed leg (dead flow child) → 409 `leg_closed`.

Placed after idempotent-replay and pending-card guards but BEFORE the
BUG-511 quota veto — a closed leg's stale pin must not emit a spurious
route card on a dead leg.

## Tests

`bug516_closed_leg_turn_relay_test.go` —

- `TestBug516_TurnOnClosedLegRelaysToActiveLeg`: real codex→claude
  switch closes leg 1; turn sent to the old run lands on leg 2.
- `TestBug516_ClosedLegWithoutSuccessorFailsHonestly`: closed leg, no
  successor → 409 `leg_closed`.
- `TestBug516_ClosedNonChatLegFailsHonestly`: closed non-chat run → 409
  `leg_closed` (replacement run owns the work).
- `TestBug516_ActiveLegUnaffected`: active legs dispatch normally.
