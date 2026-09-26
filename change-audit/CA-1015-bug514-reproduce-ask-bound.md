# CA-1015 — BUG-514: reproduce-gated ask_user episodes bounded

## Context

Deep review B-4: r-reproduce reprompts only at turn end; ask_user parks
inside the turn via channel so `repromptAttempts` never incremented and
`maxFlowGateReprompts` was unreachable — a reproducer that could not
reproduce parked on questions forever (live run-90420).

## Changes

- `turnBridge.askQuestion` (`interactive_service.go`): resolves
  `reproduceTurnForRun` before taking `s.mu` (the predicate locks
  internally); a reproduce-gated episode increments
  `rs.repromptAttempts`; once `>= maxFlowGateReprompts` the call is
  refused ("conclude the report"), ending the turn so the gate's
  existing exhausted-reprompt escalation fires. Non-reproduce runs
  untouched.
- `bug514_reproduce_ask_loop_bound_test.go`: reproduce child parks
  `max` questions (counted), next refused; non-reproduce child unbounded.

## Verification

`go test -count=1 -run TestBug514_ ./internal/runner/` — green.
