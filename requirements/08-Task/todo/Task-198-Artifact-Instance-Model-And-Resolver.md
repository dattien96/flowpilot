# Task-198: Artifact Instance Model And Resolver

## Metadata

- Document ID: `Task-198`
- Title: `Artifact Instance Model And Resolver`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-08`
- Last Updated: `2026-07-08`
- Parent Documents: [CP-45: Generic Artifact Types And User-Scoped Artifact Instances](../../07-Coding-Plan/todo/CP-45-Generic-Artifact-Types-And-Instances.md), [SD-22: Pluggable Context Source Registry](../../06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md)
- Child Documents: `None`
- Related Documents: [Task-197: Artifact Type Catalog And Schema](Task-197-Artifact-Type-Catalog-And-Schema.md), [BUG-236: Builtin Flow Mirror Stores Node Definition On Workflow Steps](../../09-BugFix/done/BUG-236-Builtin-Flow-Mirror-Stores-Node-Definition-On-Workflow-Steps-Instead-Of-Step-Definitions.md)
- Replaces: `None`
- Tags: `artifact, artifact-instance, resolver, supabase, flow-mode`

## AI Quick View

### Summary

- Adds persisted user-authored artifact instances based on system-owned artifact types.
- Adds `step_artifact_bindings` so step definitions can bind input/output slots to artifact instances.
- Establishes resolver behavior that can load a step's artifact bindings without writing config onto `workflow_steps`.

### Current Ask

- Create the data model, Supabase schema, and resolver plumbing for artifact instances and step bindings.

### Key Decisions

- `T-1` Artifact instances are project-scoped in v1.
- `T-2` Step bindings use a dedicated table, not JSON blobs on `workflow_steps`.
- `T-3` Binding attaches to `step_definitions`, preserving BUG-236's rule that workflow-step rows remain relationship/runtime rows.

### Constraints

- Depends on Task-197.
- Do not migrate CP-44 `contextSources` yet; this task only creates the substrate.
- Missing/stale bindings must be represented as resolver warnings or validation errors, not silent defaults.

### Open Questions

- `Q-1` Should `artifact_instances.config_json` be validated in Supabase constraints or app-level schema validation only? Proposed v1: app-level validation by type schema.

### Source Refs

- `CP-45 P-2`
- `BUG-236`
- current code: `supabaseAdminRepository.ts`, `supabase_workflow_flow_store.go`, `flow_definition_resolver.go`

## 1. Goal

Persist artifact instances and step-to-instance bindings so flow authoring can reference reusable artifact configurations.

## 2. Parent Links

- coding plan: `CP-45`
- tech design: `SD-22`
- system spec: `SS-13`
- specific upstream ids: `CP-45 P-2`

## 3. Trigger

Artifact instance authoring and step binding need durable state. CP-44's single `context_sources` column is too narrow for the generalized artifact model.

## 4. Exact Change

- `T-1` Add `artifact_instances` schema/model with `project_id`, `artifact_type_id`, `name`, `description`, `config_json`, and `status`.
- `T-2` Add `step_artifact_bindings` with `step_definition_id`, `direction`, `slot_name`, `artifact_instance_id`, `required`, and `position`.
- `T-3` Add client-core mapping and repository CRUD/read methods.
- `T-4` Add runner resolver structs so flow definition resolution can carry artifact binding metadata.

## 5. Touched Areas

- files:
  - `packages/flowpilot-client-core/src/domain/adminModels.ts`
  - `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts`
  - `apps/local-runner/internal/runner/flow_definition_resolver.go`
  - `apps/local-runner/internal/runner/supabase_workflow_flow_store.go`
  - new Supabase migration
- modules:
  - client-core admin repository
  - local runner flow resolver/store
- routes: none
- tables:
  - `artifact_instances`
  - `step_artifact_bindings`

## 6. Acceptance Check

- Artifact instances can be saved, loaded, updated, and deleted for a project.
- Step bindings round-trip through Supabase and resolve with the step definition.
- No artifact config is written to `workflow_steps`.

### 6.1 Test Items

- `TestArtifactInstanceRoundTripsThroughRepo`
- `TestStepArtifactBindingRoundTripsThroughRepo`
- `TestSaveWorkflowDoesNotWriteArtifactBindingsToWorkflowSteps`
- `TestFlowDefinitionResolverIncludesStepArtifactBindings`

### 6.2 Definition of Done

- [ ] `DOD-1` Supabase schema exists for artifact instances and step bindings.
- [ ] `DOD-2` Client-core models and repo methods map the new state.
- [ ] `DOD-3` Runner resolver can load bindings attached to step definitions.
- [ ] `DOD-4` BUG-236 boundary is preserved.

## 7. Out of Scope

- Settings UI page.
- Step editor picker UI.
- Context-source migration.
- File artifact producer.

## 8. Completion Notes

- result: `TBD`
- follow-ups: Task-199 creates the user-facing instance page.
- upstream docs updated: `TBD`
