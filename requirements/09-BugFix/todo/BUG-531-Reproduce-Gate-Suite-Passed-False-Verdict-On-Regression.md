# BUG-531 — reproduce gate reports "the suite passed" when a baseline-green test regresses alongside the reproduction

**Status:** fixed on branch (assertion test + gate_hook fix).
**Found live:** run `run-134` / parent `run-1` on `/tmp/fp-live2` (devin/swe-2-high, bug-harness, 2026-09-27).

## Reproduction

Live sequence that produced it:

1. bug-harness on a green repo (`Base()==1`, `TestBase` green). Baseline
   recorded `suite_passed=true`, GreenTests=[TestBase].
2. Reproducer correctly detected green-on-arrival and asked
   `user_question_required` (q-750).
3. Operator re-seeded the bug (`base.go` -> `return 0`, uncommitted) and
   answered "re-run reproduction".
4. Reproducer's repro test + the pre-existing `TestBase` both FAILED.
   The gate's own scoped oracle observed `go test -v .` exit 1.
5. Yet the reprompt fired with "the suite passed, so the bug was not
   reproduced" — twice — then `max reprompts exceeded` escalated and parked
   the flow.

## Root cause

`internal/runner/gate_hook.go` populates `tr.Tests.Failed` only under
`!oracle.SuitePassed && !oracle.HasRegression` (the "baseline-red world"
coarse branch). When the baseline is green and the suite now fails,
`RunOracleContext` classifies the failure as a regression
(`HasRegression=true`): baseline-green test names land in
`oracle.Regressed`, and `failedTests` stays empty.

`checkReproduceRule` then reads `len(tr.Tests.Failed)==0` as "suite passed"
and reprompts — even though `oracle.Failed` contains every named failure of
this run, including the new reproduction test.

Impact: any real bug that also breaks a baseline-green test (the common
shape — the bug *is* a regression) can never satisfy r-reproduce. The turn
burns its bounded reprompts on a false verdict and the flow escalates/parks.

## Fix

In the reproduce-turn branch of the gate hook, repopulate
`tr.Tests.Failed` from the oracle's full named-failure list
(`oracle.Failed`, override-filtered). r-tests/r-reg are suppressed for a
reproduce turn anyway, so widening `Failed` there cannot double-fire; all
other turns keep the existing mapping byte-identical. Applied to both gate
entry paths (child gate and inline path).

## Evidence

- Serve log: `[gate] suite end cmd="go test -v ." ... err=exit status 1`
  followed by reprompt `attempt=2` whose prompt claims "the suite passed";
  `flow_control_escalate` "max reprompts exceeded on coding step".
- Red test: `bug531_reproduce_gate_regression_masked_test.go`.
