# CA-1188 — closed-leg relay + transient refusal poisons the hub reinvoke channel (sprint deadlock)

## Evidence (live run-204891 / leg run-225672, CP-03 Task-039 sprint, 2026-10-05)

```
06:23:32  rotate_leg on sprint hub → run-204891 leg CLOSED (context_reset),
          run-225672 minted legSeq=1 (BUG-1187 — separate fix)
06:49:49  run-225672's own fresh loop blocked hub_stalled
06:50:54→06:59  6× hub_reinvoke_scheduled → hub_reinvoke_start_failed
          "flow is waiting for your decision (Continue/Stop)"
          — while run-204891's own loop read "running" in the same record
```

Admission order is the trap: `startTurn` on the closed source relays to the
active successor leg FIRST, then evaluates the LEG's loop (`blocked` →
`flow_awaiting_user` 409). `scheduleChildTurn` counted that refusal against
`hubReinvokeStartFailCount`; once it passed 3 the armed
`pendingHubReinvoke` had no drain left — `shouldDrain=false` in the error
path AND the `hub_stall` tick gate requires `count<=3`. The loop cycled
running↔blocked `hub_stalled` with no turn ever starting. Sprint stopped
by operator after the deadlock was proven.

## Root cause — three stacked defects in `scheduleChildTurn`

1. **`flow_awaiting_user` not in the transient set.** A reinvoke racing the
   blocked→running resume transition — or relaying onto a still-parked leg
   — is transient like `hub_parked` (BUG-564), not a real start failure.
   Counting it saturated the consecutive-fail budget.

2. **Relayed success clears the flag on the wrong run.** `startTurn` clears
   `reinvokeInFlight` on the run the turn lands on — after a closed-leg
   relay that is the successor leg, not the scheduling run. The source
   run's flag stayed true forever; every later `maybeAutoReinvokeHub*`
   guard deferred on it.

3. **Child reprompt refused by an open parent decision card was retried
   in-process then dropped** — an in-process retry cannot beat a
   human-scale park, and dropping strands the cohort member (the BUG-1185
   shape).

## Fix — `interactive_service.go` `scheduleChildTurn`

- Root branch: `flow_awaiting_user` joins `hub_parked` as `transientBusy`
  — no budget burn, `pendingHubReinvoke` stays armed, the drain path
  (`notifyTurnIdle` + stall tick) retries after the unblock.
- Success tail: after any `startTurn` returning nil, clear
  `reinvokeInFlight` on `runID` — the scheduler owns the flag; covers the
  relay case where the in-admission clear lands on the leg.
- Child branch: `flow_awaiting_user` parks on the DURABLE gate-reprompt
  intent (`pendingGateReprompt{Prompt,StepID,Gen++}`) +
  `waiting_user_approval` + orchestrator-summary sync (BUG-1179 shape) +
  `persistProviderSession`. `resumeFlowWithFeedback`'s child sweep flushes
  it after unblock, preserving `repromptAttempts` per BUG-539, and the
  intent survives restart.

## Regression

`internal/runner/bug1188_closed_leg_relay_test.go` — three red→green
shapes: root transient keeps armed pending + zero fail count; relayed
success clears the SOURCE flag (real `createRun` leg, real relayed
dispatch); child refusal parks on the durable intent without burning
budget or arming the parent-only path. BUG-289 (retry budget),
BUG-454 (child reprompt retry), BUG-564 (hub_parked transient),
BUG-1176/1177/1179/1180 all still green.
