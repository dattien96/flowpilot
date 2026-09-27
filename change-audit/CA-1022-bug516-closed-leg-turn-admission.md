# CA-1022 — BUG-516: closed legs no longer accept new turns

## Context

Grok review round 3: `startTurn` had no leg-state guard — a run whose leg
was CLOSED by a committed route kept dispatching turns on the stale
binding. Live evidence: run-30802's leg was durably closed after an
operator `use_once` grok answer minted run-31122, yet a re-sent turn
completed `PONG-511` on devin — silently bypassing the route.

## Changes

- `interactive_service.go` `startTurn`: admission check — a
  `LegStateClosed` chat leg relays the turn to the chat's active leg
  (same pattern as the BUG-511 cross-provider rotate relay); a closed
  leg with no successor, or a non-chat closed run, returns 409
  `leg_closed`. Placed after idempotent replay + pending-card guards and
  BEFORE the BUG-511 veto (a dead leg's stale pin must not emit a
  spurious route card).

## Tests

`bug516_closed_leg_turn_relay_test.go` — relay to active leg through a
real codex→claude switch; no-successor and non-chat cases 409
`leg_closed`; active legs unaffected.
