# CA-1047 — BUG-550: Retry on a gate-escalated plain chat run dead-loops "running"

## What changed

`apps/local-runner/internal/runner/interactive_service.go`:

- `resumeFlowWithFeedback` — new branch placed *after* the BUG-520
  orphan/intent sweep (so parked children with armed `pendingResume*`
  intents still flush first — BUG-538 contract preserved): when
  `blockReason == "escalate"` and the sweep dispatched nothing
  (`len(resumeIDs) == 0`, no orphans redrived), the run re-drives a real
  turn through `resumeParkedPlainChat`. The parked `GateReason` (the
  outstanding violation, `Gate reprompt exhausted: ` prefix stripped) is
  composed into the resume prompt — verbatim under the operator's
  feedback, or as the whole prompt on a bare Retry — so the re-driven
  turn sees what the gate wants instead of re-violating blind.
- `resumeDriftParkedChat` → `resumeParkedPlainChat(parentRunID, feedback,
  blockReason)` — same helper, generalized: the re-park on dispatch
  failure re-stamps the park's own blockReason instead of always
  labelling drift, and the plain-chat predicate now also refuses
  terminal roots (`failed`/`cancelled`/`completed`) — a Failed root is a
  real outcome, not park poison (run-203966 contract). Park-poisoned
  `cancelled` runs are unaffected: the existing heal restores them to
  `running` before this point.

`apps/local-runner/internal/runner/bug550_chat_escalate_continue_test.go` —
new reproduce-first coverage (all four were red before the fix).

## Root cause (live run-2012663)

Post-turn flow gate reprompts exhausted → `applyFlowControl(escalate)` →
`parkFlowForAwaitingUser` parked the plain chat run `blocked/escalate` and
dropped the armed reprompt intent. The operator's Retry unblocked the loop
(`agent_graph_updated` seq 1498, status=running) but every re-drive path
declined: the drift branch only matched `DriftPauseBlockReason`, the
generic `maybeAutoReinvokeHubWithNote` tail no-ops on
`autoOrchestrate=false`, and `maybeScheduleHubStallCheck` refuses non-flow
runs (`!rs.flowEngineDriven`). Result: loop "running" with no turn in
flight, no watchdog, no card — a permanent silent composer soft-lock.
Same dead-end class as BUG-430 (drift_pause), one park reason over.

## Invariant

A user-facing blocked card on a plain chat run must always have a live
resolution path: Continue/Retry either dispatches a real turn or re-parks
with the dispatch-failure reason — never a dead "running" loop. Terminal
runs are never resurrected, and children with armed resume intents keep
priority (sweep runs first).

## Tests

- `TestBug550_GateEscalatedChatContinueDispatchesTurn` — red before fix:
  Retry left `turnInFlight=false`, `loop=running`.
- `TestBug550_EscalateResumePromptCarriesGateViolation` — the re-driven
  prompt carries the outstanding gate violation.
- `TestBug550_EscalateContinueReparksOnDispatchFailure` — failed dispatch
  re-parks `blocked` preserving `blockReason=escalate` + failure reason.
- `TestBug550_ContinueRouteDispatchesEscalatedChatTurn` — e2e through
  `POST /agent-loop/continue`.

Regression: BUG-430 drift tests, BUG-538 parked-successor flush
(sweep-first ordering), and run-203966 heal/stop/fail matrix all green.
