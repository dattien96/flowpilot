# BUG-259: Failed Turn With Masked Nil Error Bulk-Completes Workflow Steps

## Metadata

- Document ID: `BUG-259`
- Title: `Failed Turn With Masked Nil Error Bulk-Completes Workflow Steps`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-08`
- Last Updated: `2026-07-08`
- Parent Documents: none directly
- Child Documents: `none`
- Related Documents: [BUG-174 legacy-planner bulk-DONE regression (referenced in code comments, no standalone doc found)], [BUG-254: Failed Cohort Reviewer Mark Done And Hidden Node Identity](./BUG-254-Failed-Cohort-Reviewer-Mark-Done-And-Hidden-Node-Identity.md)
- Replaces: `none`
- Tags: `workflow-authoring, codex-adapter, usage-limit, bulk-progress, regression`

## AI Quick View

### Summary

- Live repro (`run-9096`): a workflow-attached chat run (workflow `flow-gpt-2-review-1failed` selected, YOLO on) sent one message. The Codex account had already hit its usage limit; the turn genuinely failed (`rs.status` correctly settled to `failed`, the chat shows the usage-limit error). But the Flow Step Timeline sidebar showed all 4 workflow steps (`coder-gpt`, `review-gpt`, `review-security-gpt`, synthesis) as green "done" — confirmed via a direct `GET /client/workflow-runs/{runId}/steps-runtime` call, not just a UI screenshot: the backend itself returned `status:"DONE"` for every step, timestamped to the exact turn start/end.
- No flow-engine diagnostic log exists for this run at all (`flow_start_begin` never fired) and the orchestrator's loop state stayed at its zero value throughout — proving the flow engine itself never ran. The bulk "done" came from the *legacy* `WorkflowOrchestrator.Progress`/`PlanWorkflowProgress` planner instead.
- Root cause: `codex_adapter.go`'s `SendTurn` returns `nil` for **both** `EventTurnCompleted` and `EventTurnFailed` terminal events. `runTurn`'s legacy-planner gate (`if err == nil && !rs.flowEngineDriven`) trusted that `nil` at face value, so a genuinely failed turn on a non-flow-engine-driven, workflow-attached run still triggered `Progress()`, which bulk-marked every still-`PENDING` step `DONE` (YOLO mode, no approval required) in one pass — the same defect class BUG-174 already fixed for the *direct* case, resurfacing through the adapter's masked-error side door.

### Current Ask

- A turn that actually failed (regardless of what the adapter's `SendTurn` returns) must never trigger the bulk workflow-step "mark everything done" fallback.

### Key Decisions

- `V-1` Fix at the `runTurn` call site, not the Codex adapter's error contract: check the run's own settled `status` (already correctly set to `Failed` by `emitLocked`'s `EventTurnFailed` handling, which runs synchronously via `bridge.Emit` before `SendTurn` returns) in addition to `err == nil`, before invoking the legacy `Progress()` fallback.
- `V-2` (follow-up, found via a second live repro after `V-1` shipped): skipping `Progress()` on a failed turn is correct, but nothing else settles the step that `markStepRunning` had already flipped to `RUNNING` at turn start — it hung at `RUNNING` forever instead of falsely showing `DONE`, which is a smaller but still-real lie. Added `markStepFailed` (mirrors `markStepRunning`) and call it from `runTurn` when the turn failed and the run isn't flow-engine-driven, settling `in.StepID` to `FAILED`.
- Rejected: changing `codex_adapter.go`'s `SendTurn` to return a non-nil error for `EventTurnFailed`. That is the more "textbook correct" fix, but `finishTurn`'s error-handling `switch` has no de-dup guard for a `default:`-branch error arriving after `emitLocked` already processed an `EventTurnFailed` for the same turn — doing so would call `emitLocked` a second time with a redundant/duplicate failure event, risking a doubled error message in the UI and double-signaling any listening children. Left as a known, separately-fixable latent issue; not exercised by this fix.

### Constraints

- Must not affect the already-correct BUG-174 behavior: a flow-engine-driven run (`rs.flowEngineDriven == true`) already skips this legacy planner unconditionally, before and after this fix.
- Must not affect a genuinely successful turn on a plain workflow-attached run: `Progress()` must still bulk-complete steps in that case (see the existing `control_bulk_completes` regression test).

### Open Questions

- None for this fix. Flagged as a follow-up, not fixed here: `codex_adapter.go:261-262`'s `SendTurn` should arguably return a non-nil error on `EventTurnFailed` for correctness of any *other* `err == nil`-gated logic that might be added later; that would additionally require hardening `finishTurn`'s error switch against a duplicate `EventTurnFailed` emission for the same turn.

### Source Refs

- Live repro: `run-9096` — `GET /client/workflow-runs/run-9096/steps-runtime` returned all 4 steps `DONE`; `.flowpilot/chats/sessions.ndjson` records for `run-9096` show `status: failed`, `last_message` containing the Codex usage-limit text, and `loop_state` staying at its zero value (`{"status":"","round":0,"roundCap":3}`) throughout — proving the flow engine never engaged.
- [interactive_service.go](../../../apps/local-runner/internal/runner/interactive_service.go) — `runTurn`'s legacy-planner gate.
- [codex_adapter.go](../../../apps/local-runner/internal/runner/codex_adapter.go) — `SendTurn`'s masked-nil-error terminal handling.
- [workflow_state_machine.go](../../../apps/local-runner/internal/runner/workflow_state_machine.go) — `PlanWorkflowProgress`, the bulk-complete walker.
- [flow_step_runtime_test.go](../../../apps/local-runner/internal/runner/flow_step_runtime_test.go) — `TestFlowEngineDrivenRunSkipsBulkProgress`.

## 1. Issue Summary

A workflow-attached chat run whose own turn failed (Codex usage-limit error) had every one of its still-pending `workflow_steps` bulk-marked `DONE`, because the failure was masked as `err == nil` by the Codex adapter and the legacy bulk-progress gate trusted that value unconditionally.

## 2. Parent Links

- impacted coding plan: none directly (this is a `run_kind: "workflow"` chat path, not the CP-36 flow-engine's own step machinery)
- impacted task: none directly
- impacted tech design: none directly
- impacted system spec: none directly

## 3. Environment and Reproduction

- environment: desktop-flowpilot + local-runner, Codex provider account already at its usage limit
- reproduction steps:
  1. In the desktop's Workflow mode, select a workflow (e.g. a custom review-loop clone).
  2. Send a message with YOLO mode on, using a Codex account that has already exhausted its usage limit.
  3. The turn fails immediately with "You've hit your usage limit."
  4. Observe the Flow Step Timeline sidebar: all steps show green "done" checkmarks despite the run failing before any step could genuinely run.

## 4. Expected vs Actual

- expected: the run shows `Failed`, and its workflow steps remain `PENDING` (or reflect only what genuinely ran) — not bulk-`DONE`.
- actual: the run correctly shows `Failed`, but every workflow step is bulk-marked `DONE`.

## 5. Impact

- users affected: anyone whose provider turn fails on a workflow-attached (non-flow-engine) run — usage-limit errors, rate limits, or any other terminal provider error that reaches `EventTurnFailed`
- workflows affected: any `run_kind: "workflow"` chat run that isn't itself a flow-engine-driven run (i.e. a workflow selected for step-tracking without going through `startResolvedFlow`)
- severity: medium — no data corruption of the underlying work, but the step timeline actively lies about what happened, which is actively misleading during failure diagnosis

## 6. Root Cause

- confirmed cause 1: `codex_adapter.go:261-262` — `SendTurn`'s event pump returns `nil` for both `EventTurnCompleted` and `EventTurnFailed`:
  ```go
  bridge.Emit(ev)
  if ev.Type == EventTurnCompleted || ev.Type == EventTurnFailed {
      return nil
  }
  ```
- confirmed cause 2: `interactive_service.go`'s `runTurn` trusted that return value as the sole success signal for the legacy bulk-progress fallback:
  ```go
  err := s.sendTurnWithRetry(ctx, adapter, req, bridge)
  if err == nil && !rs.flowEngineDriven {
      _, _ = s.orchestrator.Progress(ctx, rs.id, yolo)
  }
  ```
  Since `bridge.Emit(ev)` synchronously ran `emitLocked`, which already set `rs.status = RunStatusFailed` for the `EventTurnFailed` case, the run's own state correctly reflected the failure — but the gate above never consulted it, only the adapter's (unreliable) `err`.
- mechanism: `PlanWorkflowProgress` (`workflow_state_machine.go:150`) walks every non-terminal step; with YOLO mode on and no approval required, it completes each one unconditionally in a single pass — exactly what BUG-174 already identified and fixed for flow-engine-driven runs, but that fix's guard (`!rs.flowEngineDriven`) does not protect a plain workflow-attached run whose turn merely *failed*.

## 7. Fix Strategy

- `F-1` In `runTurn` (`interactive_service.go`), after `sendTurnWithRetry` returns, take a locked snapshot of `rs.status` and gate the legacy `Progress()` call on `err == nil && rs.status != RunStatusFailed && !rs.flowEngineDriven` instead of `err == nil && !rs.flowEngineDriven` alone.
- `F-2` (follow-up) Add `markStepFailed(ctx, runID, stepID)`, mirroring `markStepRunning`, and call it from `runTurn`'s `else if turnFailed && !rs.flowEngineDriven` branch so the in-flight step (`in.StepID`) settles to `FAILED` instead of hanging at `RUNNING` once `Progress()` is correctly skipped.

## 8. Validation

- `rtk go test ./internal/runner -run 'TestFlowEngineDrivenRunSkipsBulkProgress' -v -count=1` — 4 subtests passed, including `failed_turn_with_nil_err_skips_despite_not_flow_engine_driven`, which now asserts the in-flight step settles to `FAILED` (not just "not DONE"); confirmed to fail against the pre-`F-1`/pre-`F-2` code via a temporary revert (`s1 = "DONE"`, `s2 = "DONE"`).
- Second live confirmation after `F-1` shipped: `run-9102` (pre-fix binary, unchanged repro) still bulk-showed all 4 steps `done`, confirming the running server needed a rebuild+restart. After rebuild+restart, `run-9325` correctly showed `Failed` with `review-gpt`/`review-security-gpt`/synthesis at `pending` — but `coder-gpt` was stuck at `running` (not yet settled), which is exactly what `F-2` fixes.
- `rtk go test ./internal/runner/... -count=1` — same 15 pre-existing unrelated failures on both baseline and fixed code (Codex CLI unavailable in this environment, Windows home-dir/path fixtures, skill-merge fixtures); no new failures introduced.
- `rtk npx gitnexus impact --repo flowpilot runTurn` — `LOW`.

## 9. Regression Guard

- tests:
  - `TestFlowEngineDrivenRunSkipsBulkProgress/failed_turn_with_nil_err_skips_despite_not_flow_engine_driven` (new) — a fake Codex adapter emits `EventTurnFailed` then returns `nil` (mirroring the real adapter's exact behavior), on a non-flow-engine-driven run with two seeded `PENDING` steps; asserts the in-flight step settles to `FAILED` and the untouched step stays `PENDING`.
  - existing `TestFlowEngineDrivenRunSkipsBulkProgress/control_bulk_completes` continues to prove a genuinely successful turn still bulk-completes as before.
- impact analysis:
  - `rtk npx gitnexus impact --repo flowpilot runTurn` — `LOW`.

## 10. Follow-Up Document Updates

- upstream docs updated: none required.
- notes left unchanged on purpose: `codex_adapter.go`'s `SendTurn` masked-nil-error behavior on `EventTurnFailed` is left as-is; fixing it properly requires also hardening `finishTurn`'s error-handling switch against a duplicate `EventTurnFailed` emission, out of scope for this targeted fix.
