# Task-200: Step Artifact Instance Binding UI

## Metadata

- Document ID: `Task-200`
- Title: `Step Artifact Instance Binding UI`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-08`
- Last Updated: `2026-07-09`
- Parent Documents: [CP-45: Generic Artifact Types And User-Scoped Artifact Instances](../../07-Coding-Plan/done/CP-45-Generic-Artifact-Types-And-Instances.md)
- Child Documents: `None`
- Related Documents: [Task-196: Per-Step Context Source Selection UI](Task-196-Per-Step-Context-Source-Selection-UI.md), [Task-199: Artifact Instance Settings Page](Task-199-Artifact-Instance-Settings-Page.md), [Task-189: Custom Flow Graph Authoring](../done/Task-189-Custom-Flow-Graph-Authoring.md)
- Replaces: [Task-196: Per-Step Context Source Selection UI](Task-196-Per-Step-Context-Source-Selection-UI.md) for the long-term user-authored flow UX
- Tags: `artifact, step-definition, settings-ui, input-output-binding`

## AI Quick View

### Summary

- Updates step authoring so users bind artifact instances to step input/output slots.
- Supersedes the raw context-source multi-select direction from Task-196 for the long-term UX.
- Keeps bindings attached to step definitions and validates type compatibility.

### Current Ask

- Let a step choose artifact instances as its declared inputs/outputs instead of selecting raw source IDs directly.

### Key Decisions

- `T-1` Step form attaches a **list** of `artifact_instance_id` bindings per direction (input + output), not raw producer config.
- `T-2` Slot compatibility is checked by artifact type and direction; binding the same instance to step A's output and step B's input models cross-step data flow.
- `T-3` Existing `contextSources[]` remains transition-only until Task-201 migrates context behavior.

### Constraints

- Depends on Task-198 and Task-199.
- Preserve BUG-236 boundary: no step binding state on `workflow_steps`.
- Must keep existing flows runnable while CP-44 transition state exists.

### Open Questions

- `Q-1` **(RESOLVED, 2026-07-09)** Output binds a **pre-configured instance** (built-in or user-created); no ephemeral run instances in v1. The instance is the shared handle between producer output and consumer input.

### Source Refs

- `CP-45 P-4`
- `Task-196`
- current code: `WorkflowsSettings.tsx`, `adminModels.ts`, `supabaseAdminRepository.ts`

## 1. Goal

Allow users to wire typed artifact instances into step input/output slots from the workflow step editor.

## 2. Parent Links

- coding plan: `CP-45`
- tech design: `SD-23` (`D-7` type-compatibility)
- system spec: `SS-13`
- specific upstream ids: `CP-45 P-4`

## 3. Trigger

Artifact instances become useful only when steps can declare which ones they consume or produce.

## 4. Exact Change

- `T-1` Add artifact input/output binding controls to the step editor.
- `T-2` Load available artifact instances from the project.
- `T-3` Filter options by compatible artifact type/direction.
- `T-4` Persist bindings via `step_artifact_bindings`.
- `T-5` Display clear binding summary in step cards/lists.

## 5. Touched Areas

- files:
  - `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx`
  - `packages/flowpilot-client-core/src/domain/adminModels.ts`
  - `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts`
  - `apps/local-runner/internal/runner/flow_definition_resolver.go`
- modules:
  - step authoring UI
  - client-core admin repository
  - runner flow resolver
- routes: none
- tables:
  - `step_artifact_bindings`

## 6. Acceptance Check

- User can select artifact instances for step inputs and outputs.
- Selections persist and reload correctly.
- Step card shows selected artifact bindings.
- Invalid type bindings are blocked.
- Existing steps without artifact bindings still load.

### 6.1 Test Items

- `TestStepArtifactBindingsPersistFromStepEditor`
- `TestStepArtifactBindingFiltersByCompatibleType`
- `TestUserFlowWithoutArtifactBindingsStillLoads`
- `TestWorkflowStepsDoNotReceiveArtifactBindingColumns`

### 6.2 Definition of Done

- [x] `DOD-1` Step editor can bind input/output artifact instances. — "Artifact Inputs"/"Artifact Outputs" chip rows + picker modal in `renderStepDefinitionForm`.
- [x] `DOD-2` Bindings persist through `step_artifact_bindings`. — `saveStepDefinition` delete-then-bulk-insert, mirroring the legacy artifact-definitions pattern.
- [x] `DOD-3` Compatibility filtering/validation exists. — UI: `compatibleArtifactInstancesFor` (context.produce sees `context_artifact` instances only, every other behavior sees non-context only); runner: `ValidateFlowArtifactBindings` fail-fast for a required binding to a missing instance (Task-203).
- [x] `DOD-4` Existing flow authoring remains backward compatible. — a step with no `artifactBindings` resolves exactly as before CP-45 (`TestResolveEnabledContextSourceIDsOldCP44FlowUnaffectedByArtifactValidation`).

## 7. Out of Scope

- Creating artifact instances inline inside the step form.
- Custom artifact type creation.
- Context-source migration implementation.

## 8. Completion Notes

- result: `done` — 2026-07-09: chip rows + picker modal + type-compat filter landed in `WorkflowsSettings.tsx`; `tsc --noEmit` clean.
- follow-ups: Task-201 migrates context-source selection onto artifact instances.
- upstream docs updated: `TBD`
