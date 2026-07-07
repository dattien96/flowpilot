# CA-239: Node-Specific Flow Step Model Resolution

## Summary

- Restored per-step model display after the BUG-236 schema move by exposing node metadata from `step_definitions` through the runner catalog.
- Updated flow node model resolution to prefer an exact `step_definitions.node_id` match before falling back to the older `flow-agent-delegate-<role>` row from CA-230.
- Scoped node-specific model resolution to delegate nodes only, so the review-loop `synthesis` `hub.inline` step inherits the main run posture instead of showing a stale `haiku` step-definition model.
- Fixed the migration repair path for existing databases: legacy `workflow_steps` rows now get node-specific `step_definitions` rows that preserve configured model/Yolo settings, then `workflow_steps.step_type` is repointed away from shared role rows.
- Added a follow-up repair migration for databases that already applied the earlier incorrect `20260703160000` shape, because Supabase will not rerun an edited applied migration.
- Added regression coverage proving a reviewer node configured to `gpt-5.4` does not fall back to a `gpt-5.4-mini` role/default row.

## Validation

- `rtk go test ./internal/runner -run 'TestResolveFlowNodeModelPrefersNodeSpecificStepDefinition|TestCoderCompletionAutoSpawnsReviewerCohortWithOwnModel|TestWorkflowStepsRuntimeReflectsPerNodeModelOverride|TestSupabaseCatalogStoreShaping' -count=1`
- `rtk go test ./internal/runner -run 'TestRepairMigrationRepointsLegacyWorkflowStepsToNodeDefinitions|TestStepDefinitionsMigrationOwnsFlowNodeDefinitionColumns|TestResolveFlowNodeModelPrefersNodeSpecificStepDefinition|TestSupabaseCatalogStoreShaping|TestWorkflowStepsRuntimeReflectsPerNodeModelOverride' -count=1`
- `rtk go test ./internal/runner -run 'TestResolveFlowNodeModelIgnoresHubInlineAgentRef|TestResolveFlowNodeModelPrefersNodeSpecificStepDefinition|TestWorkflowStepsRuntimeReflectsPerNodeModelOverride|TestRepairMigrationRepointsLegacyWorkflowStepsToNodeDefinitions|TestStepDefinitionsMigrationOwnsFlowNodeDefinitionColumns' -count=1`
- `rtk go test ./internal/agentpack ./internal/runner -count=1`
- `rtk npm run typecheck`

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-239
change_type: bugfix
summary: Resolve flow node model from node-specific step_definitions metadata and repair legacy workflow_steps relations away from shared role defaults.
# --->8---
