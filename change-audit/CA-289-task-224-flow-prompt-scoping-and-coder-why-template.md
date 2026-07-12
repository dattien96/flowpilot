# CA-289 — Task-224 Flow Prompt Scoping And Coder Why Template

Implements Task-224 / BUG-277 after CP-45 live E2E discuss:

1. **History inject scoping** — skip full Prior work block for flow-engine synthesis prompts and flow review handoffs (`isFlowEnginePrompt` / `isFlowReviewHandoffPrompt`). Package hop still carries history for first post-context consumer; hub user turns still inject.
2. **Review path-first** — when target has `file_artifact` INPUT, omit full coder final message from `tryAdvanceFlowFromNode` brief.
3. **Coder OUTPUT template** — required file write section now requires **What / Why / Baseline**; Why must list closed past decisions from package history.

## Files Changed

- `apps/local-runner/internal/runner/feature_history.go`
- `apps/local-runner/internal/runner/flow_executor.go` (`buildFlowReviewHandoffPrompt`)
- `apps/local-runner/internal/runner/artifact_type_registry.go` (template + `nodeHasFileArtifactInput`)
- tests under `runner`
- Task-224 / BUG-277 status → done

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-224
change_type: feature
summary: scope history inject; omit review coder dump; What/Why/Baseline OUTPUT template
# --->8---
