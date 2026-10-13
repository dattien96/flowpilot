# BUG-668 — Closed-Leg Relay Picks a Random Stale `legState=active` Leg

## Metadata

- Document ID: `BUG-668`
- Title: `closed-leg relay selects any legState=active sibling in nondeterministic map order — dead relay`
- Phase: `bugfix`
- Status: `FIXED — CA-1237`
- Owner: `flowpilot-runner`
- Created: `2025-01-24`
- Last Updated: `2025-01-24`
- Parent Documents: `BUG-516` (closed-leg relay), `BUG-541`, `SD-26`
- Related Documents: `BUG-666`, `BUG-669`
- Tags: `chat-legs, relay, determinism, severity:high`

## AI Quick View

### Summary

- `startTurn`'s closed-leg relay (`interactive_service.go:12075`) and the quota-rotate relay (`:12134`) pick **the last iterated** leg with `legState==LegStateActive && leg.id != runID`.
- `activeChatLegsLocked` iterates `s.runs` — a Go map → **random order per call**. With >1 stale `active` record (healed incompletely, e.g. completed legs that kept `legState=active`), the relay target is a coin flip.
- Live: turns relayed into a completed grok leg (`run-815559`, `legState=active`), a parked phantom-loop leg (`run-816751`), or a `stopped`-loop leg — each accept-then-die or hard-refuse, with no deterministic way to reach the live leg.

### Current Ask

- Deterministic live-leg selection: among `LegStateActive` siblings, prefer the leg that can actually accept a turn — highest `legSeq` first, excluding terminal/dead states — applied at both relay sites via one shared helper.

### Key Decisions

- `V-1` `legState` is the leg-lifecycle field but alone it is not a liveness guarantee; selection must ALSO consider run status (terminal legs cannot take turns) and order (newest leg wins).
- `V-2` The helper is shared: duplicating selection criteria at each relay site re-introduces drift.

### Source Refs

- live `run-752742` chat `cht_3078718bf299`: legs `run-815559` (completed, legState active), `run-816045` (cancelled), `run-816751` (active, own parked loop), `run-817740` (idle) all `legState=active`; `/turns` on parent relayed randomly.
- `interactive_service.go:12075-12084`, `12132-12139`; `chat_switch.go:78-99`.

## 1. Issue Summary

When multiple chat legs carry `legState=active` — a reachable state because `healChatLegsLocked` closes only one old leg per pass and completed/cancelled legs can keep `active` — the closed-leg relay iterates the resident-runs map and takes whatever comes last. Hub turns die on legs that are completed, parked, or own phantom loops; the operator had to interrupt and retry until the coin flip landed on a live leg.

## 3. Environment and Reproduction

- environment: any chat that accumulated >1 `legState=active` leg (switch chain, reattach chain, heal partial)
- reproduction: seed two `active` legs where the older one is terminal; call `startTurn` on the closed leg repeatedly → relay target alternates between the terminal leg (turn lost) and the live leg.
- frequency: high once the stale-active state exists (per-call random)

## 4. Expected vs Actual

- expected: relay deterministically reaches the newest live leg
- actual: relay target = Go map iteration order — nondeterministic, often a dead leg

## 5. Impact

- severity: high — turns silently die into dead legs; `turn_in_progress` zombies; manual recovery takes hours

## 6. Root Cause

- confirmed cause: selection loops at `interactive_service.go:12075` and `:12134` assign `newLegID = leg.id` for every match → last-in-iteration wins → map-order random. No liveness/status check, no ordering.
- evidence: live relay behavior alternating `409` (dead leg guards) vs acceptance on live leg.

## 7. Fix Strategy

- `F-1` Extract `selectRelayLegLocked(chatID, excludeRunID) *interactiveRun`: among `LegStateActive` legs, skip `runStatusTerminal(leg.status)` and legs whose own loop is `blocked`/`stopped`-terminal when a better candidate exists; pick highest `legSeq`.
- `F-2` Call it from both relay sites (closed-leg relay, quota-rotate relay).
- `F-3` Additive tests: two `active` legs — one `completed`, one `idle` — relay must always pick the `idle` one regardless of map order (assert over N=50 iterations / direct selection call).
