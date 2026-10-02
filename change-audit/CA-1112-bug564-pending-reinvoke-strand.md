# CA-1112 — BUG-564: armed pendingHubReinvoke drains on any flow; hub_parked no longer burns the retry budget

## Why

Live run-69320 parked `hub_stalled` twice on a plain vibe sprint with a
healthy pending reinvoke stranded underneath:

1. `startTurn` failed with `hub_parked` (children still settling — a
   transient busy window that can last minutes). The failure path counted
   it against `hubReinvokeStartFailCount` AND scheduled a +25ms re-fire —
   three retries burned inside ~1s while children were still settling, then
   `will_drain=false` left `pendingHubReinvoke` armed with nothing to fire
   it (`hub_reinvoke_start_failed` x5 in the live log).
2. The stall watchdog's drain for this exact strand (CA-1091) was scoped to
   runs hosting an owner debate — a plain sprint flow skipped it, so the
   watchdog parked the hub at 2m while owed progress sat armed.
3. Downstream corruption: when the stranded pending eventually fired late,
   the hub read stale state and dispatched duplicate legs (live run-69320
   ran a second coder fix-leg and a third adhoc reviewer over an already-
   converged review).

## What changed

`apps/local-runner/internal/runner/hub_stall.go`:

- `checkAndBlockStalledHub` now drains an armed `pendingHubReinvoke` on ANY
  flow — not only owner-debate flows — before considering `hub_stalled`.
  The drain clears the pending flag, touches hub progress, re-fires via the
  saved prompt (or the bare reinvoke), and re-arms the watchdog.
- The drain is bounded by `hubReinvokeStartFailCount <= 3`: once the
  consecutive-fail budget is genuinely spent the strand is real and
  `hub_stalled` below remains the fail-closed escalation (BUG-289 F-0
  contract preserved — `bug289_test.go` pins the terminal at count 4).

`apps/local-runner/internal/runner/interactive_service.go`:

- `scheduleChildTurn` failure classification: `hub_parked` is transient
  busy — it no longer increments `hubReinvokeStartFailCount` and no longer
  schedules the +25ms re-fire loop. The pending stays armed; the CA-1091
  child-settle drain and the stall tick above own the re-fire.
  `turn_in_progress`/`gate_in_progress` keep the old deferred-drain shape.

## Invariant

An armed pendingHubReinvoke is owed progress, never a stall symptom — the
loop must fire it or spend the bounded retry budget before parking.

## Tests

`bug564_transient_parked_reinvoke_test.go`:

- `hub_parked` start failures don't burn the retry budget
- an armed pending fires after the transient settle window clears
- the stall tick drains an armed pending on a non-debate flow
- after the drain budget is spent, `hub_stalled` still parks fail-closed

`bug289_test.go`: terminal-stall fixture re-pinned to a spent budget
(`hubReinvokeStartFailCount: 4`) so F-0's park stays asserted.
