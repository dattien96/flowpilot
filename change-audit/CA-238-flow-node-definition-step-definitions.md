# CA-238: Flow Node Definition Lives On Step Definitions

## Summary

- Moved builtin flow node-definition persistence from `workflow_steps` relation rows to node-specific `step_definitions` rows.
- Updated Supabase mirror reads/writes, admin Settings Step Definition editing, and runtime run-step decoding to treat `workflow_steps` as relation/order only.
- Adjusted migrations so fresh schemas create flow node fields on `step_definitions`, with a conditional compatibility backfill for older databases that already had legacy relation-table columns.

## Validation

- `rtk go test ./internal/runner -run 'TestSupabaseWorkflowFlowStore|TestStepDefinitionsMigrationOwnsFlowNodeDefinitionColumns|TestSupabaseStoreLoadRunStepsShaping' -count=1`
- `rtk go test ./internal/agentpack ./internal/runner -count=1`
- `rtk npm run typecheck`
- `rtk npm run build`

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-236
change_type: bugfix
summary: Moved flow node definition data from workflow_steps relation rows to step_definitions.
# --->8---
