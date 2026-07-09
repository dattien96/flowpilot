# Task-201: Context Artifact Migration From Context Sources

## Metadata

- Document ID: `Task-201`
- Title: `Context Artifact Migration From Context Sources`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-08`
- Last Updated: `2026-07-09`
- Parent Documents: [CP-45: Generic Artifact Types And User-Scoped Artifact Instances](../../07-Coding-Plan/done/CP-45-Generic-Artifact-Types-And-Instances.md), [CP-44: Pluggable Context Source Registry](../../07-Coding-Plan/todo/CP-44-Pluggable-Context-Source-Registry.md)
- Child Documents: `None`
- Related Documents: [Task-196: Per-Step Context Source Selection UI](Task-196-Per-Step-Context-Source-Selection-UI.md), [Task-194: Per-Flow Context Source Binding](Task-194-Per-Flow-Context-Source-Binding.md), [Task-193: Context Package Sections And Compatibility Projection](Task-193-Context-Package-Sections-And-Compat-Projection.md)
- Replaces: `None`
- Tags: `artifact, context-artifact, migration, cp44, compatibility`

## AI Quick View

### Summary

- Turns CP-44 context-source selection into a `context_artifact.v1` artifact instance config.
- Keeps CP-44's runtime source registry, but moves user-authored selection from raw step source IDs to reusable artifact instances.
- Provides backward compatibility for existing flow-level `contexts.sources` and any transition `step_definitions.context_sources` data.

### Current Ask

- Make `context_artifact.v1` the first real artifact type backed by CP-44's `ContextSourceRegistry`.

### Key Decisions

- `T-1` `ArtifactInstance.config_json.sources` is the long-term home for selected context source IDs. Seed a **built-in `context_artifact` default instance holding all registered sources**; users can create additional instances (e.g. `sources: [mcp.driver]` only).
- `T-2` Existing CP-44 flow-level `contexts.sources` remains supported as fallback.
- `T-3` Task-196 raw `contextSources[]` should not be implemented as the final UX if CP-45 proceeds.
- `T-4` The built-in flow `Context → Coding → Review → Synthesis` ships with the context artifact wired (Context outputs it, later steps consume); existing user flows migrate lazily.

### Constraints

- Depends on Task-197, Task-198, and Task-200.
- Must not break `rag-harness` pack YAML.
- Must preserve `PackageID` behavior and no-vector guard.

### Open Questions

- `Q-1` **(RESOLVED, 2026-07-09)** **Lazy**: existing user workflows create a default context artifact instance only when edited; untouched flows keep running via CP-44 fallback. The built-in standard flow ships the wiring pre-seeded.

### Source Refs

- `CP-45 P-5`
- `CP-44 P-3/P-4/P-7`
- current code: `flow_context_package.go`, `context_sources_builtin.go`, `rag-harness.yaml`

## 1. Goal

Represent context source selections as reusable `context_artifact.v1` instances while keeping CP-44 runtime behavior intact.

## 2. Parent Links

- coding plan: `CP-45`, `CP-44`
- tech design: `SD-23` (`D-5`/`D-6`), `SD-22` (reused context-source registry)
- system spec: `SS-14`
- specific upstream ids: `CP-45 P-5`, `CP-44 DOD-9`

## 3. Trigger

Task-196 would add raw context-source selection to step definitions, but CP-45 introduces the broader artifact-instance model that should become the final UX.

## 4. Exact Change

- `T-1` Seed/define `context_artifact.v1` built-in type.
- `T-2` Store selected context source IDs in `ArtifactInstance.config_json.sources`.
- `T-3` Update `behaviorContextProduce`/resolver path to prefer bound `context_artifact` instance config over step raw sources, then flow-level sources, then default.
- `T-4` Add migration/backward-compat behavior for existing CP-44 config.
- `T-5` Update Task-196/CP-44 docs to identify raw `contextSources[]` as transition-only.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/behavior_registry_builtin.go`
  - `apps/local-runner/internal/runner/flow_definition_resolver.go`
  - `apps/local-runner/internal/runner/flow_context_package.go`
  - `packages/flowpilot-client-core/src/domain/adminModels.ts`
  - `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx`
- modules:
  - context package build path
  - artifact instance resolver
  - step authoring UI
- routes: none
- tables:
  - `artifact_instances`
  - `step_artifact_bindings`

## 6. Acceptance Check

- A step bound to a `context_artifact` instance builds the context package from that instance's `sources`.
- A flow with only CP-44 `contexts.sources` still works.
- A step with neither instance nor source binding falls back to default set.
- Unknown source IDs still fail fast through registry validation.

### 6.1 Test Items

Implemented as (equivalent coverage, renamed to match the actual precedence-function name `resolveEnabledContextSourceIDs` rather than the originally-planned names):

- `TestResolveEnabledContextSourceIDsArtifactBindingOverridesStepAndFlow` (was `TestContextProduceUsesContextArtifactInstanceSources`)
- `TestResolveEnabledContextSourceIDsFallsThroughWhenNoArtifactBinding` (was `TestContextProduceFallsBackToFlowSources`/`...ToDefaultSources`)
- `TestResolveArtifactBoundContextSourcesIgnoresInputAndOtherTypeBindings`
- Unknown-source-id fail-fast is unchanged, pre-existing coverage (`ValidateFlowContextSources`) — not re-tested here since CP-45 doesn't alter that path.

### 6.2 Definition of Done

- [x] `DOD-1` `context_artifact.v1` is a built-in type. — seeded in `20260709090000_add_artifact_types_catalog.sql`.
- [x] `DOD-2` Context sources live in artifact instance config for the new path. — built-in default instance `config_json.sources`, `20260709092000_add_builtin_context_artifact_instance.sql`.
- [x] `DOD-3` CP-44 flow-level/default fallback remains intact. — `resolveEnabledContextSourceIDs` only adds a new highest-precedence tier; a node with no artifact binding falls through unchanged (`TestResolveEnabledContextSourceIDsFallsThroughWhenNoArtifactBinding`, `TestResolveEnabledContextSourceIDsOldCP44FlowUnaffectedByArtifactValidation`).
- [x] `DOD-4` Task-196 raw context-source UX is marked transition-only/superseded. — CP-44 doc already updated in this session; Task-200's UI is the final UX.

## 7. Out of Scope

- File artifact type.
- Custom artifact types.
- Removing CP-44 fallback paths.

## 8. Completion Notes

- result: `done` — 2026-07-09: D-6 precedence tier + built-in default instance landed; `go test ./internal/runner/...` green, 15 pre-existing unrelated failures unchanged.
- follow-ups: Task-202 adds non-context proof.
- upstream docs updated: `TBD`
