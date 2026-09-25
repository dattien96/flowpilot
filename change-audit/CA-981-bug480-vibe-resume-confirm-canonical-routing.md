# CA-981: BUG-480 — generic Continue routed through vibeResumeConfirm gate

## Why

Live run-37268: `vibeResumeConfirm` mounts a resume-confirm gate
(ok/cancel) exposed as `PendingGate`, but `resumeFlowWithFeedback` — the
generic `/agent-loop/continue` path — only special-cased sprint-boundary
and tournament cards. A generic Continue unblocked the loop and
re-invoked the hub while the gate stayed mounted: the operator's intent
was applied to the wrong transition and stale card state survived.

## What changed

- `resumeFlowWithFeedback` now checks `rs.vibeResumeConfirm` after the
  sprint-boundary branch and before the tournament-card branch:
  - feedback "" / "ok" / "continue" → `SubmitGateDecision(runID, "ok")` —
    the canonical consumer (clears the gate, resumes from
    `vibeResumeFromNode`, honors stop-fence/seal semantics).
  - feedback "cancel" → `SubmitGateDecision(runID, "cancel")` — honest
    park, loop stays blocked, gate stays mounted.
  - any other prose → typed `*apiErr` 409 `pending_gate_decision`;
    nothing is unblocked or mutated.
- `handleContinueFlow` propagates a typed `*apiErr` from
  `resumeFlowWithFeedback` verbatim (was: everything wrapped as 422
  `continue_flow_failed`) so clients can route to `gate-decision`.

## Why this shape

The BUG-411 `decisionTarget` path inside `SubmitGateDecision` already
excludes runs carrying their own `vibeResumeConfirm` surface, so routing
generic Continue through `SubmitGateDecision` cannot recurse. `handleAmendFlow`'s
"" feedback now also resumes via the canonical gate when a mounted
confirm coexists with a widened contract — same resume intent, no bypass.

## Tests

- `bug480_vibe_resume_confirm_surface_test.go` (4 tests): unambiguous
  Continue consumes the gate and resumes; "cancel" feedback stays parked;
  prose → 409 pending_gate_decision with gate + block untouched;
  post-consume duplicate Continue does not replay.

## Verification

- `go test ./internal/runner -run 'TestBUG480'` — 4/4 green.
- CA801–CA816 vibe-resume gate suite + BUG-430/411/414 + full suite
  results recorded in the BUG doc.
