# Task-202: File Artifact Type Proof Of Generality

## Metadata

- Document ID: `Task-202`
- Title: `File Artifact Type Proof Of Generality`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-08`
- Last Updated: `2026-07-09`
- Parent Documents: [CP-45: Generic Artifact Types And User-Scoped Artifact Instances](../../07-Coding-Plan/done/CP-45-Generic-Artifact-Types-And-Instances.md)
- Child Documents: `None`
- Related Documents: [Task-197: Artifact Type Catalog And Schema](Task-197-Artifact-Type-Catalog-And-Schema.md), [Task-201: Context Artifact Migration From Context Sources](Task-201-Context-Artifact-Migration-From-Context-Sources.md)
- Replaces: `None`
- Tags: `artifact, file-artifact, proof, prompt-context`

## AI Quick View

### Summary

- Adds a second built-in artifact type, `file_artifact.v1`, to prove CP-45 is not hardcoded to context artifacts.
- Stores file path lists in artifact instance config and validates workspace-safe path handling.
- Allows a consuming step to receive/render file artifact content according to type contract.

### Current Ask

- Implement a small non-context artifact type that uses the same type/instance/binding framework.

### Key Decisions

- `T-1` `file_artifact.v1` config is an explicit bounded `paths: []` list. Flow: step A declares a `file_artifact` **output** carrying the path(s); step B binds the same instance as **input**; the resolver injects the file path into step B's prompt so B knows which file to read.
- `T-2` File artifact readers reuse workspace-safe path protections from `source.excerpt`; dispatch goes through the small `ArtifactTypeRegistry` (mirror `ContextSourceRegistry`), not a new behavior node.
- `T-3` This task proves framework shape, not a full document management system.

### Constraints

- Depends on Task-197, Task-198, and Task-200.
- Do not allow arbitrary outside-workspace reads.
- Keep content bounded before prompt injection.

### Open Questions

- `Q-1` **(RESOLVED, 2026-07-09)** **Read live** at step runtime; the instance carries the path(s), the resolver injects the path (and bounded excerpt) into the consumer prompt at run time — content is not snapshotted at instance creation.

### Source Refs

- `CP-45 P-6`
- current code: `readSourceExcerpts`, `FlowContextExcerpt`

## 1. Goal

Add `file_artifact.v1` as a second built-in artifact type to demonstrate the framework handles more than context-source selection.

## 2. Parent Links

- coding plan: `CP-45`
- tech design: `SD-23` (`D-8` non-context type)
- system spec: `SS-13`
- specific upstream ids: `CP-45 P-6`

## 3. Trigger

CP-45's DOD requires at least one non-context type so the framework does not quietly remain context-specific.

## 4. Exact Change

- `T-1` Define built-in `file_artifact.v1` type with config schema `{ paths: string[] }`.
- `T-2` Add artifact instance form controls for file paths (in the Artifacts tab).
- `T-3` Add the small `ArtifactTypeRegistry` + a `file_artifact` resolver that injects workspace-safe file path(s)/bounded excerpts into the consumer step's prompt at runtime.
- `T-4` Add tests proving step A's `file_artifact` output feeds step B's input (path reaches B's prompt).

## 5. Touched Areas

- files:
  - artifact type catalog seed/migration
  - artifact instance settings UI
  - runner artifact resolver/render path
  - tests around workspace-safe file reads
- modules:
  - artifact type catalog
  - file artifact runtime
  - step prompt assembly
- routes: none
- tables:
  - `artifact_types`
  - `artifact_instances`

## 6. Acceptance Check

- User can create a `file_artifact.v1` instance with one or more workspace paths.
- Step can bind and consume that instance.
- Outside-workspace paths are omitted with clear reasons.
- Prompt/render output is bounded and source-referenced.

### 6.1 Test Items

Implemented (renamed to match `fileArtifactResolver`/`resolveInputArtifactPrompt`, the actual function names — equivalent coverage to the planned names below):

- `TestFileArtifactResolverReadsWorkspaceSafePaths` (was `TestFileArtifactInstanceReadsWorkspaceSafePaths`)
- `TestFileArtifactResolverRejectsOutsideWorkspacePath` (was `TestFileArtifactRejectsOutsideWorkspacePath`)
- `TestResolveInputArtifactPromptInjectsFileArtifactContent` (was `TestStepConsumesFileArtifactInstance`)
- `TestResolveInputArtifactPromptSkipsContextArtifactBindings`
- `TestArtifactTypeRegistryRegisterRejectsDuplicate` / `...ResolveUnknownFails` / `TestDefaultArtifactTypeRegistryHasFileArtifact`

### 6.2 Definition of Done

- [x] `DOD-1` `file_artifact.v1` type exists. — `20260709093000_add_file_artifact_type.sql`.
- [x] `DOD-2` File artifact instance config can be authored. — Artifacts tab (Task-199) file-paths textarea.
- [x] `DOD-3` Runtime can resolve/render bounded file contents. — `fileArtifactResolver` (reuses `readSourceExcerpts`) via `ArtifactTypeRegistry`, injected into the delegate target's prompt at `flow_executor.go`'s `startInlineEntryChain`.
- [x] `DOD-4` Workspace-safety tests pass. — `TestFileArtifactResolverRejectsOutsideWorkspacePath`.

## 7. Out of Scope

- Binary/file upload artifact management.
- Remote file providers.
- Rich document parsing.

## 8. Completion Notes

- result: `done` — 2026-07-09: `ArtifactTypeRegistry` + `fileArtifactResolver` + prompt injection landed; proves the framework generalizes beyond context.
- follow-ups: richer artifact types after CP-45 stabilizes.
- upstream docs updated: `TBD`
