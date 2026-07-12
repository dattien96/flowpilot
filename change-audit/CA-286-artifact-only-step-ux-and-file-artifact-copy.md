# CA-286 — Artifact-Only Step UX And File Artifact Semantics Copy

CP-45 live E2E showed two product confusions: (1) Step editor still exposed raw CP-44 Context Sources chips next to Artifact I/O, and (2) `file_artifact.v1` looked like a coder write target (e.g. generate `summary-coder.md`) when it is only a path-list read/inject contract.

## Files Changed

- `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx`
  - Default Step form no longer shows raw Context Sources authoring (`+ Add` picker).
  - Legacy readout only when `draft.contextSources.length > 0` (clear chips only; no new raw adds).
  - Artifacts tab: clearer copy for `context_artifact.v1` sources and `file_artifact.v1` paths (read/inject, not write).
  - Type catalog hints for both built-in types.
- `implementation_plan.md` — Codex Phase-1 guide for this residual (Task-222).
- `requirements/08-Task/done/Task-222-Artifact-Only-Step-UX-And-File-Artifact-Semantics-Copy.md` — task closeout.

## Scope

UI/copy only for CP-45 residual (Task-222). Runner D-6 precedence, `fileArtifactResolver`, and inject seam in `startInlineEntryChain` intentionally unchanged.

## Residual Notes

- Broader prompt injection of file artifacts on every consumer node (beyond context→delegate hop) remains a separate follow-up.
- Raw `step_definitions.context_sources` data and runner fallback remain for old flows.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-222
change_type: feature
summary: hide default Step raw Context Sources; clarify file_artifact read/inject copy
# --->8---
