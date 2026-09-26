# BUG-514 — Reproduce-gated child can park on ask_user forever; reprompt cap unreachable

## Status
FIXED — 2026-09-26 (CA-1015).

## Found during
Deep review B-4 (`CP-Deep-Review-12CP-2026-09-26.md`), verified against
source. Live symptom: run-90420's reproducer parked twice asking the
operator to unblock it instead of producing a RED test or escalating.

## Observed

`checkReproduceRule` reprompts only when a turn *ends* without a red
test. But `ask_user` parks *inside* the turn — the question resolves via
channel, no new `startTurn` runs, so `repromptAttempts` was never
incremented and `maxFlowGateReprompts` (2) could never be reached. A
reproducer that couldn't reproduce the bug had exactly one honest
ending demanded of it (a red test) and no bounded path to conclude
"not reproducible" — so it parked on questions indefinitely.

## Fix (CA-1015)

A reproduce-gated `ask_user` episode now counts against the same
reprompt budget the gate enforces (`rs.repromptAttempts`, already
durable). Once the budget is exhausted, `AskQuestion` is refused with an
error directing the child to conclude the report (reproduced or not
reproducible) — the turn ends, the post-turn gate reprompts once more,
and the `attempts >= max` branch takes the existing `escalate`
flow-control path. The "not reproducible" escape therefore converges on
the escalation card rather than a new schema/state.

- Scope: `reproduceTurnForRun` predicate — reproduce node or
  behavior-change flow only; normal coder/delegate children and chat
  ask_user are untouched.
- No counter reset hazard: questions park inside one turn, and
  `repromptAttempts` survives in the run snapshot across restart.

## Tests

`bug514_reproduce_ask_loop_bound_test.go` — reproduce child parks
`maxFlowGateReprompts` questions (each counted), the next is refused
with a reproduce-budget error; a non-reproduce child asks
`max+1` questions with zero budget impact. Green.

## Round 2 (2026-09-26, CA-1019) — own counter, not the gate's budget

The first fix incremented `repromptAttempts` per parked question — the
same counter the flow gate uses for turn-end reprompts. Two answered
questions could exhaust the gate's budget before the provider ever
attempted the required repair, starving the escalation path.

Round 2: the reproduce bound now counts `EventUserQuestionRequired` on
the run's OWN durable event stream — no shared counter, restart-safe by
construction (the events are persisted and rehydrated). The gate's
`repromptAttempts` keeps its full reprompt budget, so the turn-end
reprompt→escalate ladder (BUG-391) is untouched.

## Tests (round 2)

- `TestBug514_ReproduceChildAskUserBounded` — updated assertion: after
  the parked episodes `repromptAttempts == 0` (gate budget intact) while
  the durable stream holds the question events; the next ask is refused.
- `TestBug514_ReproduceAskBoundSurvivesRehydratedEvents` — a run whose
  rehydrated stream already carries the episodes is refused immediately.
