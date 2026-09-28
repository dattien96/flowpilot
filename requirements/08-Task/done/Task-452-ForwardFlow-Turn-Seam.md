# Task-452: `forwardFlow` Turn — Admission, Pin Resolution, Fences

- Document ID: `Task-452`
- Title: `Explicit forward turn: admission field, pinned-ref resolution seam, first-turn fences at forward time, typed errors`
- Phase: `task`
- Status: `done`
- Created: `2026-09-28`
- Parent Documents: `CP-89`, `Task-451`
- Related Documents: `Task-453` (entry prompt), `CP-89-Test-Steps`, BUG-261/BUG-315 (gates that must not soften)
- Tags: `flow`, `admission`, `gate`, `fail-closed`

## AI Quick View

### Summary

The forward turn carries `forwardFlow: true`. Today it would die twice:
at the BUG-509 empty-body content check (F-1) and inside the flow-start
gates that read `in.FlowRef` while the pin lives on `rs.chatFlowRef` (F-2).
This task wires the seam: admission counts `forwardFlow` as content, the
forward resolves the pinned ref through a second seam (never softening the
`turnCount==0` guard), the first-turn fence set runs at forward time, and
`pending→started` latches exactly once.

### Current Ask

A `forwardFlow` turn on a `pending` run performs exactly what turn-1 flow
start does today — same fences, same `startResolvedFlow`, same
`flowStartOnly` hub suppression — keyed off the pin instead of the turn
field.

### Key Decisions

- `forwardFlow` counts as actionable content in `handleStartTurn` — bare
  `{"forwardFlow": true}` is legal (F-1).
- Pin resolution at forward: `rs.chatFlowRef` → `workflowID`-mounted pin →
  typed error. `resolveWorkflowFlowRef`'s `turnCount != 0` bail stays
  untouched; a dedicated `resolvePinnedFlowRefForForward` seam exists for
  the forward path (F-2).
- The `invalid_cp_source` fence runs at forward time on the resolved pin,
  honoring the create-pinned `SourceDocID` (F-4 decision in note §8).
- `forwardFlow` on a run with no pin → 422 `forward_requires_flow_pin`;
  corrupt definition between pin and forward → 422 `invalid_flow_definition`
  — never degrade to chat (F-5).
- `forwardFlow` on `immediate` or already-`started` → 422
  `flow_already_started` (no-op would silently swallow user intent).
- One-shot latch: `pending→started` inside the same durable mutation that
  stamps the flow-start — a forward turn that fails a fence does NOT
  consume the latch.

### Constraints

- No model-inference path to forward — only the flag. Strings in prompts
  are content, never consent.
- `flowStartOnly` suppression reuses the existing synthetic
  `turn_completed` mechanism — it is already turnCount-agnostic.
- All BUG-261/BUG-315/`restoredFrom` guards preserved verbatim.

## 1. Goal

An explicit flag turns a `pending` run into a live flow run atomically,
with the same fences turn-1 carries today.

## 2. Parent Links

- coding plan: `CP-89` §3.2, §8 (F-1, F-2, F-4, F-5)

## 3. Trigger

`pending` needs a start door that isn't `turnCount == 0` — chat turns close
that door permanently today (§4 of the note).

## 4. Exact Change

- `T-1` `StartTurnInput.ForwardFlow bool`; admitted into the content check
  and `handleStartTurn`.
- `T-2` In `startTurn`, before the `turnCount == 0` flow-start block:
  `if in.ForwardFlow { return s.forwardPinnedFlow(...) }` —
  resolves the pin, runs fences, latches `started`, calls
  `startResolvedFlow` with the forward prompt + transcript package (shape
  handed to Task-453's packer).
- `T-3` `resolvePinnedFlowRefForForward(rs) (flowRef, err)` — chatFlowRef →
  workflowID pin → typed 422; corrupt → `invalid_flow_definition`.
- `T-4` Move first-turn fence set behind a callable `runFirstTurnFences`
  used by both turn-1 start and the forward turn: working-mode (already
  fenced at pin — revalidate), `invalid_cp_source` (now scoped to
  pin+SourceDocID), flow-definition validation.
- `T-5` A failed fence on the forward turn leaves `flowArm=pending` —
  the card/error surfaces and the user may fix + forward again.

## 5. Touched Areas

- files: `interactive_handlers.go` (admission), `interactive_service.go`
  (`startTurn` forward branch), `vibe_cp.go` (source validation signature),
  `flow_executor.go` (new resolve seam — `resolveWorkflowFlowRef` untouched)
- modules: `runner`
- routes: `POST /runs/{id}/turns` body field
- tables: none new

## 6. Code Guide Signatures

```go
// StartTurnInput
ForwardFlow bool // explicit forward — content-check + startTurn seam

func (s *InteractiveService) forwardPinnedFlow(ctx context.Context, rs *interactiveRun, in StartTurnInput) *apiErr
func resolvePinnedFlowRefForForward(rs *interactiveRun) (string, *apiErr)
func (s *InteractiveService) runFirstTurnFences(ctx context.Context, rs *interactiveRun, flowRef string, in StartTurnInput) *apiErr
// errors
forwardRequiresFlowPinErr  = 422 "forward_requires_flow_pin"
flowAlreadyStartedErr      = 422 "flow_already_started"
// reuses pendingFlowRefInvalidErr shape for invalid_flow_definition
```

## 7. Test Signatures

- `TestTask452_BareForwardPassesAdmission` — `{"forwardFlow": true}` alone
  reaches `startTurn` (not the BUG-509 400).
- `TestTask452_ForwardStartsFlowFromPin` — entry child spawns; hub
  suppressed; `flowStartOnly` synthetic turn_completed emitted.
- `TestTask452_ForwardWithoutPinIs422` — `forward_requires_flow_pin`.
- `TestTask452_ForwardOnStartedIs422` — `flow_already_started`, no second
  entry spawn.
- `TestTask452_ForwardOnImmediateRunIs422` — same typed error.
- `TestTask452_FailedFenceKeepsPending` — `invalid_cp_source` on forward →
  `flowArm` stays `pending`; fix + re-forward works.
- `TestTask452_CorruptDefinitionFailsClosed` — `invalid_flow_definition`,
  never chat.
- `TestTask452_WorkflowIDMountedPinResolves` — second seam covers
  workflowID pins without touching the `turnCount==0` guard.
- `TestTask452_TurnCountGuardUnchanged` — non-forward turn 3+ with
  `flowRef` still ignored (BUG-261 protection intact).
- `TestTask452_PinnedSourceDocIDSatisfiesCpIngest` — create-pinned
  `SourceDocID` passes `vibe-cp-ingest` forward validation (F-4).

## 8. Acceptance Check

Chat 3 turns on `pending`, bare-Forward → flow starts with full fences;
double-Forward → typed error; no-pin Forward → typed error; nothing in
`resolveWorkflowFlowRef` changed.

## 9. Out of Scope

- Transcript packing into the entry prompt (Task-453 consumes a
  `forwardPromptPackage` seam — Task-452 may pass the raw forward text).
- UI affordance.

## 10. Definition of Done

- [ ] §6 signatures landed or deviation documented
- [ ] §7 tests additive + green; full `./internal/runner/...` suite green
- [ ] BUG-261/BUG-315/run-63960 gate tests untouched and green
- [ ] Fences never degrade to silent chat on forward failures
- [ ] CA entry + commit `[Feature][forward-flow] ...`
