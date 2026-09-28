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

## Addendum 2 — full-diff review findings (post-commit e13c5509)

A production-diff review pass found and fixed four more real issues:

1. **Supabase runtime blob dropped `flow_arm`.** `dbProviderSessionRow` /
   `sessionRuntimeBlob` carried `chat_flow_ref` but no `flow_arm` — on the
   Supabase backend the latch survived only in RAM and a restart silently
   reverted a pending run to `immediate` (auto-start on next turn), violating
   the durable-state contract. Fix: `flow_arm` jsonb field on
   `sessionRuntimeBlob` + both `sessionRuntimeFromState` /
   `applySessionRuntimeBlob` mappings. Red test:
   `TestTask451_FlowArmSurvivesSupabaseRuntimeBlob`.

2. **`started` was forgeable through `switchFromRunId`.**
   `parseFlowArm`'s `internal` gate accepted `SwitchFromRunID != ""` — but
   that field is a client-writable JSON input (chat reattach), so any client
   could mint an internal `started` latch. Tightened to
   `in.SpawnedInternally` only (`json:"-"`, unforgeable over HTTP). Server
   leg-mints (switchChatLeg, spawnChildRun) all set SpawnedInternally.

3. **Pending reconstruct normalized too little.** The first cut cleared two
   vibe fields; a corrupt pending row could still leak `vibeLockedCP`,
   checkpoint/park/resume markers, `activeFlowNodes`, `flowEngineDriven`, and
   `pendingFlowGateSettle` into the chat run. Fix: the pending branch now
   clears the complete persisted derived set — a pending run never ran, so
   any arm/park/gate residue is corrupt by definition. AND two ordering
   fixes the initial edit missed:

   - `applyVibeCheckpointFromDisk` runs after the clear and `demote`
     rescans workspace files on an empty marker — it would re-arm a
     checkpoint on a not-yet-started run. Now skipped for pending.
   - The vibe reopen sweep (boundary repark / missing-artifact restarts /
     resume-confirm parks) re-derives parks from workspace files — same
     class of leak. Now gated `flowArm != FlowArmPending`.

   Red test: `TestTask451_PendingReconstructClearsStaleVibeMarkers`
   (corrupt pending row + armed markers → reconstruct clean).

4. **Pin resolution order diverged from baseline.** The forward resolver
   preferred `chatFlowRef` over `workflowID`; the baseline mount path
   (`resolveWorkflowFlowRef`) prefers `workflowID` first, `chatFlowRef`
   second (root-only, since a child's fallback pin is not a mount). Aligned
   — a forward now launches the same pin the first-turn path would.

### Residuals — FIXED in review pass 2

- **Prepared-relaunch forward loss.** FIXED: the forward seam now runs on
  durable relaunch — the latch is the idempotency key. Relaunch on
  arm=started completes synthetically (no respawn); relaunch on pending
  runs the full fence+flip+launch. Tests:
  `TestTask452_PreparedRelaunchForwardOnPendingStillLaunches`,
  `TestTask452_PreparedRelaunchForwardOnStartedCompletesNoRespawn`.
- **Flip-vs-persist crash window.** FIXED: the forward path stamps flow
  topology and persists arm=started BEFORE spawning the goroutine
  (durable-first). A crash can only leave pending (pre-persist, no
  children possible) or started+topology (resume machinery owns it).
  Backward-compat heal on reconstruct: `started` with no persisted
  topology normalizes to pending so a retry forward launches cleanly
  (`TestTask451_StartedWithoutTopologyHealsToPending`,
  `TestTask452_ForwardPersistsStartedBeforeSpawn`).

### Review pass 3 — deeper concurrency + seam findings (all FIXED)

1. **Forward-time launch metadata was not adopted onto the run.** The
   immediate first-turn block adopts `ChangeType`/`SourceDocID`/`SubMode`
   into `rs` before the fences; the forward branch validated the turn's
   `SourceDocID` but never stored it — `vibeLockedCP` would stamp the stale
   create-time pin (or empty) instead of the validated value. The forward
   is this run's launch turn; adoption now mirrors the immediate path.
   Test: `TestTask452_ForwardTurnSourceDocIDAdopted` (turn-level source
   lands on `rs.sourceDocID` and `vibeLockedCP`).

2. **Lock-ordering hardening on the durable-first persist.**
   `persistProviderSession`/`flowDiagLog` take `o.mu` (loopStateFor) and
   are documented safe under `s.mu` — but baseline `o.mu→s.mu` inversion
   sites exist (mutateLoop closures), so holding `s.mu` across them widens
   an ABBA exposure, and holding `s.mu` across disk I/O is unnecessary.
   The forward path now drops `s.mu` for diag+persist, then re-locks and
   verifies the run is still present and non-terminal before spawning.
   The interleavings are handled explicitly:
   - deleted mid-gap → re-delete the resurrected durable row (delete wins
     as last writer), no spawn;
   - stopped mid-gap → re-persist the terminal snapshot (stop's status
     wins as last writer), no spawn.
   Adjacent baseline inversion fixed in the vibe reopen sweep
   (interactive_resume: the mutateLoop closure no longer takes `s.mu`
   inside — the map read is hoisted out). Other `o.mu→s.mu` closures
   (cohort_stall, vibe_cp, vibe_debate, vibe_gate, vibe_sprint) are
   pre-existing baseline sites — same latent hazard class, unchanged
   by this fix, tracked as follow-up.

3. **Heal guard refined for switch legs.** `started` + no topology +
   `SwitchFromRunID != ""` is an inherited-latch leg whose flow runs on
   the ORIGIN leg — healing it to pending would let a retry forward
   launch the same flow twice. Heal now applies only to first-leg rows
   (`SwitchFromRunID` empty).

4. **Corrupt pending-child row could forward a nested flow.** Clients
   cannot mint a pending child (`StartRunInput` has no ParentRunID) and
   spawnChildRun never stamps pending, but a bad durable row reaching
   reconstruct could. `forwardPinnedFlow` now fails closed with
   `forward_requires_root_run` before pin resolution.
   Test: `TestTask452_ForwardOnPendingChildRunIs422` (422 + latch stays
   pending).
