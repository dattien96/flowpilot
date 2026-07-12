# Task-222: Artifact-Only Step UX And File Artifact Semantics Copy

## Metadata

- Document ID: `Task-222`
- Title: `Artifact-Only Step UX And File Artifact Semantics Copy`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-11`
- Last Updated: `2026-07-11`
- Parent Documents: [CP-45: Generic Artifact Types And Instances](../../07-Coding-Plan/done/CP-45-Generic-Artifact-Types-And-Instances.md)
- Child Documents: `None`
- Related Documents: [Task-196: Per-Step Context Source Selection UI](Task-196-Per-Step-Context-Source-Selection-UI.md), [Task-200: Step Artifact Instance Binding UI](Task-200-Step-Artifact-Instance-Binding-UI.md), [Task-201: Context Artifact Migration From Context Sources](Task-201-Context-Artifact-Migration-From-Context-Sources.md), [Task-202: File Artifact Type Proof Of Generality](Task-202-File-Artifact-Type-Proof-Of-Generality.md), [CA-286](../../../change-audit/CA-286-artifact-only-step-ux-and-file-artifact-copy.md)
- Replaces: raw Step Context Sources as the default authoring UX (Task-196 transition remains data/runner fallback only)
- Tags: `artifact, settings-ui, context-sources, file-artifact, cp-45-residual`

## AI Quick View

### Summary

- Default Step editor is artifact-only: no raw Context Sources `+ Add` chips.
- Legacy raw `contextSources` still visible/clearable only when already present on a step (D-6 fallback data).
- `file_artifact.v1` copy originally stated read/inject only; **superseded for product semantics by Task-223** (OUTPUT write contract + INPUT read). Task-222 still owns artifact-only Step UX (hide raw Context Sources).

### Current Ask

- Close CP-45 live-E2E residual: dual context-source UX + file_artifact mental-model confusion.

### Key Decisions

- `T-1` Hide raw Step Context Sources authoring by default; keep field in model for runner D-6.
- `T-2` Sources for context packages are authored on `context_artifact.v1` instances (Artifacts tab).
- `T-3` _(amended by Task-223)_ File artifact `paths[]` designate deliverables: OUTPUT = write contract, INPUT = read/inject. Task-222's earlier "not a write target" copy is obsolete.
- `T-4` No runner change in this task.

### Constraints

- Do not remove D-6 fallback or migrations.
- Do not expand `resolveInputArtifactPrompt` to all nodes (follow-up).

### Source Refs

- `implementation_plan.md` (Codex Phase-1 guide)
- `WorkflowsSettings.tsx`, `context_sources_builtin.go`, `artifact_type_registry.go`

## 1. Goal

Ship minimal desktop UX/copy so CP-45 E2E authors only artifacts, and file_artifact behavior is unambiguous.

## 2. Parent Links

- coding plan: `CP-45`
- tech design: `SD-23` D-6 / D-8

## 3. Exact Change

- Step form: legacy Context Sources readout only when non-empty; no default add picker.
- Artifacts tab: help text for context sources + file paths; type catalog hints.
- CA-286 ledger entry.

## 4. Definition of Done

- [x] `DOD-1` New Step create/edit does not show raw Context Sources chips by default.
- [x] `DOD-2` Existing raw `contextSources` still visible to clear (legacy).
- [x] `DOD-3` File artifact editor copy documents read/inject, not generate.
- [x] `DOD-4` Desktop typecheck passes; runner untouched.

## 5. Completion Notes

- result: `done` 2026-07-11 — UI-only residual from CP-45 live test (gate-sandbox).
- Live retest: B–D matrix on gate-sandbox including file_artifact bind on implement.
