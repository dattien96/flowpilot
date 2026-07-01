# CA-183: Workflow Dirty-Snapshot Tracks CP-42 Node Fields (BUG-NOTE-CP42 #3)

## Scope

Verified and fixed a P2 issue from `requirements/09-BugFix/todo/BUG-NOTE-CP42.md`: editing a CP-42 per-node field in `WorkflowsSettings.tsx`'s step form did not enable the Save button.

## The bug

`normalizeWorkflowSnapshot`'s per-step snapshot only included `stepType`/`orderIndex`/`isEnabled`/`modelOverride`/`reasoningEffortOverride`/`requiresApproval` — it omitted `nodeId`, `behaviorId`, `agentRef`, `dependsOn`, `joinMode`, `cohort` even though the step form has editable inputs for all of them. `workflowDirty` (which gates the Save button) compares this snapshot before/after editing, so changing only one of these fields produced an identical snapshot and Save stayed disabled — a user could type a new `behaviorId` and have no way to persist it unless they also happened to touch some older field.

## Fix

Added the six named fields to the per-step snapshot, plus `promptTemplateRef`/`contextRef` — the same class of CP-42 per-node field, editable in the same form, not named in this specific bug entry but equally missing for the identical reason. `dependsOn` is sorted before stringifying, matching the existing `requiredMcps`/`requiredSkills` sort convention already used in `normalizeStepSnapshot`, so an array-order-only difference (which carries no semantic meaning for a dependency set) doesn't produce a false-dirty flip.

## Verification

- `tsc` (the real compiler, not the `npx tsc` stub — see CA-168) against `tsconfig.phase1-tests.json`: no new type errors; the same 3 pre-existing errors in unrelated files remain.
- Full `tests/phase1/*.test.js` suite: 42 passed, 3 pre-existing failures (same unrelated files) — unchanged from baseline.
- No dedicated unit test added, for the same reason as CA-182: no existing test harness for this component to extend, and the change is additive snapshot-field wiring, not new branching logic.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-NOTE-CP42
change_type: bugfix
summary: normalizeWorkflowSnapshot now includes nodeId/behaviorId/agentRef/dependsOn/joinMode/cohort/promptTemplateRef/contextRef so editing a CP-42 per-node field correctly enables Save instead of requiring an unrelated field to also change
# --->8---
