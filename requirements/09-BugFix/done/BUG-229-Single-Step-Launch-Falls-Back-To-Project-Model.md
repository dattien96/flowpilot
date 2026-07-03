# BUG-229: Single-Step Launch Falls Back To Project Model

## Metadata

- Document ID: `BUG-229`
- Title: `Single-Step Launch Falls Back To Project Model`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-03`
- Last Updated: `2026-07-03`
- Parent Documents: [Task-183: User-Owned Model/Provider In Flow Mode](../../08-Task/todo/Task-183-User-Owned-Model-Provider-Resolution-Across-Chat-And-Flow.md), [BUG-165: Implement Step > Flow > Project > Default Model Resolution](./BUG-165-Implement-Step-Flow-Project-Default-Model-Resolution.md), [BUG-183 (YOLO mode single-step/flow split, code comment only — no doc)](../../../apps/local-runner/internal/runner/interactive_handlers.go)
- Child Documents: `none`
- Related Documents: [BUG-227: Flow Mode Main Card Shows Pre-Run Catalog Model Instead Of Resolved Model](./BUG-227-Flow-Mode-Main-Card-Shows-Pre-Run-Catalog-Model-Instead-Of-Resolved-Model.md), [BUG-228: Step-Level Model Override Not Re-Resolved Mid-Flow](./BUG-228-Step-Level-Model-Not-Reresolved-Mid-Flow.md)
- Replaces: `none`
- Tags: `agent-flow-engine, workflow-engine, model-resolution, backend`

## AI Quick View

### Summary

- User restated the canonical model/YOLO resolution definition: Chat mode reads both from the controller; a **normal flow** launch reads YOLO from the Flow and Model from `Step > Flow > Project`; a **direct single-step** launch reads **both** Model and YOLO from the Step only.
- YOLO mode already matched this definition exactly in both branches of `createRun` (Flow-level `wf.YoloMode` for normal-flow launches, Step-level `step.YoloMode` for single-step launches — no Project-level YOLO tier exists anywhere).
- Model resolution did **not** match: the single-step branch (`stepID != "" && (workflowID == "" || stepID != workflowID)`) fell back to `projects.default_model` when the selected step had no model of its own — a Project tier that has no equivalent in the normal-flow branch's sibling YOLO logic and contradicts the "Step only" definition for this launch shape.
- `SS-05` §3 and `SD-06` §6.2/§6.3 previously described a single, undifferentiated `Step > Flow > Project` chain for "workflow/step-mode runs" without distinguishing a normal workflow launch from a direct single-step launch — updated to state the two distinct chains explicitly.

### Current Ask

- A direct single-step launch (`stepId` set, no workflow context) must resolve its model from the selected step's `step_definitions.model` only — no Project fallback. If unset, the run is blocked/non-runnable, same as every other unresolved case (Task-183 T-1).

### Key Decisions

- `F-1` Removed the Project-fallback block from `createRun`'s single-step branch (`interactive_handlers.go`) — an empty step model now returns `no_model_configured` directly instead of trying `projects.default_model`.
- `F-2` Updated the shared Go test fixture (`newInteractiveCatalog()`, `interactive_catalog.go`) so every fixture step carries the same model the project used to (silently) supply via the now-removed fallback, keeping ~25 unrelated tests (approval flow, event replay, run history, skills merge, etc.) passing unchanged — they use `startRun`/`startProjectRun` purely as setup scaffolding and never asserted on model resolution itself.
- `F-3` `SS-05` §3 and `SD-06` §6.2/§6.3 updated to state the two distinct resolution chains: normal workflow launch (`Step > Flow > Project > Unresolved`) vs. direct single-step launch (`Step > Unresolved`, no Flow, no Project).

### Constraints

- Do not change the normal-flow branch's `Step > Flow > Project` chain — it already matched the definition and is independently covered by `BUG-165`'s existing tests.
- Do not touch YOLO resolution — it already matched the definition in both branches; confirmed via existing `TestCreateRunResolvesYoloFromSingleStepWhenEnabled` and friends.

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/interactive_handlers.go:537-655` (`createRun` — normal-flow branch vs. single-step branch)
- `apps/local-runner/internal/runner/interactive_catalog.go:36-53` (`newInteractiveCatalog` — fixture steps given explicit models)
- `apps/local-runner/internal/runner/workflow_model_resolution_test.go` (`TestCreateRunSingleStepResolvesModelFromStepOnly`, `TestCreateRunSingleStepDoesNotFallBackToProjectModel`)
- `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md` §3
- `requirements/06-System-Tech-Design/SD-06-AI-Provider-Integration.md` §6.2, §6.3

## 1. Issue Summary

A direct single-step launch (no workflow context) silently ran on the project's default model when the selected step type had no model of its own configured, instead of being treated as unresolved/non-runnable — inconsistent with the otherwise-identical single-step YOLO resolution (Step only, no Project tier) and with the user-clarified canonical definition for this launch shape.

## 2. Parent Links

- impacted coding plan: `CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor`
- impacted tech design: `SD-06-AI-Provider-Integration.md` §6.2, §6.3 (updated)
- impacted system spec: `SS-05-Workflow-Ai-Provider.md` §3 (updated)

## 3. Environment and Reproduction

- environment: local-runner, Flow Mode `launchMode === "step"` (direct single-step launch, no workflow selected).
- reproduction steps (pre-fix): select a step type with no `step_definitions.model` configured and launch it directly (single-step mode) in a project that has its own `projects.default_model`; the run started successfully on the project's model instead of being blocked.
- frequency: deterministic whenever a single-step launch's step has no model of its own but the project does.

## 4. Expected vs Actual

- expected: single-step launch resolves Step only; an unset step model blocks the run (non-runnable), matching the single-step YOLO behavior and Task-183's mandatory-model principle.
- actual (pre-fix): single-step launch fell back to the project's default model, silently running on a model the user never configured for that step.

## 5. Impact

- users affected: anyone using Flow Mode's direct single-step launch with a step type that has no model configured.
- workflows affected: single-step runs only; normal multi-step workflow launches were already correct (`BUG-165`).
- severity: low-to-medium — no crash, but a step could silently run on the wrong (project-default) model/provider instead of surfacing as a configuration gap to fix.

## 6. Root Cause

- confirmed cause: `createRun`'s single-step branch included a Project-fallback block that has no equivalent in the branch's own YOLO resolution and does not appear in the corresponding normal-flow branch's model resolution's intended per-launch-shape scoping.
- evidence: see Source Refs.

## 7. Fix Strategy

- `F-1`..`F-3` as described in Key Decisions.

## 8. Validation

- `V-1` New tests: `TestCreateRunSingleStepResolvesModelFromStepOnly`, `TestCreateRunSingleStepDoesNotFallBackToProjectModel` — both pass.
- `V-2` Updated `TestCreateRunResolvesYoloFromSingleStepWhenEnabled` fixture (added a step model so the run can start; its own assertion — YOLO resolution — is unchanged).
- `V-3` `go build ./...` in `apps/local-runner` — passes.
- `V-4` `go test ./internal/runner/...` — 1056 passed, 15 failed (same pre-existing, environment-specific failure set as before this change — Windows path mismatches, mocked Codex CLI resume, skills-merge ordering — confirmed identical/subset of the baseline), 14 skipped. No new failures introduced by this change (verified: fixing the shared fixture in `interactive_catalog.go` brought ~25 initially-broken unrelated tests back to green).
- `V-5` Targeted regression re-run: `go test ./internal/runner -run 'TestCreateRun|TestE2EReviewLoop|TestStartTurnWithFlowRefEmitsSyntheticTurnCompletedForHubHandoff|TestAutoReinvokeHub|TestWorkflowStepsRuntime'` — 37 passed.

## 9. Regression Guard

- tests: the two new `TestCreateRunSingleStep*` tests lock in the corrected chain; the existing `TestCreateRunFallsBackToProjectWhenStepAndFlowModelEmpty` (normal-flow branch) continues to lock in that the Project tier still applies there.
- alerts: none.
- audit checks: recorded in `change-audit/CA-228-single-step-model-resolution-step-only.md`.

## 10. Follow-Up Document Updates

- upstream docs updated as part of this fix: `SS-05-Workflow-Ai-Provider.md` §3, `SD-06-AI-Provider-Integration.md` §6.2/§6.3.
- notes left unchanged on purpose: the normal-flow branch's `Step > Flow > Project` chain and its "resolved once, at run start, not re-resolved mid-flow" known limitation (`BUG-228`) are unaffected by this fix.
