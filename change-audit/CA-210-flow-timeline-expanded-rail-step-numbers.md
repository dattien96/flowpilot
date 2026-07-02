# CA-210: Flow Timeline Expanded Rail Step Numbers

## Summary

Fixed `BUG-173`: the Flow Mode step-timeline sidebar numbered steps 1-2-3-4 in its collapsed rail but showed blank circles for pending/running steps when expanded, because `STATE_GLYPH` is an empty string for `idle`/`running`. The expanded rail now renders the step index for non-terminal states, matching the collapsed rail.

## What Changed

- `apps/desktop-flowpilot/src/components/FlowStepTimeline.tsx`: the step icon now renders `index + 1` for all non-terminal states in both compact and expanded modes, keeping the `✓`/`✕` glyphs only for `done`/`error`. Status stays conveyed by the `fti-{state}` color classes.

## Verification

- `npm run typecheck` in `apps/desktop-flowpilot` — clean.
- Not verified live (the timeline only renders inside an active Flow run, which needs the Go runner + a provider account, unavailable here) — flagged in `BUG-173` (`V-2`).

## Notes

- Cosmetic sibling to the two functional Flow-mode issues from the same user report; the deeper orchestration defect is tracked separately in `BUG-174`.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-173
change_type: bugfix
summary: Show step numbers 1-2-3-4 in the expanded Flow-mode step-timeline rail to match the collapsed rail
# --->8---
