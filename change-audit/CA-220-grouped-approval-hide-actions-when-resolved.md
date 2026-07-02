# CA-220: Hide Grouped-Approval Bulk Actions Once Resolved

## Summary

Fixed `BUG-182`: after clicking "Approve all" on a grouped approval card, the bulk buttons stayed visible and the count did not update, reading as if the action failed (the decisions were actually applied). `ApprovalGroup` now hides the bulk actions and switches to a resolved label once every item is decided.

## What Changed

- `apps/desktop-flowpilot/src/components/Timeline.tsx`: `ApprovalGroup` computes `unresolved` items; renders "Approve all / Deny all" only while some remain; labels the group "N approvals resolved" when none remain; `bulkDecide` skips already-decided items.

## Verification

- `npm run typecheck` in `apps/desktop-flowpilot` — clean.
- Not verified live (no render-test harness; desktop vitest can't run here — pre-existing ESM config error) — flagged in `BUG-182` (`V-2`).

## Notes

- Sibling `QuestionGroup` has the same shape and is left unchanged (approvals were the reported case).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-182
change_type: bugfix
summary: Hide the grouped-approval Approve-all/Deny-all buttons and show a resolved label once every approval in the group is decided
# --->8---
