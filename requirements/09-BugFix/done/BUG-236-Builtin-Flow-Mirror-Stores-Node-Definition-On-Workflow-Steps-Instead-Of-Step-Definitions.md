# BUG-236: Builtin Flow Mirror Stores Node Definition On `workflow_steps` Instead Of `step_definitions`

## Metadata

- Document ID: `BUG-236`
- Title: `Builtin Flow Mirror Stores Node Definition On workflow_steps Instead Of step_definitions`
- Phase: `bugfix`
- Status: `implemented`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-04`
- Last Updated: `2026-07-04`
- Parent Documents: `requirements/07-Coding-Plan/todo/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md`
- Child Documents: `none`
- Related Documents: `requirements/09-BugFix/done/BUG-160-Workflow-Step-Model-Override-Forced-And-Step-Identity-Hidden.md`, `requirements/09-BugFix/done/BUG-161-Add-Coder-Reviewer-Step-Types-And-Fix-Reasoning-Override.md`, `requirements/09-BugFix/done/BUG-174-Flow-Mode-Workflow-Picker-Run-Does-Not-Orchestrate-No-Agent-Spawn-Steps-Bulk-Completed.md`
- Replaces: `none`
- Tags: `agent-flow-engine, cp-42, supabase, schema, builtin-flow, workflow, step-definition, migration`

## AI Quick View

### Summary

- The current CP-42 mirror model treats `workflow_steps` as the storage location for builtin flow node definition fields such as `behavior_id`, `agent_ref`, `depends_on_json`, `join_mode`, `cohort`, `prompt_template_ref`, `context_ref`, `inputs_json`, `outputs_json`, and now `node_lifecycle`.
- That design conflicts with the intended domain model now explicitly clarified by the product decision: `FLOW = Workflow` and `NODE = Step`.
- Under that model, node definition belongs to `step_definitions`; `workflow_steps` is a workflow-to-step relation and must not become the primary definition store for builtin flow node metadata.
- The generic seeded `step_type` rows such as `flow-agent-delegate` are only dispatch-category placeholders to satisfy the FK from `workflow_steps.step_type` to `step_definitions.step_type`; they do not model real nodes and have forced the real node identity into the wrong table.

### Current Ask

- Redesign builtin flow mirroring so builtin node/step definition data lives in `step_definitions`, and `workflow_steps` only maps a workflow to those steps in order.

### Key Decisions

- `V-1` Product/domain rule: `FLOW = Workflow`, `NODE = Step`.
- `V-2` `step_definitions` is the source of truth for step definition, including builtin flow node definition fields.
- `V-3` `workflow_steps` must remain a relation/ordering table, not the main definition store for builtin flow node data.
- `V-4` The current generic `step_type` seed model (`flow-agent-delegate`, `flow-hub-inline`, etc.) is an implementation workaround, not the correct domain representation.

### Constraints

- The existing schema, admin repositories, runner flow resolver, and settings UI all currently assume CP-42 node metadata is stored on `workflow_steps`; any fix will be cross-layer and must be staged carefully.
- Builtin flow mirror data already exists in the current structure, so the eventual fix will likely require a data migration or a compatibility bridge during rollout.
- The current repo behavior must remain inspectable while the new contract is introduced; a silent schema flip without a compatibility path risks breaking existing mirrored flows and settings screens.

### Open Questions

- Should every builtin node map to a unique `step_definition` key, or should there be a reusable naming convention that still preserves real node identity without falling back to generic dispatch categories?
- Which fields, if any, should remain overridable per workflow-step relation once node definition moves to `step_definitions`?
- Whether manual workflow authoring should allow relation-level overrides at all, or strictly reference immutable step definitions, remains a product decision.

### Source Refs

- `supabase/migrations/20260701090000_add_flow_engine_attrs_to_workflows.sql`
- `supabase/migrations/20260703160000_move_flow_node_definition_to_step_definitions.sql`
- `apps/local-runner/internal/runner/supabase_workflow_flow_store.go`
- `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts`
- `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx`
- `apps/local-runner/internal/agentpack/flow-pack/flows/review-loop.yaml`

## 1. Issue Summary

Builtin flow mirroring currently stores node-definition data on `workflow_steps` rather than `step_definitions`. That makes the workflow-step relation table carry the true definition of builtin flow nodes, which conflicts with the intended domain model where a flow node is a step and step definition belongs in `step_definitions`.

## 2. Parent Links

- impacted coding plan: `requirements/07-Coding-Plan/todo/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md`
- impacted tech design: `none identified`
- impacted system spec: `none identified`

## 3. Environment and Reproduction

- environment: FlowPack builtin flow mirror (`review-loop`, `rag-harness`) into Supabase-backed `workflows` + `workflow_steps`; desktop Settings `Workflows` screen; runner flow definition resolver.
- reproduction steps:
  1. Inspect the builtin flow mirror migration and the flow store insert path.
  2. Observe that mirrored node fields are written to `workflow_steps`, while `step_definitions` only contains generic dispatch-category rows such as `flow-agent-delegate`.
  3. Inspect the settings UI and note that flow node fields are edited under workflow-step cards, not on the step-definition page.
- frequency: deterministic by schema and code design.

## 4. Expected vs Actual

- expected: if `FLOW = Workflow` and `NODE = Step`, builtin node/step definition fields should live in `step_definitions`, and `workflow_steps` should only relate a workflow to those steps in order.
- actual: builtin node/step definition fields are mirrored onto `workflow_steps`, while `step_definitions` holds only generic dispatch categories and not the real builtin node definitions.

## 5. Impact

- users affected: maintainers authoring or inspecting builtin flows and custom flows in Settings; any future implementation that needs a clean `Flow/Step` contract.
- workflows affected: builtin flow mirroring, settings editing semantics, runner definition resolution, and any future schema migration for CP-42.
- severity: medium-high — current behavior can function at runtime, but the schema and UI semantics are mis-modeled, making further flow work harder and increasingly inconsistent.

## 6. Root Cause

- hypothesis: CP-42 was implemented with a compatibility-first bridge that reused the existing `workflow_steps.step_type -> step_definitions.step_type` FK shape, so the real node definition had to be stored somewhere else and landed on `workflow_steps`.
- confirmed cause: `20260701090000_add_flow_engine_attrs_to_workflows.sql` explicitly seeds generic `step_definitions` rows like `flow-agent-delegate` only to satisfy the FK, while the real mirrored node identity is stored in `workflow_steps.node_id`, `behavior_id`, `agent_ref`, `depends_on_json`, `join_mode`, `cohort`, `prompt_template_ref`, `context_ref`, `inputs_json`, and `outputs_json`. The newer `node_lifecycle` migration extends the same mistaken pattern.
- evidence:
  - `supabase/migrations/20260701090000_add_flow_engine_attrs_to_workflows.sql` states that mirrored nodes need a valid `step_type` even though their real identity lives in `node_id/behavior_id/agent_ref`.
  - `apps/local-runner/internal/runner/supabase_workflow_flow_store.go` writes builtin node fields directly into `workflow_steps`.
  - `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx` edits these fields under workflow-step cards, confirming the UI follows the same mis-modeling.

## 7. Fix Strategy

- `F-1` Define the target domain contract for `builtin flow`, `workflow`, `step_definition`, and `workflow_step` before any further CP-42 schema changes.
- `F-2` Move builtin node-definition fields from `workflow_steps` to `step_definitions` in the target schema.
- `F-3` Redesign builtin flow mirror so each builtin node maps to a real `step_definition`, not just a generic dispatch-category `step_type`.
- `F-4` Reduce `workflow_steps` to relation/order/resolution fields only, preserving only fields that are intentionally relation-level overrides.
- `F-5` Move editing of builtin node definition fields to the Step Definition page and remove them from the workflow-step editor once the schema supports the corrected model.

### Implementation Notes

- Builtin flow mirror now upserts one node-specific `step_definitions` row per flow node and inserts `workflow_steps` only as relation/order rows.
- Runtime run-step decoding reads node identity and behavior from nested `step_definitions`, with compatibility fallback for legacy responses.
- Settings now edits node lifecycle, behavior, agent, dependency, join/cohort, prompt/context, and IO bindings on the Step Definition form rather than the workflow-step relation card.

## 8. Validation

- `V-1` Confirm the target schema makes `step_definitions` the single source of truth for builtin node definition.
- `V-2` Confirm builtin flow mirror round-trips from YAML to DB and back without needing `workflow_steps` to carry node-definition data.
- `V-3` Confirm the settings UI exposes definition fields on the Step Definition page and no longer requires editing them in workflow-step relation cards.
- `V-4` Verified with focused runner tests, full local-runner agentpack/runner tests, desktop typecheck, and desktop build on `2026-07-04`.

## 9. Regression Guard

- tests: add schema and mirror-sync tests proving builtin node definition is reconstructed from `step_definitions`, not `workflow_steps`.
- alerts: none.
- audit checks: verify no new migration adds definition-only fields back onto `workflow_steps` without an explicit override rationale.

## 10. Follow-Up Document Updates

- upstream docs that must change: `CP-42` needs an explicit contract section that distinguishes `workflow`, `step_definition`, and `workflow_step` under the `FLOW = Workflow`, `NODE = Step` model.
- notes left unchanged on purpose: this bug document does not choose the final `step_type` naming convention yet; that should be resolved in the follow-up design before code migration begins.
