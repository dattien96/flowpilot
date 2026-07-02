# CA-225: User-Owned Model/Provider In Flow Mode

## Summary

Implemented user-owned model/provider resolution in Flow Mode, removing the hardcoded `gpt-5.4` fallback floor. When no model is resolved, runs fail with `no_model_configured` instead of silently defaulting. Enabled editing of model override and YOLO mode for built-in workflows without cloning, by updating `saveWorkflow` to write only these three fields when `editable === false`. Updated Agents sidebar panel to show only when a workflow/step is selected, rendering model/provider directly within the Main Agent list item.

## What Changed

- `apps/local-runner/internal/runner/interactive_handlers.go`: Removed hardcoded `gpt-5.4` fallback in `createRun` for both workflow and single-step runs. Added project default model fallback for step resolution.
- `apps/local-runner/internal/runner/interactive_catalog.go`: Configured default model `gpt-5.4` on projects for test compatibility.
- `apps/local-runner/internal/runner/workflow_model_resolution_test.go`: Set default model in test catalog and asserted `no_model_configured` error.
- `apps/local-runner/internal/runner/phase7_test.go`: Added explicit `model: claude-haiku` to test request.
- `apps/local-runner/internal/runner/phase8_a1_test.go`: Configured test project/workflow in custom catalog.
- `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts`: Updated `saveWorkflow` to scoped-update model_override, reasoning_effort_override, and yolo_mode on built-in workflows.
- `apps/desktop-flowpilot/src/types/contract.ts`: Declared model/yoloMode properties on TS interfaces.
- `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx`: Removed `DEFAULT_MODEL` seeding, added empty validation, unlocked inputs on built-in workflows.
- `apps/desktop-flowpilot/src/components/ChatWorkspace.tsx`: Removed standalone green Main Agent card.
- `apps/desktop-flowpilot/src/components/AgentsPanel.tsx`: Updated panel to hide when no flow/step is selected in Flow mode, and display the resolved model + provider inside the main card.

## Verification

- `go test ./internal/runner/...` passes.
- `npm run typecheck` passes.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-183
change_type: bugfix
summary: Implement user-owned model resolution in Flow Mode and integrate model/provider displays directly into Agents Panel
# --->8---
