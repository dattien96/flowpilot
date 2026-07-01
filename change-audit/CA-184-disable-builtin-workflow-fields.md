# CA-184: Disable Built-In Workflow Fields to Match the Read-Only UX (BUG-NOTE-CP42 #8)

## Scope

Verified and fixed a P3 issue from `requirements/09-BugFix/todo/BUG-NOTE-CP42.md`: `WorkflowsSettings.tsx` told the user a built-in workflow "cannot be edited directly" and hid Save/Delete for it, but every actual form field remained fully editable.

## The bug

The detail panel's copy (`selectedWorkflow?.editable === false ? "This is a built-in template and cannot be edited directly..." : ...`) and the Save/Delete buttons were correctly gated on `editable`, but none of the workflow-level fields (Owner project, Name, Description, Model override, Reasoning effort, YOLO mode) had a `disabled` binding at all. Separately, `renderWorkflowStepsEditor` already had a correctly-gated `readOnly` constant, but it was only wired to the Add/Move/Remove step buttons — every field *inside* each step card (Model override, Reasoning, Enabled, Requires approval, and all six CP-42 flow-node inputs) was missing the same binding. A user could type changes into a built-in workflow's name or a step's `behaviorId` that could never be persisted — undercutting the read-only UX the copy promised.

## Fix

- Added `workflowDetailReadOnly = selectedWorkflow?.editable === false` and bound `disabled={workflowDetailReadOnly}` to all six workflow-level detail fields.
- Added `disabled={readOnly}` to every per-step field inside `renderWorkflowStepsEditor`'s step card: Model override, Reasoning, Enabled, Requires approval, Node ID, Behavior ID, Agent ref, Depends on, Join mode, Cohort — the existing `readOnly` constant (already correctly scoped to `source === "detail" && selectedWorkflow?.editable === false`) needed no changes, only wiring to these additional elements.

## Verification

- `tsc` (the real compiler, not the `npx tsc` stub) against `tsconfig.phase1-tests.json`: no new type errors; the same 3 pre-existing errors in unrelated files remain.
- Full `tests/phase1/*.test.js` suite: 42 passed, 3 pre-existing failures (same unrelated files) — unchanged from baseline.
- **Not verified in a live browser preview**: `desktop-flowpilot` is an Electron app, not a plain web dev server; `preview_start` reported success but no server was actually reachable afterward (`preview_list` returned empty, a screenshot attempt timed out). Verified via `tsc`/tests and direct code review of the JSX instead — the fix is a mechanical `disabled` prop addition with no new logic, low risk for a visual-only miss.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-NOTE-CP42
change_type: bugfix
summary: bind disabled to every workflow-level detail field and every per-step field in WorkflowsSettings.tsx based on the workflow's editable flag, so a built-in (read-only) workflow's fields can no longer be typed into despite Save/Delete already being correctly hidden
# --->8---
