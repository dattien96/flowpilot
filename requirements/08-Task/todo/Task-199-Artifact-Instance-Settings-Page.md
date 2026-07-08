# Task-199: Artifact Instance Settings Page

## Metadata

- Document ID: `Task-199`
- Title: `Artifact Instance Settings Page`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-08`
- Last Updated: `2026-07-08`
- Parent Documents: [CP-45: Generic Artifact Types And User-Scoped Artifact Instances](../../07-Coding-Plan/todo/CP-45-Generic-Artifact-Types-And-Instances.md)
- Child Documents: `None`
- Related Documents: [Task-197: Artifact Type Catalog And Schema](Task-197-Artifact-Type-Catalog-And-Schema.md), [Task-198: Artifact Instance Model And Resolver](Task-198-Artifact-Instance-Model-And-Resolver.md), [Task-179: Settings Flow Pack Authoring UI](../done/Task-179-Settings-Flow-Pack-Authoring-UI.md)
- Replaces: `None`
- Tags: `artifact, settings-ui, artifact-instance, authoring`

## AI Quick View

### Summary

- Adds a new Settings page/surface for creating and managing artifact instances.
- User selects a built-in artifact type, fills type-specific config, and saves a reusable instance.
- Step editor will later bind these instances rather than authoring raw artifact config inline.

### Current Ask

- Build the artifact instance management surface backed by the schema from Task-198.

### Key Decisions

- `T-1` Artifact instance authoring lives on a dedicated Settings page, not only inside the step form.
- `T-2` Artifact types are shown as a read-only catalog; user actions create instances.
- `T-3` Type-specific forms are generated from built-in config schema where possible, with hand-authored controls for first supported types if needed.

### Constraints

- Depends on Task-197 and Task-198.
- Keep UX consistent with existing Settings surfaces.
- Do not expose custom artifact type creation.

### Open Questions

- `Q-1` Should artifact instances be nested under Workflows settings or have a first-class Settings nav item? Proposed: first-class "Artifacts" or "Artifact Instances" page.

### Source Refs

- `CP-45 P-3`
- current code: `WorkflowsSettings.tsx`, Settings components, `supabaseAdminRepository.ts`

## 1. Goal

Let users create, edit, list, and delete artifact instances from built-in types before binding them to workflow steps.

## 2. Parent Links

- coding plan: `CP-45`
- tech design: `SD-22`
- system spec: `SS-13`
- specific upstream ids: `CP-45 P-3`

## 3. Trigger

CP-45 intentionally keeps artifact configuration out of individual step forms. Users need a dedicated place to create reusable instances.

## 4. Exact Change

- `T-1` Add Settings page/surface for artifact instances.
- `T-2` Show built-in artifact type catalog as read-only source of available types.
- `T-3` Add create/edit form for `artifact_instances.config_json` based on selected type.
- `T-4` Add delete/archive behavior that protects instances still bound to steps.

## 5. Touched Areas

- files:
  - `apps/desktop-flowpilot/src/components/settings/*`
  - `packages/flowpilot-client-core/src/domain/adminModels.ts`
  - `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts`
- modules:
  - desktop Settings UI
  - client-core admin repository
- routes: none
- tables:
  - `artifact_types`
  - `artifact_instances`
  - `step_artifact_bindings` (read for delete protection)

## 6. Acceptance Check

- User can list existing artifact instances.
- User can create an instance from a built-in type and save config.
- User can edit instance metadata/config.
- User cannot create a custom type.
- Deleting a bound instance is blocked or clearly guarded.

### 6.1 Test Items

- `TestArtifactInstanceSettingsListsTypesAndInstances`
- `TestCreateContextArtifactInstanceFromSettings`
- `TestDeleteBoundArtifactInstanceIsBlocked`

### 6.2 Definition of Done

- [ ] `DOD-1` Settings page exists for artifact instance management.
- [ ] `DOD-2` Create/edit/delete flows are wired to Supabase repo methods.
- [ ] `DOD-3` Built-in type catalog is visible but read-only.
- [ ] `DOD-4` Bound-instance delete protection exists.

## 7. Out of Scope

- Step form binding.
- Runtime artifact production.
- Custom type creation.

## 8. Completion Notes

- result: `TBD`
- follow-ups: Task-200 binds instances to steps.
- upstream docs updated: `TBD`
