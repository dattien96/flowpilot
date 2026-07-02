# CA-205: Flow Timeline Sidebar Visible During Approval/Question

## Summary

Fixed `BUG-168`: the Flow Mode step-timeline sidebar unmounted entirely whenever the run paused on an approval card or an `ask_user` question — exactly when the user most needs the step context to make that decision.

## What Changed

- `apps/desktop-flowpilot/src/components/FlowTimelineSidebar.tsx`: the `visible` predicate now also holds during `waiting_approval` and `waiting_question`, not only `running`.

## Verification

- `npm run typecheck` in `apps/desktop-flowpilot` — clean.
- Not verified live (no runner + provider account in this environment) — flagged in `BUG-168` (`V-2`).

## Notes

- Investigated alongside `BUG-169` and `BUG-170` from the same user report (three Flow Mode bugs reported together).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-168
change_type: bugfix
summary: Keep the Flow Mode step-timeline sidebar mounted while the run is paused on an approval or ask_user question instead of hiding it
# --->8---
