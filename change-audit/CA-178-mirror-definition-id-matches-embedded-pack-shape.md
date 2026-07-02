# CA-178: Mirrored Definition.ID Matches the Embedded Pack's Shape (BUG-NOTE-CP42 #21)

## Scope

Verified and fixed a P2 issue from `requirements/09-BugFix/todo/BUG-NOTE-CP42.md`: the same built-in flow produced a different `agentpack.FlowDefinition.ID` depending on whether it was resolved from the embedded pack or a Supabase mirror row, violating Task-175's "same normalized shape" requirement.

## The bug

`builtinRecordFromFlow` (the embedded-pack resolution path, `flow_definition_resolver.go`) keeps `Definition.ID` as whatever the flow YAML itself declares (`id: review-loop`). `recordFromWorkflowRow` (the Supabase mirror path, `supabase_workflow_flow_store.go`) instead set `Definition.ID = row.ID` — the mirror row's own database-assigned UUID. So `review-loop`'s `Definition.ID` was `"review-loop"` when resolved from the embedded pack (no mirror row, or store unavailable) but a UUID string once a mirror row existed — the exact same logical flow exposing two different identities depending on an implementation detail of where it happened to be resolved from.

Confirmed latent rather than actively broken: nothing in `internal/runner` currently reads `record.Definition.ID` (or `FlowDefinitionRecord`'s `Definition.ID` field) at all — identity elsewhere in the codebase is tracked via `FlowRef` (already correctly normalized to the stable `"packId/flowId"` ref for both sources) and per-node `FlowNode.ID`. Still worth fixing: any future code that reasonably assumes `Definition.ID` is the flow's semantic id would silently get wrong data depending on mirror-row presence.

## Fix

`recordFromWorkflowRow` now uses `*row.PackFlowID` as `Definition.ID` when the row is a mirrored built-in (i.e. `PackFlowID` is set), matching `builtinRecordFromFlow`'s shape exactly. Falls back to `row.ID` only for a genuine user-owned flow (no `PackFlowID`), which has no other semantic id to use.

## Verification

- Extended the existing `TestSupabaseWorkflowFlowStoreGetByRefBuiltinPackFlowID` test with an assertion that `record.Definition.ID == "review-loop"`, not the row's raw UUID (`"11111111-..."` in the test fixture).
- Full suite: 1008 passed, 15 pre-existing/environmental failures (unchanged from before this fix).
- `go build ./...` clean.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-NOTE-CP42
change_type: bugfix
summary: recordFromWorkflowRow now sets Definition.ID to the pack's own flow id for a mirrored built-in row instead of the row's raw UUID, matching the embedded-pack path's normalized shape
# --->8---
