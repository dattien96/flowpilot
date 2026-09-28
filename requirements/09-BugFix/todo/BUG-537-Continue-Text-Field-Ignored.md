# BUG-537 — `agent-loop/continue` silently ignores `{"text": "..."}` feedback

Status: FIXED (unit-verified red→green; pending live re-run)
Filed: 2026-09-27 (R6 residual — option-card contract review)
CA: CA-1044

## Symptom

`POST /client/workflow-runs/{runId}/agent-loop/continue` decoded only the
`feedback` field. Callers that post `{"text": "..."}` (the option-card
capture contract) had their feedback silently dropped: the loop resumed
with an empty string, which on the drift-parked chat path dispatched a
bare `"continue"` prompt and on gated paths re-escalated with
`(no progress since last continue)`.

## Root cause

`handleContinueFlow` (interactive_handlers.go) declared only `Feedback`
in the body struct; `text` fell through the decoder untouched.

## Fix (CA-1044)

The body struct gains `Text string`. `text` aliases `feedback`: the
resolved feedback is `feedback` when non-empty, else `text`. Both
consumers — `captureDecisionChoice` (option id/label matching) and
`resumeFlowWithFeedback` — receive the resolved value, so an explicit
`feedback` keeps precedence and an empty `text` never clobbers a real
`feedback`.

## Tests

`internal/runner/bug430_drift_continue_test.go` —
`TestContinueFlowTextFieldAliasesFeedback`: POST `{"text":"marker"}`
against a drift-parked chat run must dispatch a resume turn whose prompt
carries the marker (pre-fix the prompt fell back to the bare `"continue"`
placeholder — verified RED).
