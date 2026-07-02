# CA-222: Propagate Flow Mode YOLO Defaults To Child Agents

## Summary

Fixed a Flow Mode review-loop regression where a workflow/flow definition could have `yolo_mode=true`, but the runtime run still showed `YOLO OFF` and spawned child agents requested approvals. The catalog read path now carries workflow-level and `step_definitions.yolo_mode`, and the runner now applies the user's intended split: normal workflow execution reads YOLO from the workflow, while single-step execution reads it from the selected step. Existing child-agent inheritance then passes the correct YOLO value to coder/reviewer turns.

## What Changed

- `apps/local-runner/internal/runner/provider_event.go`: added `Workflow.YoloMode` and `Step.YoloMode` to the catalog DTOs.
- `apps/local-runner/internal/runner/supabase_catalog_store.go`: selected and mapped `workflows.yolo_mode` and `step_definitions.yolo_mode` for catalog reads.
- `apps/local-runner/internal/runner/interactive_handlers.go`: resolved run-level YOLO from `StartRunInput.YoloMode || workflow.YoloMode` for workflow starts, and from `StartRunInput.YoloMode || step.YoloMode` for direct single-step execution. Workflow-level YOLO is now applied even when the entry step already resolved the run's model.
- `apps/local-runner/internal/runner/workflow_model_resolution_test.go` and `flow_executor_test.go`: added regression coverage for workflow-vs-step YOLO resolution, the workflow-yolo-plus-step-model bug, and parent run/child inheritance.

## Verification

- `go test ./internal/runner -run 'TestCreateRunDoesNotApplyEntryStepYoloToWorkflowLaunch|TestCreateRunResolvesYoloFromWorkflowWhenEnabled|TestCreateRunResolvesWorkflowYoloEvenWhenEntryStepDefinesModel|TestCreateRunResolvesYoloFromSingleStepWhenEnabled|TestStartResolvedFlowChildInheritsWorkflowYoloDefault'` passes.
- Earlier broader check: `go test ./internal/runner -run 'TestCreateRun|TestStartResolvedFlowChildInheritsWorkflowYoloDefault|TestSupabaseCatalog|TestWorkflowStepsRuntimeIncludesRunLevelProviderModelYolo|TestYolo|TestCodexAdapterYolo|TestClaudeAdapterYolo'` passed (24 tests).
- `go test ./internal/runner -run 'TestStartTurnWithFlowRef|TestStartResolvedFlow|TestContinueReinvokeUsesEdgeResolvedTargetForFlowStartedRun'` passes (10 tests).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-42
change_type: bugfix
summary: Resolve Flow Mode YOLO from workflow for normal flow runs and from the selected step for single-step runs so child agents inherit the correct approval posture
# --->8---
