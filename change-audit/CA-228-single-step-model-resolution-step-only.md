# CA-228: Single-Step Model Resolution — Step Only

## Summary

User restated the canonical model/YOLO resolution definition across all launch modes: Chat mode reads both from the controller; a normal flow launch reads YOLO from the Flow and Model from Step > Flow > Project; a direct single-step launch reads both Model and YOLO from the Step only. Audited `createRun` against this definition: YOLO already matched exactly in both branches; Model resolution's single-step branch incorrectly fell back to `projects.default_model` when the step had no model of its own. Removed that fallback so a direct single-step launch resolves Step only, matching the already-correct YOLO behavior, and updated the shared Go test fixture plus `SS-05`/`SD-06` to reflect the corrected, launch-shape-specific resolution chains. Filed `BUG-229` (fixed) for this change.

## What Changed

- `apps/local-runner/internal/runner/interactive_handlers.go`:
  - Removed the Project-fallback block from `createRun`'s single-step branch (`runKind != "chat" && stepID != ""`); an empty step model now returns `no_model_configured` directly.
- `apps/local-runner/internal/runner/interactive_catalog.go`:
  - Added an explicit `Model: "gpt-5.4"` to every fixture step in `newInteractiveCatalog()` (`step-plan`, `step-code`, `step-test`, `step-sum`, `bug-repro`, `bug-patch`, `scr-ui`, `scr-vm`) so single-step launches across ~25 unrelated tests keep resolving to the same model/provider they did before, now sourced from the Step tier instead of the removed Project fallback.
- `apps/local-runner/internal/runner/workflow_model_resolution_test.go`:
  - Added `TestCreateRunSingleStepResolvesModelFromStepOnly` and `TestCreateRunSingleStepDoesNotFallBackToProjectModel`.
  - Updated `TestCreateRunResolvesYoloFromSingleStepWhenEnabled`'s fixture to give its step an explicit model (its own assertion, YOLO resolution, is unchanged).
- `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md` §3: documented that the Flow/Project tiers only apply to a normal workflow launch; a direct single-step launch resolves Step only.
- `requirements/06-System-Tech-Design/SD-06-AI-Provider-Integration.md` §6.2/§6.3: split the previously-undifferentiated `Step > Flow > Project` resolution order into the two distinct chains per launch shape.
- `requirements/09-BugFix/done/BUG-229-Single-Step-Launch-Falls-Back-To-Project-Model.md`: new bug doc recording the fix.

## Verification

- `go build ./...` in `apps/local-runner` — passes.
- `go test ./internal/runner/...` — 1056 passed, 15 failed (identical pre-existing, environment-specific failure set as the established baseline: Windows path mismatches, mocked Codex CLI resume, skills-merge ordering), 14 skipped. Confirmed the fixture update in `interactive_catalog.go` was required and sufficient — before it, this same change broke ~25 unrelated tests that used `startRun`/`startProjectRun` purely as setup scaffolding.
- `go test ./internal/runner -run 'TestCreateRun|TestE2EReviewLoop|TestStartTurnWithFlowRefEmitsSyntheticTurnCompletedForHubHandoff|TestAutoReinvokeHub|TestWorkflowStepsRuntime'` — 37 passed.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-183
change_type: bugfix
summary: Fix single-step launches to resolve model from the step only, matching the already-correct single-step YOLO resolution
# --->8---
