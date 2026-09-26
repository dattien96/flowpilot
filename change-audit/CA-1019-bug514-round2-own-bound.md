# CA-1019 — BUG-514 round 2: durable event bound, gate budget untouched

## Context

Round-2 review: counting parked reproduce questions into
`repromptAttempts` shared the GATE's reprompt budget — two answered
questions could exhaust it before the provider attempted the required
repair, so the escalate path could never fire.

## Changes

- `interactive_service.go` `turnBridge.askQuestion`: the reproduce bound
  now counts `EventUserQuestionRequired` on the run's durable event
  stream — persisted and rehydrated, so it survives restart and never
  touches the gate's `repromptAttempts` budget. Over budget → refuse the
  question so the turn ends and the gate's reprompt→escalate ladder
  (BUG-391) fires on its own counter.

## Tests

Updated `TestBug514_ReproduceChildAskUserBounded` (my round-1 file —
assertion now locks the corrected semantics: `repromptAttempts == 0`
after episodes) + new
`TestBug514_ReproduceAskBoundSurvivesRehydratedEvents`.
