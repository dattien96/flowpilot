# CA-231: Flow Step-Timeline Per-Node Posture Display

## Summary

Follow-up to `CA-230`/`BUG-228`. After the per-role model-resolution fix shipped, the user's live screenshot showed the desktop's step-timeline sidebar (`1/4 steps` panel) still displaying `claude-haiku` for a "Flow: Reviewer" step configured to Claude Sonnet, even though the underlying spawn logic was already fixed. Traced to a display-layer gap distinct from the execution fix: `flowStepRowsFromNodes` (the flow-engine step-runtime seeder) never populates a step row's `Provider`/`Model`, and `WorkflowStepPatch` had no fields to patch them in afterward — so every flow-engine step row stayed permanently blank for the life of the run, and the desktop fell back to displaying the run's single baseline posture for every row regardless of that row's own actual resolved model. Added `Provider`/`Model` to `WorkflowStepPatch`, a `setFlowStepPosture` helper, and `stampFlowNodePosture` in `flow_executor.go`, which resolves each node's effective posture (its own role's `step_definitions` row, else the run's baseline) and patches it onto that node's step-timeline row right after each RUNNING transition.

## What Changed

- `apps/local-runner/internal/runner/workflow_state_machine.go`:
  - Added `WorkflowStepPatch.Provider *string` / `.Model *string`.
- `apps/local-runner/internal/runner/workflow_store.go`:
  - `fakeWorkflowStore.ApplyStepTransition` (the real production `WorkflowStore`, embedded by `localFileSessionStore`) now applies the new `Provider`/`Model` patch fields when present.
  - Guarded the existing unconditional `Status` assignment so a patch that only sets `Provider`/`Model` (empty `Status`) no longer clobbers the step's current status.
- `apps/local-runner/internal/runner/flow_step_runtime.go`:
  - Added `setFlowStepPosture(ctx, parentRunID, nodeID, provider, model)`.
- `apps/local-runner/internal/runner/flow_executor.go`:
  - Added `stampFlowNodePosture(ctx, parentRunID, node)`: resolves the node's effective provider/model (via `resolveFlowNodeModel`, falling back to the run's own baseline the same way `spawnChildRun` does) and calls `setFlowStepPosture`.
  - Called `stampFlowNodePosture` immediately after each of the three `setFlowStepStatus(..., StepStatusRunning)` calls (`startResolvedFlow`, `startInlineEntryChain`, `tryAdvanceFlowFromNode`).
- `apps/local-runner/internal/runner/workflow_step_runtime_test.go`:
  - Added `TestWorkflowStepsRuntimeReflectsPerNodeModelOverride`: drives the flow through the HTTP `/steps-runtime` endpoint and asserts the reviewer nodes' rows show their own `claude`/`claude-sonnet` posture while the coder row shows the run's own baseline.
  - Updated `TestWorkflowStepsRuntimeIncludesRunLevelProviderModelYolo`'s comment (it previously asserted, now-outdatedly, that "a built-in flow node has no provider/model override of its own").
- `requirements/09-BugFix/done/BUG-228-Step-Level-Model-Not-Reresolved-Mid-Flow.md`: added `F-7` and a "Follow-Up Fix (display parity)" section documenting this gap and its fix.

## Verification

- `go build ./...` and `go vet ./internal/runner/...` in `apps/local-runner` — both clean.
- `go test ./internal/runner/...` — 1058 passed, 15 failed (identical pre-existing, environment-specific failure set already established as this session's baseline), 14 skipped.
- `TestWorkflowStepsRuntimeReflectsPerNodeModelOverride` and `TestCoderCompletionAutoSpawnsReviewerCohortWithOwnModel` re-run 10x each (`-count=10`) — 20/20 passed, no flakiness observed.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-183
change_type: bugfix
summary: Stamp each flow-engine step's actual resolved provider/model onto its step-timeline row so the desktop sidebar stops mirroring the run's baseline posture for every step
# --->8---
