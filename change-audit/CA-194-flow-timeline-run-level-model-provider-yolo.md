# CA-194: Flow Timeline Run-Level Model/Provider/Yolo

## Summary

Fixed `BUG-158`: the Flow Mode sidebar showed no model/agent/yolo data at all, because a flow-engine node genuinely has no per-node model/provider override to read (no data source existed), and yolo was being surfaced at the wrong level (step-type default instead of the run's own posture). Also numbered the collapsed rail's icons.

## What Changed

- `apps/local-runner/internal/runner/interactive_handlers.go`: `workflowStepsRuntimeSnapshot` gained top-level `provider`/`model`/`yoloMode`, sourced from the run's own `providerKey`/`modelName`/`yolo` in `workflowStepsRuntime`.
- `apps/local-runner/internal/runner/workflow_step_runtime_test.go`: added `TestWorkflowStepsRuntimeIncludesRunLevelProviderModelYolo`.
- `apps/desktop-flowpilot/src/types/contract.ts`: `WorkflowStepsRuntimeSnapshot` gained matching optional fields.
- `apps/desktop-flowpilot/src/state/store.ts`: added `workflowStepRuntimeMeta` state, populated in `refreshWorkflowStepRuntime`, reset alongside every existing `workflowStepRuntime: []` reset.
- `apps/desktop-flowpilot/src/components/FlowStepTimeline.tsx`: accepts `runProvider`/`runModel` fallback props (`step.provider || runProvider`); compact mode now shows `index + 1` instead of the state glyph.
- `apps/desktop-flowpilot/src/components/FlowTimelineSidebar.tsx`: reads `workflowStepRuntimeMeta`, shows a `YOLO` pill + provider/model chips in the expanded summary header, passes `runProvider`/`runModel` to `FlowStepTimeline`.
- `apps/desktop-flowpilot/src/styles.css`: `.flow-sidebar-meta`, `.fti-idle .fti-icon` text color for numbered idle steps.

## Verification

- `go build ./...` + `go test ./internal/runner/... -run 'TestWorkflowStepsRuntime'` in `apps/local-runner` — pass.
- `npm run typecheck` in `apps/desktop-flowpilot` — clean for every file this change touches.
- Not verified live (no backend/Supabase available in this environment) — flagged in `BUG-158` (`V-4`). The reported "step name still generic" symptom is very likely a stale, not-yet-rebuilt local-runner binary (the fix for that already shipped in `BUG-155`); the user should rebuild/restart the Go backend and re-check.

## Notes

- Deliberately did not add a fake per-node `provider_override`/`model_override` data source — none exists in the flow YAML schema or the built-in agent definitions, so a step's model/provider is, correctly, always the run's own.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-158
change_type: bugfix
summary: Surface the run's own provider/model/yolo in the Flow Mode sidebar (with per-step fallback) instead of a nonexistent per-node override, and number the collapsed timeline rail
# --->8---
