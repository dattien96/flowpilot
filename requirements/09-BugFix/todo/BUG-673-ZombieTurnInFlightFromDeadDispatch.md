# BUG-673 — Zombie `turn_in_progress`: Dead Dispatch Pins turnInFlight Forever

## Metadata

- Document ID: `BUG-673`
- Title: `turnInFlight pinned on a dead dispatch — every new turn/relay rejected until operator interrupt`
- Phase: `bugfix`
- Status: `FIXED — CA-1238`
- Owner: `flowpilot-runner`
- Created: `2025-01-24`
- Parent Documents: `BUG-653` (dead-dispatch sweep), SD-24 dispatch ledger
- Tags: `dispatch, turn-lifecycle, sweep, severity:high`

## Evidence (live run-752742, turn-816726)

- A minted turn recorded the prompt but produced no transcript — the dispatch
  driver died between `prepared` and provider acceptance.
- `rs.turnInFlight` stayed true for ~5h; every `/turns` POST and relay
  returned `turn_in_progress` until an operator `interrupt` cleared it.

## Root Cause

- `unstickSettleResidueLocked` only runs for terminal+settle-owed records —
  a dispatch that dies *before* terminal (prepared/send_claimed/send_started)
  is never enumerated by `ListRecoverable`, so the flag stays pinned forever.
- `turnInFlight` had no dead-driver detector of its own.

## Fix (fail-closed)

- `sweepWedgedFlowWork` collects `turnInFlight` runs with a `currentTurnID`
  and no live gate; `maybeReleaseZombieTurn` releases ONLY when the durable
  dispatch record proves death:
  - record terminal, or
  - record stuck in a pre-acceptance state ≥ `zombieTurnPreAcceptanceBound`
    (10m; the prepared→accepted hop is ms-scale when alive), or
  - record absent on a V2-active run past the bound (prepare writes the
    record before the flag arms — absence + age = lost).
- `provider_accepted`/`uncertain` records are never touched — recovery and
  settle own that lane; a fresh pre-acceptance record keeps the flag (the
  concurrent-send protection is not weakened).
- Release persists the session row and `notifyTurnIdle` so queued turns drain.

## Regression Tests

- `bug673_zombie_turn_test.go` — stale-prepared releases, terminal releases,
  fresh-prepared keeps, provider-accepted keeps (protection intact).
