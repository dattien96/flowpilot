# CA-642 — CP-89 Task-451: Run-Scoped `flowArm` Latch + Pin-vs-Arm Split

**Date**: 2026-09-28
**Author**: Devin
**Ticket**: CP-89 / Task-451

---

## Problem

CP-89 adds a second flow-launch mode: pin a flow at run create without
starting it (`flowArm=pending`), let the user chat normally, and start the
flow only on an explicit `forwardFlow` turn (Task-452). Before this change
the runner had exactly one mode — a pinned `flowRef` armed the flow on the
first turn unconditionally — and no durable latch existed to distinguish
"pinned-not-started" from "started" across restart, kill, Drive restore,
and provider switch.

## Change

- `FlowArm` contract type + `immediate|pending|started` constants in
  `provider_event.go`; `chat_then_forward` is an accepted input alias for
  `pending`.
- `StartRunInput.FlowArm` + `StartRunInput.SourceDocID` (create-pinned CP
  source, needed by the forward-time ingest fence in Task-452).
- `parseFlowArm` validates at create: empty→immediate, unknown→400
  `invalid_flow_arm`, pending without a flow pin (flowRef /
  flowRefFallback / workflowID) →400 `flow_arm_requires_flow`, `started`
  accepted only on internal paths (spawned children, switch legs).
- `interactiveRun.flowArm` — run/chat-scoped, not leg-scoped.
- Pin vs arm split in `createRun`: `chatFlowRef` always stamps; vibe start
  markers (`vibeAwaitingLock`, `vibeSprintBudget`) arm only when
  `flowArm==immediate`. Working-mode fence stays at pin time for both
  modes (unchanged `enforceWorkingModeStart`).
- Durability: `ProviderSessionState.FlowArm` → `sessions.ndjson`;
  `ChatSessionManifest.flowArm` → Drive restore; `switchChatLeg` carries
  `FlowArm` + the pin via `FlowRefFallback` + `WorkingMode` to the new
  leg — a pending latch now survives a provider switch (previously the
  new leg silently lost `chatFlowRef` entirely).
- Reconstruction: `parsePersistedFlowArm` — empty (legacy row)→immediate,
  pending→chat-run restore with markers forced off (latch is
  authoritative), started→no re-arm, unknown→422 `corrupt_flow_arm`
  fail-closed (never a zero-value resume).
- `setFlowArmStartedLocked` helper — pending→started only; Task-452 calls
  it at the forward seam.
- Turn-1 `sourceDocID` resolve now keeps a create-pinned value when the
  turn doesn't resend one (explicit turn value still wins).

## Files

- `internal/runner/provider_event.go` — `FlowArm`, parse helpers,
  `setFlowArmStartedLocked`, `StartRunInput.FlowArm`/`SourceDocID`.
- `internal/runner/interactive_handlers.go` — create-time validation +
  pin-vs-arm split + create-pinned `sourceDocID` stamp.
- `internal/runner/interactive_service.go` — `interactiveRun.flowArm`,
  `sessionStateOf` persistence, turn-1 sourceDocID preserve.
- `internal/runner/workflow_store.go` — `ProviderSessionState.FlowArm`.
- `internal/runner/interactive_resume.go` — restore + fail-closed +
  pending marker normalization.
- `internal/runner/chat_session_sync.go` — Drive manifest `flowArm`
  round-trip.
- `internal/runner/chat_switch.go` — leg carries arm + pin + mode.
- `internal/runner/task451_flowarm_test.go` — 9 additive tests.

## Test evidence

- `TestTask451_*` ×9 green: default-immediate, pending-requires-pin,
  markers-off-when-pending, working-mode fence at pin, session-row
  round-trip, restart-pending→chat, restart-started-no-rearm,
  corrupt-fails-closed, provider-switch-keeps-pending.
- Focused regression: `WorkingMode|TestBug315|TestBug263|TestBug547|
  TestBug548|TestBug506|ChatSwitch` — green (immediate path unchanged).

## Risk / blast radius

- Default absent/empty `flowArm` is byte-identical: all pre-CP-89 inputs
  take the `immediate` branch and hit the same code as before.
- The only behavioral delta for existing inputs: none — the new fields
  are additive `omitempty` on every wire/durable surface.

## Addendum — sessions.ndjson durability gap (found by live drill)

The first cut stamped `FlowArm` on `ProviderSessionState` but never onto
`ndjsonSessionRecord`/`sessionRecordFrom`/`sessionStateFromRecord`, and
`createRun`'s inline session-row literal skipped the field — the latch
survived only in RAM (the `sessionStateOf` projection) and silently
reverted to `immediate` on a real process restart. Caught by
`TestTask451_FlowArmSurvivesSessionsNDJSONRoundTrip` (real
`NewLocalFileSessionStore` file reload, red→green). Fix: `flow_arm` on
the durable record + both mappers + `FlowArm: string(rs.flowArm)` in
`createRun`'s persisted literal. L-9/L-10 now prove it end-to-end.
