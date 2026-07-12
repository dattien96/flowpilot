# BUG-276: File Artifact Input Should Mention Paths Not Paste Content

## Metadata

- Document ID: `BUG-276`
- Title: `File Artifact Input Should Mention Paths Not Paste Content`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Codex`
- Created: `2026-07-11`
- Last Updated: `2026-07-11`
- Parent Documents: [CP-45](../../07-Coding-Plan/done/CP-45-Generic-Artifact-Types-And-Instances.md), [Task-223](../../08-Task/done/Task-223-File-Artifact-Output-Contract-And-Review-Input-Chain.md)
- Child Documents: `None`
- Related Documents: [CA-288](../../../change-audit/CA-288-cp45-e2e-edge-dirty-file-path-only-hub-dedupe.md), [Task-224](../../08-Task/done/Task-224-Flow-Prompt-Scoping-And-Coder-Output-Why-Template.md)
- Replaces: `None`
- Tags: `file-artifact, prompt-composition, done`

## AI Quick View

### Summary

- `file_artifact` INPUT must mention **paths only**; agent reads via tools.
- Full body paste is wrong; only context package pushes content (no durable path handle).
- Fixed in CA-288.

### Current Ask

- Keep closed; Task-224 builds on this for review handoff scoping.

### Key Decisions

- `V-1` INPUT inject = path list + “read with tools”.
- `V-2` OUTPUT write contract remains path list + gate existence.

### Constraints

- Do not reintroduce body paste.

### Open Questions

- None.

### Source Refs

- Live `run-41359` (pre-fix body dump); post-fix `run-658` path-only; CA-288.

## 1. Issue Summary

Reviewer prompts pasted full file content for bound `file_artifact` INPUT.

## 2. Parent Links

- impacted coding plan: `CP-45`
- impacted tech design: `SD-23` (product rule narrows excerpt wording for INPUT)
- impacted system spec: none specific

## 3. Environment and Reproduction

- environment: flow with file INPUT on review
- reproduction: open reviewer prompt after coder wrote deliverable
- frequency: always pre-fix

## 4. Expected vs Actual

- expected: path mentions only
- actual: full file body in prompt

## 5. Impact

- Token bloat; wrong mental model for path-based artifacts.

## 6. Root Cause

- confirmed: `resolveInputArtifactPrompt` used `fileArtifactResolver.Resolve` excerpts.

## 7. Fix Strategy

- `F-1` Path-only listing in `artifact_type_registry.go` (CA-288).

## 8. Validation

- `V-1` Unit tests path-only; live `run-658` path-only section.

## 9. Regression Guard

- tests: `TestResolveInputArtifactPromptMentionsFilePathsOnly`, compose path-only
- audit: CA-288

## 10. Follow-Up Document Updates

- Task-224/BUG-277 for remaining review handoff scoping.
