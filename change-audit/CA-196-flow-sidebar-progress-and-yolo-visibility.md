# CA-196: Flow Sidebar Progress And Yolo Visibility

## Summary

Fixed `BUG-159`, found via live user testing of `BUG-158`: the Flow Mode sidebar's "N/4 steps" counter undercounted the currently-running step, and the yolo badge disappeared entirely when yolo was off instead of showing an explicit off-state.

## What Changed

- `apps/desktop-flowpilot/src/components/FlowTimelineSidebar.tsx`: progress numerator (`reachedCount`) now counts `DONE`/`RUNNING`/`WAITING_USER_APPROVAL` steps instead of only `DONE`; the `YOLO` badge now always renders (`YOLO ON` / `YOLO OFF`) instead of only when on.
- `apps/desktop-flowpilot/src/styles.css`: added `.wsr-retry-badge.yolo-off` muted variant.

## Verification

- `npm run typecheck` in `apps/desktop-flowpilot` — clean.
- Not re-verified live (no backend/Supabase in this environment) — flagged in `BUG-159` (`V-2`).

## Notes

- Direct follow-up from user-reported live screenshots after `BUG-158` landed and the backend was rebuilt.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-159
change_type: bugfix
summary: Count the in-progress step toward the sidebar's progress counter and always show the flow's yolo on/off state instead of hiding it when off
# --->8---
