# BUG-480: generic Continue does not consume vibe resume-confirm gate

## Metadata

- Document ID: `BUG-480`
- Phase: `bugfix`
- Status: `done`
- Severity: `medium-high`
- Evidence: `live-observed run-37268; code-confirmed`
- Feature Keys: `vibe-mode`, `agent-flow-engine`
- Parent Documents: `CP-60`, `CP-58`
- Related Documents: `BUG-411`, `BUG-471`
- Affected Areas: `interactive_handlers.go`, `interactive_service.go`

## Summary

A Vibe `vibeResumeConfirm` card is exposed as a pending gate and is correctly
consumed by `/gate-decision`. The generic `/agent-loop/continue` endpoint does
not route this gate: `resumeFlowWithFeedback` has a dedicated sprint-boundary
branch but no equivalent `vibeResumeConfirm` branch. A Continue can unblock or
reinvoke the hub without consuming the mounted gate.

## Evidence

- Live run-37268: `agent-loop/continue` answered with a fresh hub turn instead
  of consuming the resume card; `/gate-decision` was required.
- `handleContinueFlow` always calls `resumeFlowWithFeedback` after member/card
  handling.
- `resumeFlowWithFeedback` explicitly special-cases
  `vibeSprintBoundaryReason` and tournament cards, but not
  `rs.vibeResumeConfirm`/`vibeResumeFromNode`.
- Snapshot exposes the state as `PendingGate`, so surfaces can plausibly send
  either generic Continue or gate decision unless the server rejects/reroutes.

## Expected vs Actual

- Expected: one canonical action contract consumes the mounted decision, or the
  wrong endpoint returns a typed conflict without changing the run.
- Actual: Continue is accepted on a gate it does not own and may drive unrelated
  hub work.

## Impact

Operator intent can be accepted but applied to the wrong transition, causing
extra provider turns, stale cards, or a running/blocked mismatch.

## Required Fix Contract

1. Route mounted resume-confirm through its gate consumer, or reject generic
   Continue fail-closed.
2. Never unblock the loop before the gate decision commits.
3. Preserve BUG-471 requirement-resume and sprint-boundary semantics.
4. Desktop/TUI must use the same decision endpoint contract.

## Required Tests

- RED: generic Continue while resume-confirm is mounted cannot spawn a hub turn.
- OK consumes once and resumes the recorded node; Cancel remains parked/stops
  according to contract.
- Duplicate/stale decisions are idempotent or typed 409.
- Live replay of the run-37268 shape.

## Implementation Plan

### P-1 — Reproduce endpoint mismatch

- Add `bug480_vibe_resume_confirm_surface_test.go` that mounts
  `vibeResumeConfirm`, parks the loop and calls the real Continue handler.
- Assert current behavior clears/reinvokes without consuming the gate, while
  `SubmitGateDecision` consumes it correctly.
- Record turn/spawn counts so accepted-but-wrong routing is explicit.

### P-2 — Establish one server authority

- Add a preflight decision-surface check before generic loop mutation.
- Preferred behavior: route an unambiguous Continue/OK into the existing Vibe
  gate consumer; otherwise return typed `pending_gate_decision` with the
  canonical endpoint/options.
- Never clear block state, gate fields or cancellation flags before the gate
  transition commits.
- Keep sprint-boundary and BUG-471 requirement-park branches in their existing
  precedence order.

### P-3 — Client convergence

- Desktop and TUI should submit `gate-decision` whenever snapshot has
  `PendingGate`; generic Continue remains for cap/escalate/free-form cards.
- Stale/double submissions return the already-resolved result or typed 409;
  clients refresh rather than issuing a hub turn.

### P-4 — Regression/live matrix

- Cover OK, Cancel, custom/invalid input, duplicate click, restart with mounted
  gate and generic Continue attempted from both clients.
- Replay run-37268 shape and verify zero extra hub turns.

## Definition of Done

- [ ] RED endpoint test captures the extra/wrong hub dispatch.
- [ ] Mounted resume-confirm is consumed only by its canonical transition.
- [ ] Wrong endpoint cannot unblock or spawn before decision commit.
- [ ] Desktop and TUI use matching decision-surface rules.
- [ ] Duplicate/stale decisions are idempotent or typed conflicts.
- [ ] BUG-471 requirement resume and sprint-boundary tests remain green.
- [ ] Live run-37268 equivalent produces zero unrelated hub turn.
- [ ] Decision routing and endpoint contract are documented in CA evidence.
