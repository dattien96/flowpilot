# Task-197: Artifact Type Catalog And Schema

## Metadata

- Document ID: `Task-197`
- Title: `Artifact Type Catalog And Schema`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-08`
- Last Updated: `2026-07-08`
- Parent Documents: [CP-45: Generic Artifact Types And User-Scoped Artifact Instances](../../07-Coding-Plan/todo/CP-45-Generic-Artifact-Types-And-Instances.md), [SD-23: Generic Artifact Framework](../../06-System-Tech-Design/SD-23-Generic-Artifact-Framework.md), [SD-22: Pluggable Context Source Registry](../../06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md), [SS-13: AI-Followable Document Contract](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md)
- Child Documents: `None`
- Related Documents: [CP-44: Pluggable Context Source Registry](../../07-Coding-Plan/todo/CP-44-Pluggable-Context-Source-Registry.md), [Task-168: Flow Mode Context Package Contract](../done/Task-168-Flow-Mode-Context-Package-Contract.md)
- Replaces: `None`
- Tags: `artifact, artifact-type, schema, catalog, flow-mode`

## AI Quick View

### Summary

- Defines the system-owned `ArtifactType` catalog that CP-45 builds on.
- Captures type metadata, config schema, producer/consumer hints, render contract, and the rule that users cannot create custom types in v1.
- Seeds the first built-in type contracts conceptually: `context_artifact.v1` and later `file_artifact.v1`.

### Current Ask

- Establish the artifact type contract and catalog shape before adding user-created instances or step bindings.

### Key Decisions

- `T-1` `ArtifactType` is system-owned in v1 and is **the only layer hardcoded in code**; user authoring starts at `ArtifactInstance`, not custom type creation. Instances come in two flavors — built-in (seeded, `is_builtin=true`, read-only) and user-created (`is_builtin=false`).
- `T-2` Type metadata includes config schema and consumer hints but not instance-specific config such as selected sources or paths.
- `T-3` Type IDs are stable versioned contracts such as `context_artifact.v1`.

### Constraints

- Do not introduce a user-defined type DSL in this task.
- Preserve existing `flow_context_package.v1` behavior while defining the newer generalized type model.
- Keep schema expressive enough for context and file artifacts without baking in either one.

### Open Questions

- `Q-1` **(RESOLVED, 2026-07-09)** Type catalog + built-in instances **seed qua migration/mirror-sync** (service role, đối xứng built-in Flow) là source-of-truth; startup chỉ verify khớp (SD-23 `Q-5`).

### Source Refs

- `CP-45 P-1`
- `CP-44 P-3/P-4`
- current code: `agentpack.ContextArtifact`, `FlowContextPackage`

## 1. Goal

Define the built-in artifact type contract that all CP-45 artifact instances reference.

## 2. Parent Links

- coding plan: `CP-45`
- tech design: `SD-23` (`SD-22` for the reused context-source registry)
- system spec: `SS-13`
- specific upstream ids: `CP-45 P-1`

## 3. Trigger

CP-45 needs a stable system-owned type layer before user-created artifact instances can be safely persisted, validated, and bound to steps.

## 4. Exact Change

- `T-1` Define `ArtifactType` fields: `id`, `version`, `category`, `producerBehavior`, `consumerHints`, `configSchema`, `renderTemplate`, `systemOwned`, `status`.
- `T-2` Define built-in type IDs and config schema conventions.
- `T-3` Document that users cannot create custom types in v1.
- `T-4` Add tests or fixtures proving type metadata can represent `context_artifact.v1` without hardcoding context-only assumptions.

## 5. Touched Areas

- files:
  - `packages/flowpilot-client-core/src/domain/adminModels.ts`
  - `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts`
  - future Supabase migration files
- modules:
  - artifact catalog domain model
  - admin data mapping
- routes: none
- tables:
  - `artifact_types`

## 6. Acceptance Check

- Built-in artifact type catalog shape is documented and represented in client-core/domain models.
- Type-specific config schema is metadata on type, not copied into every step.
- User-created types are explicitly out of scope.

### 6.1 Test Items

- `TestArtifactTypeCatalogMapsBuiltInTypes`
- `TestArtifactTypeRejectsUserOwnedTypeCreationInV1`
- `TestContextArtifactTypeSchemaDoesNotStoreInstanceSources`

### 6.2 Definition of Done

- [ ] `DOD-1` `ArtifactType` contract exists in docs/domain model.
- [ ] `DOD-2` Built-in type IDs and config schema conventions are defined.
- [ ] `DOD-3` V1 rule "system-owned types only" is enforced or clearly represented.
- [ ] `DOD-4` Context artifact can be represented as a type without storing per-instance sources on the type.

## 7. Out of Scope

- Artifact instance CRUD.
- Step binding UI.
- Runtime producer implementation.
- Custom user-defined artifact types.

## 8. Completion Notes

- result: `TBD`
- follow-ups: Task-198 persists instances and resolver state.
- upstream docs updated: `TBD`
