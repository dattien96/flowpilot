# Task-203: Artifact Framework Validation And Fallback

## Metadata

- Document ID: `Task-203`
- Title: `Artifact Framework Validation And Fallback`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-08`
- Last Updated: `2026-07-08`
- Parent Documents: [CP-45: Generic Artifact Types And User-Scoped Artifact Instances](../../07-Coding-Plan/todo/CP-45-Generic-Artifact-Types-And-Instances.md)
- Child Documents: `None`
- Related Documents: [Task-201: Context Artifact Migration From Context Sources](Task-201-Context-Artifact-Migration-From-Context-Sources.md), [Task-202: File Artifact Type Proof Of Generality](Task-202-File-Artifact-Type-Proof-Of-Generality.md), [CP-44: Pluggable Context Source Registry](../../07-Coding-Plan/todo/CP-44-Pluggable-Context-Source-Registry.md)
- Replaces: `None`
- Tags: `artifact, validation, fallback, migration, e2e`

## AI Quick View

### Summary

- Adds validation and fallback coverage for CP-45 after type, instance, UI, binding, and runtime support exist.
- Ensures old CP-44 flows still run while artifact-instance bindings roll out.
- Defines E2E checks across context artifact, file artifact, stale bindings, and type mismatch.

### Current Ask

- Close CP-45 with migration safety, validation coverage, and documented fallback semantics.

### Key Decisions

- `T-1` Existing CP-44 flows remain valid until explicit migration removes the fallback.
- `T-2` Invalid artifact bindings fail clearly at authoring/load time, not during an opaque provider turn.
- `T-3` Missing optional artifacts degrade; missing required artifacts block with actionable UI.

### Constraints

- Depends on Task-197 through Task-202.
- Do not remove CP-44 fallback in this task.
- Validation must cover both UI authoring and runner flow-load paths.

### Open Questions

- `Q-1` Should required/optional artifact binding failures use the flow-gate modal machinery or Settings validation only? Proposed: Settings validation for authoring, runtime error for stale DB state.

### Source Refs

- `CP-45 P-7`
- `CP-44 DOD-9`
- current code: `flow_definition_resolver.go`, `WorkflowsSettings.tsx`, runner tests for context source binding

## 1. Goal

Make the artifact framework safe to roll out by proving compatibility, validation, and fallback behavior.

## 2. Parent Links

- coding plan: `CP-45`
- tech design: `SD-22`
- system spec: `SS-13`
- specific upstream ids: `CP-45 P-7`

## 3. Trigger

Once artifact instances and step bindings exist, stale/mismatched data and old CP-44 flows become the main rollout risk.

## 4. Exact Change

- `T-1` Add validation for missing artifact type, missing instance, mismatched type, and invalid config.
- `T-2` Add fallback tests for old CP-44 `contexts.sources` flows.
- `T-3` Add E2E/manual test matrix for context and file artifact instances.
- `T-4` Update CP-44/Task-196 docs after CP-45 path is validated.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/flow_definition_resolver.go`
  - `apps/local-runner/internal/runner/*_test.go`
  - `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx`
  - CP/task docs
- modules:
  - runner validation
  - settings validation
  - migration docs
- routes: none
- tables:
  - `artifact_types`
  - `artifact_instances`
  - `step_artifact_bindings`

## 6. Acceptance Check

- Old CP-44 default and flow-level source bindings still run.
- New context artifact instance bindings run.
- New file artifact instance bindings run.
- Missing/mismatched required bindings fail with clear error.
- Optional missing bindings degrade cleanly.

### 6.1 Test Items

- `TestOldContextSourcesFallbackStillWorks`
- `TestArtifactBindingMissingRequiredInstanceFailsClearly`
- `TestArtifactBindingOptionalMissingInstanceWarns`
- `TestArtifactBindingTypeMismatchFailsFlowLoad`
- Manual E2E: create two context instances, bind to different steps, run flow.

### 6.2 Definition of Done

- [ ] `DOD-1` Validation covers stale/missing/mismatched artifact bindings.
- [ ] `DOD-2` CP-44 fallback path remains tested.
- [ ] `DOD-3` Context and file artifact E2E checks are documented.
- [ ] `DOD-4` CP-44/Task-196 docs reflect the migration state.

## 7. Out of Scope

- Removing legacy context source fields.
- Custom artifact type authoring.
- Additional artifact types beyond context/file.

## 8. Completion Notes

- result: `TBD`
- follow-ups: later CP can remove transitional `contextSources[]` once CP-45 adoption is complete.
- upstream docs updated: `TBD`
