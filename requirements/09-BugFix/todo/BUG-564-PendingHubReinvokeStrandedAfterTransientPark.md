# BUG-564 — Armed `pendingHubReinvoke` stranded after transient `hub_parked` retries; stall watchdog parks the run

- **ID:** BUG-564
- **Severity:** High (run parks spuriously mid-sprint; cancels in-flight turns)
- **Status:** open
- **Found:** live run-69320 (Task-024 sprint), 2026-10-02 13:55–13:58

## Symptom

At 13:55:51 a debate cohort join scheduled a hub reinvoke. `startTurn`
failed with `hub is parked while flow children are running or waiting;
retry after children settle` (`hub_parked`, transient) — children were
still settling. The retry drain fired **3 times in the same second**
(13:55:52), all hitting the identical still-settling state; the
`hubReinvokeStartFailCount <= 3` cap then stopped draining with
`pendingHubReinvoke` still armed.

Subsequent notifies at 13:56:13 and 13:56:41 were only `deferred`
(`hub_reinvoke_deferred` / `hub_notify_reinvoke_deferred`) — nothing ever
re-fired the armed pending reinvoke. At 13:58:49 the hub stall watchdog
correctly detected "no progress for 2m0s" and parked the flow, cancelling
in-flight turns (synthesis step → WAITING_USER_APPROVAL).

## Log evidence

```
13:55:51  cohort join → hub_reinvoke_scheduled
13:55:52  hub_reinvoke_start_failed ×5  err=hub_parked transient_busy
          (drains 1–3 will_drain=true; 4–5 will_drain=false → stranded)
13:56:13  flow_advance_no_targets → hub_notify → reinvoke_deferred
13:56:41  debate_synthesis DONE → hub_notify_reinvoke_scheduled → deferred
13:56:48  hub_notify_reinvoke_scheduled
13:57:03  flow_control_stale_hub_done (consumed, no dispatch — CA-1087)
13:58:49  hub_stalled → flow_parked_awaiting_user
```

## Defect

The transient-failure drain budget is spent **while the busy condition
cannot possibly clear** (children settling takes seconds–minutes; all 3
drains land within ~1s). Once the cap is hit, `pendingHubReinvoke` stays
armed forever — later notifies defer instead of draining — so the only
exit is the stall watchdog parking the run.

## Expected fix direction

One of:
- Exponential/extended backoff for `hub_parked` transient retries (the
  deferred `notifyTurnIdle` at +25ms is far too short for a
  children-settling window), or
- Don't count `hub_parked` attempts against `hubReinvokeStartFailCount`
  (transient busy is not a real failure), or
- Re-drain `pendingHubReinvoke` when the child-settle gate clears (hook
  the settle completion → `notifyTurnIdle`), so an armed pending can
  never outlive the busy window silently.

Also audit `hub_reinvoke_deferred` at 13:56:13/41: why did it not drain
the armed pending once children had settled?

## Follow-on damage observed (same run, 15:03–15:05)

The same stranded-reinvoke pattern recurred at 15:03:04 (5×
`hub_reinvoke_start_failed`, `hub_parked`). When a later `hub_notify`
finally drained the armed pending at 15:05:07, the re-driven hub read
**stale** state (reviewer verdict `changes_requested`) and dispatched a
SECOND coder fix-leg — while the continue back-edge had already re-driven
and completed the rework (`run-82767` turn-90699) and a fresh reviewer
leg was already running. The duplicate fix-leg first spawn attempt failed
on `no connected local account for provider "claude"` (step FAILED), then
respawned on devin (`run-91649`) — a redundant coder leg racing the live
reviewer. So a stranded pending reinvoke doesn't just strand — when it
finally drains it can act on outdated state and duplicate work.

Later in the same run the pattern repeated as **triple reviewers**: at
15:09 the official `flow-auto-validate-round-2` cohort (run-91475 +
run-92500, `join: all`) ran alongside a hub-adhoc reviewer (`run-92706`,
cohort `flow-adhoc-reviewer`) the re-driven hub spawned on top — 3
concurrent reviewer legs on one sprint. Join dedupes by label so the
verdict is not corrupted, but the duplicate legs waste provider calls and
can read the tree while a stale-dispatched fix-leg is writing it.

## Repro sketch

Fixture: parent run with `pendingHubReinvoke` armed, hub state
`hub_parked` while a child leg is RUNNING; fire `scheduleChildTurn` →
expect retry scheduled, not counted-or-cap-bypassed; settle the child →
assert the pending reinvoke fires without a watchdog park.
