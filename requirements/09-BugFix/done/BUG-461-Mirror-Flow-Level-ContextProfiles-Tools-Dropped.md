# BUG-461: Mirrored flow definitions drop flow-level contextProfiles/tools — vibe-sprint fails validation on load

- status: done
- found: live run-34947 (vibe snake run, post-BUG-458 binary)
- fixed_by: CA-958
- tests: internal/runner/bug458_mirror_node_fields_test.go (TestRecordFromWorkflowRowBuiltinMirrorRestoresContextProfiles)

## Symptom (live)

After task_slicer completed, the vibe parent tried to start the first sprint
and failed at flow resolution:

```
[flow-executor] resolve flowRef "flowpilot-core-flow-pack/vibe-sprint" for
run "run-34947" failed: stored definition for "vibe-sprint" failed
validation: flow "vibe-sprint" node "preflight_contract_plan" declares
unknown context profile "scout"
```

Sprint start dead-ended; the run showed running with no pending card.

## Root cause

BUG-458's node-level restore re-attached `contextProfile: scout` onto
mirrored `preflight_contract_plan` — but the flow-level `contextProfiles:`
map itself has no `workflows` column and was never restored, so the restored
node ref resolved against an empty profile map and validation failed
closed. `tools:` (flow-level tool face list) is dropped the same way
(schema has no column; runner consumption is via OfferReviewOutcomeTool so
the runtime impact is fidelity, not the validation blocker).

## Fix

`recordFromWorkflowRow` (supabase_workflow_flow_store.go): for builtin
mirrors, restore `def.ContextProfiles` and `def.Tools` from the embedded
pack via a new `embeddedFlowDefinition` helper (which `embeddedFlowNodesByID`
now shares). Admin-authored rows are untouched — no column exists to
override these, so embedded is authoritative, same contract as BUG-458's
node fields.

## Regression coverage

- TestRecordFromWorkflowRowBuiltinMirrorRestoresContextProfiles — mirrored
  vibe-sprint row must reconstruct with ContextProfiles["scout"] and pass
  ValidateFlowContextSources.
