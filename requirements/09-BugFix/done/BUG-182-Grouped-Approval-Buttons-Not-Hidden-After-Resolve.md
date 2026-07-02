# BUG-182: Grouped Approval "Approve All / Deny All" Buttons Not Hidden After Resolve

## Metadata

- Document ID: `BUG-182`
- Title: `Grouped Approval "Approve All / Deny All" Buttons Not Hidden After Resolve`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-02`
- Last Updated: `2026-07-02`
- Parent Documents: `requirements/09-BugFix/done/BUG-157-Multiple-Concurrent-Approvals.md`
- Child Documents: `none`
- Related Documents: `none`
- Replaces: `none`
- Tags: `agent-flow-engine, ui, desktop, approvals, review-loop`

## AI Quick View

### Summary

- When several approvals are outstanding, the desktop shows a grouped card with "Approve all" / "Deny all". After clicking "Approve all" the underlying decisions are applied correctly, but the bulk-action buttons stay visible and the header still reads "N approvals required" — as if the action didn't take.
- Root cause: `ApprovalGroup` always rendered the bulk-action buttons and a static "N approvals required" label, with no check for whether the group's items were already decided.
- Fix: compute the unresolved items; hide the bulk buttons and switch the label to "N approvals resolved" once none remain; skip already-decided items in the bulk handler.

### Current Ask

- After resolving a grouped approval, hide the bulk buttons and reflect the resolved state.

### Key Decisions

- `V-1` `ApprovalGroup` derives `unresolved = items.filter(decision === undefined)`; renders the bulk-action row only while `unresolved.length > 0`, and labels the group "N approvals resolved" when empty.

### Constraints

- Desktop-only, display logic in one component. Individual `ApprovalCard`s underneath still show their own resolved state (BUG-157 behavior unchanged).

### Open Questions

- The sibling `QuestionGroup` has the same shape; not changed here (approvals were the reported case). The same pattern would apply if questions show the analogous issue.

### Source Refs

- `apps/desktop-flowpilot/src/components/Timeline.tsx:399` (`ApprovalGroup` — static label + always-rendered bulk actions, pre-fix)

## 1. Issue Summary

The grouped-approval card's "Approve all" / "Deny all" buttons remained visible after the user approved all of them, and the header count did not update, making it look like the approval had not registered (even though it had).

## 2. Parent Links

- impacted coding plan: `requirements/07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md`
- impacted tech design: `none`
- impacted system spec: `none`

## 3. Environment and Reproduction

- environment: desktop-flowpilot; a run with ≥2 concurrent approvals (grouped card).
- reproduction steps:
  1. Reach a state with two or more outstanding approvals (the "N approvals required" group).
  2. Click "Approve all".
  3. Observe the buttons stay and the count does not change.
- frequency: deterministic.

## 4. Expected vs Actual

- expected: after resolving, the bulk buttons hide and the group reads as resolved.
- actual: buttons persist; label unchanged.

## 5. Impact

- users affected: anyone acting on grouped approvals.
- workflows affected: approval UX only; the decisions were applied, but the UI was misleading.
- severity: low-medium — no data issue, but reads as a broken action.

## 6. Root Cause

- confirmed cause: `ApprovalGroup` (`Timeline.tsx:399`) hard-rendered the bulk-action buttons and a `${items.length} approvals required` label with no dependence on the items' `decision` state, so resolving them changed nothing in the group header.

## 7. Fix Strategy

- `F-1` `ApprovalGroup` computes `unresolved = items.filter((it) => it.decision === undefined)`; renders the bulk-action row only when `unresolved.length > 0`; labels the group `${unresolved.length} approvals required` while pending and `${items.length} approvals resolved` once done; and `bulkDecide` skips already-decided items (`Timeline.tsx`).

## 8. Validation

- `V-1` `npm run typecheck` in `apps/desktop-flowpilot` — clean.
- `V-2` Not executed live: no render-test harness for this component, and the desktop `vitest` suite cannot run in this environment (pre-existing ESM config error). Logic is a straightforward derived-state guard; the user should confirm the buttons hide after "Approve all".

## 9. Regression Guard

- tests: none (no render-test harness); typecheck is the gate.
- alerts: none.
- audit checks: recorded in `change-audit/CA-220-grouped-approval-hide-actions-when-resolved.md`.

## 10. Follow-Up Document Updates

- upstream docs that must change: none.
- notes left unchanged on purpose: `QuestionGroup` left as-is (see Open Questions).
