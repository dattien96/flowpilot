# CA-958 — BUG-461: restore flow-level contextProfiles/tools on builtin mirror reconstruction

- type: bugfix
- bug: BUG-461 (found live run-34947 — vibe-sprint failed validation on sprint start)
- follows: CA-955 (BUG-458 node-level restore)

## Change

`internal/runner/supabase_workflow_flow_store.go`:

- `recordFromWorkflowRow`: for builtin mirrors, restore `def.ContextProfiles`
  and `def.Tools` from the embedded pack when the reconstructed maps are
  empty. Neither field has a `workflows` column, so the mirror always dropped
  them; BUG-458's node-level `ContextProfile` restore re-attached refs that
  then failed `ValidateFlowContextSources` ("unknown context profile").
- New `embeddedFlowDefinition` helper returns the pack FlowDefinition;
  `embeddedFlowNodesByID` now shares it.

## Why

Same contract as BUG-458: for builtin mirrors the embedded pack is the
authoritative source these fields were synced FROM; no admin-editable column
exists, so restoring cannot override anything a user set.

## Tests (additive only)

- `TestRecordFromWorkflowRowBuiltinMirrorRestoresContextProfiles` —
  mirrored vibe-sprint row reconstructs with `ContextProfiles["scout"]` and
  passes `ValidateFlowContextSources`.

## Provider parity

Provider-agnostic: runs in the mirror read path before any provider call.
