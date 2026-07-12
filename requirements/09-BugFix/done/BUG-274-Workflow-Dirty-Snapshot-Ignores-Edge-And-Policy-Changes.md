# BUG-274: Workflow Dirty Snapshot Ignores Edge And Policy Changes

## Metadata

- Document ID: `BUG-274`
- Title: `Workflow Dirty Snapshot Ignores Edge And Policy Changes`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Codex`
- Created: `2026-07-11`
- Last Updated: `2026-07-11`
- Parent Documents: [CP-45](../../07-Coding-Plan/done/CP-45-Generic-Artifact-Types-And-Instances.md)
- Child Documents: `None`
- Related Documents: [CA-288](../../../change-audit/CA-288-cp45-e2e-edge-dirty-file-path-only-hub-dedupe.md)
- Replaces: `None`
- Tags: `desktop, workflows-settings, done`

## AI Quick View

### Summary

- Save Workflow stayed disabled when only edges or policy fields changed.
- Root cause: `normalizeWorkflowSnapshot` omitted `edges` and policy.
- Fixed in CA-288.

### Current Ask

- Closed.

### Key Decisions

- `V-1` Dirty fingerprint must include all saved graph fields.

### Constraints

- None remaining.

### Open Questions

- None.

### Source Refs

- `WorkflowsSettings.tsx` `normalizeWorkflowSnapshot`; CA-288.

## 1. Issue Summary

Edges-only (or policy-only) edit did not enable Save Workflow.

## 2. Parent Links

- impacted coding plan: `CP-45`
- impacted tech design: none specific
- impacted system spec: none specific

## 3. Environment and Reproduction

- environment: desktop Workflows settings
- reproduction: edit only edges → Save disabled until description change
- frequency: always

## 4. Expected vs Actual

- expected: edges-only edit enables Save
- actual: Save stayed disabled

## 5. Impact

- Authors could not persist graph edges without dummy field edits.

## 6. Root Cause

- confirmed: `normalizeWorkflowSnapshot` omitted `edges` / policy from dirty compare.

## 7. Fix Strategy

- `F-1` Include stable-normalized edges + policy fields in snapshot (CA-288).

## 8. Validation

- `V-1` Edges-only edit enables Save; save persists edges.

## 9. Regression Guard

- tests: typecheck; manual edges-only save
- alerts: none
- audit: CA-288

## 10. Follow-Up Document Updates

- none beyond CA-288 / CP-45 ledger.
