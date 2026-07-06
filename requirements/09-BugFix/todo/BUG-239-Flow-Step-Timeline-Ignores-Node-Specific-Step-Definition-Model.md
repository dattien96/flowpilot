# BUG-239: Flow Step Timeline Ignores Node-Specific Step Definition Model

## Metadata

- Document ID: `BUG-239`
- Title: `Flow Step Timeline Ignores Node-Specific Step Definition Model`
- Phase: `bugfix`
- Status: `implemented`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-04`
- Last Updated: `2026-07-04`
- Parent Documents: `requirements/09-BugFix/todo/BUG-236-Builtin-Flow-Mirror-Stores-Node-Definition-On-Workflow-Steps-Instead-Of-Step-Definitions.md`
- Child Documents: `none`
- Related Documents: `change-audit/CA-230-agent-delegate-node-per-role-model.md`, `change-audit/CA-231-flow-step-timeline-per-node-posture-display.md`, `change-audit/CA-232-flow-step-posture-resolved-at-seed-time.md`, `change-audit/CA-238-flow-node-definition-step-definitions.md`
- Replaces: `none`
- Tags: `agent-flow-engine, model-resolution, step-definitions, timeline, regression`

## AI Quick View

### Summary

- After moving flow node definition data to `step_definitions`, the flow step timeline could still show the run baseline model (`gpt-5.4-mini`) for reviewer steps even when the reviewer step definition was configured to `gpt-5.4`.
- The older CA-230/231/232 fixes resolved per-role model display, but the new node-specific `step_definitions.node_id` metadata was not exposed through the runner catalog.
- Runtime model resolution therefore could not prefer the exact node definition created by the new BUG-236 mirror shape.
- The first BUG-236 migration draft also repaired existing databases incorrectly: it copied legacy node fields into shared rows such as `flow-agent-delegate-reviewer` instead of creating one node-specific `step_definitions` row per workflow node and repointing `workflow_steps.step_type`.
- `hub.inline` nodes exposed a second regression: the review-loop `synthesis` node declares `agent: agents/synthesizer.md`, so the timeline seed path accidentally treated it like a delegate node and displayed the node-specific `step_definitions.model` value (`haiku`) even though inline synthesis runs on the parent/main run model.

### Current Ask

- Restore the invariant that a flow node's step timeline row shows the model configured on its step definition, not the flow/run baseline fallback.

### Fix

- Expose `node_id`, `behavior_id`, and `agent_ref` through `SupabaseCatalogStore.ListSteps`.
- Extend runtime `Step` with the same metadata.
- Update `resolveFlowNodeModel` to prefer an exact `node_id` match before falling back to the older `flow-agent-delegate-<role>` row.
- Restrict `resolveFlowNodeModel` to `agent.delegate` nodes only, so `hub.inline` nodes such as `synthesis` inherit the parent run posture instead of displaying stale step-definition model data.
- Correct the node-definition migration to insert node-specific step definitions, preserve configured fields such as `model` and `yolo_mode`, and repoint legacy `workflow_steps.step_type` relations to those node-specific rows.
- Add `20260704153000_repair_flow_node_step_definition_relations.sql` so databases that already applied the earlier wrong migration receive the same relation repair.

### Validation

- `rtk go test ./internal/runner -run 'TestResolveFlowNodeModelPrefersNodeSpecificStepDefinition|TestCoderCompletionAutoSpawnsReviewerCohortWithOwnModel|TestWorkflowStepsRuntimeReflectsPerNodeModelOverride|TestSupabaseCatalogStoreShaping' -count=1`
- `rtk go test ./internal/runner -run 'TestRepairMigrationRepointsLegacyWorkflowStepsToNodeDefinitions|TestStepDefinitionsMigrationOwnsFlowNodeDefinitionColumns|TestResolveFlowNodeModelPrefersNodeSpecificStepDefinition|TestSupabaseCatalogStoreShaping|TestWorkflowStepsRuntimeReflectsPerNodeModelOverride' -count=1`
- `rtk go test ./internal/runner -run 'TestResolveFlowNodeModelIgnoresHubInlineAgentRef|TestResolveFlowNodeModelPrefersNodeSpecificStepDefinition|TestWorkflowStepsRuntimeReflectsPerNodeModelOverride|TestRepairMigrationRepointsLegacyWorkflowStepsToNodeDefinitions|TestStepDefinitionsMigrationOwnsFlowNodeDefinitionColumns' -count=1`
- `rtk go test ./internal/agentpack ./internal/runner -count=1`
- `rtk npm run typecheck`
