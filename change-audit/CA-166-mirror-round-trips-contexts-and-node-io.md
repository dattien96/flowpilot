# CA-166: Mirror Sync Round-Trips Flow Contexts and Per-Node Inputs/Outputs (BUG-NOTE-CP42 #10)

## Scope

Verified and fixed a P1 issue from `requirements/09-BugFix/todo/BUG-NOTE-CP42.md`: `SupabaseWorkflowFlowStore` silently dropped a pack flow's context graph on every mirror-sync round-trip.

## The bug

`rag-harness.yaml` declares flow-level named context bindings (`contexts.main_context`) and per-node `inputs`/`outputs` maps (e.g. `implement.inputs.main_context`, `context.outputs.main_context`). `SupabaseWorkflowFlowStore`'s `recordFromWorkflowRow`/`replaceSteps`/`Upsert` only round-tripped node identity fields (`node_id`, `behavior_id`, `agent_ref`, `depends_on_json`, `join_mode`, `cohort`, `prompt_template_ref`) and the workflow-level `edges_json` — nothing carried `Contexts`/`Inputs`/`Outputs` through the `workflows`/`workflow_steps` schema at all (no columns existed for them).

**Severity note**: this was confirmed as a real, unambiguous data-loss bug, but it is currently *latent* rather than actively broken — a repo-wide search found nothing in `internal/runner` that reads `agentpack.FlowDefinition.Contexts` or `FlowNode.Inputs`/`Outputs` at runtime today (the executor drives context production/rendering by behavior ID, not by walking these bindings). So no user-visible flow currently misbehaves because of this. It still needed fixing: every mirror-sync round-trip already corrupts this data, and any future feature that does read these bindings (the P2 item #6 in this same bug note, about `context.render`'s `promptTemplate` parsing, points at exactly this kind of wiring) would silently see empty maps for a mirrored flow with no error anywhere.

## Fix

- Migration (new/uncommitted file, edited in place): added `contexts_json jsonb not null default '{}'::jsonb` to `workflows`, and `inputs_json`/`outputs_json jsonb not null default '{}'::jsonb` to `workflow_steps`.
- `dbWorkflowRow` gained `ContextsJSON map[string]dbFlowContextRow`; `dbWorkflowStepRow` gained `InputsJSON`/`OutputsJSON map[string]string`. `workflowSelect`'s embedded `workflow_steps(...)` column list now includes `inputs_json,outputs_json`.
- `recordFromWorkflowRow` decodes `ContextsJSON` into `agentpack.FlowDefinition.Contexts` and each step's `InputsJSON`/`OutputsJSON` into the corresponding `FlowNode.Inputs`/`Outputs`.
- `Upsert`'s workflow payload gained `contexts_json` (via new `contextsPayload` helper); `replaceSteps`'s per-step payload gained `inputs_json`/`outputs_json` (via new `nonNilStringMap` helper, mirroring the existing `nonNilStrings` convention so a nil Go map round-trips as `{}` rather than `null`).

## Verification

- New test `TestSupabaseWorkflowFlowStoreGetByRefDecodesInputsOutputsAndContexts`: asserts a mirror row's `contexts_json`/`inputs_json`/`outputs_json` decode correctly into the normalized `FlowDefinitionRecord`.
- New test `TestSupabaseWorkflowFlowStoreUpsertSendsContextsAndNodeInputsOutputs`: asserts `Upsert`'s `/workflows` payload carries `contexts_json` and `replaceSteps`'s `/workflow_steps` payload carries `inputs_json`/`outputs_json` for the corresponding nodes.
- Full flow-related test group (57 tests: `TestFlow*`, `TestSupabaseWorkflow*`, `TestStartResolvedFlow*`, `TestCoderCompletion*`) passes unchanged.
- `go build ./...` clean.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-NOTE-CP42
change_type: bugfix
summary: add contexts_json/inputs_json/outputs_json columns and wire the store's read/write paths so a mirrored flow's context bindings and per-node input/output maps survive a Supabase round-trip instead of being silently dropped
# --->8---
