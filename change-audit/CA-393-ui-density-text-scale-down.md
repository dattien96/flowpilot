# CA-393: Global UI density — text and component scale down

## Summary

Reduced FlowPilot desktop and admin type/component density so the UI reads smaller and tighter without redesigning individual screens.

## What Changed

- `apps/desktop-flowpilot/src/styles.css`: body `14px → 12px`; every `font-size: Npx` reduced by 2px (large display ≥24px by 4px), floor 9px; single- and multi-value `padding` / `gap` reduced by 2px; small control `height`/`min-height` (22–48px) reduced by 2px.
- `apps/admin-web/src/app/globals.css` + `apps/admin-web/src/styles.css`: root `html` font-size `12px` so rem-based Tailwind `text-*` steps down together.

## Notes

- No TSX class rewrites; desktop is almost entirely CSS-px driven.
- Admin scales via rem root rather than bulk `text-sm→text-xs` edits.

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: CP-51
change_type: refactor
summary: Shrink desktop CSS type/spacing by ~2px and admin rem root to 12px for denser UI
# --->8---
