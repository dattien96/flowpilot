# CA-078: Move Desktop Chat Selected Skills Into Prompt History

## Scope

Desktop chat renderer state and timeline UI in `apps/desktop-flowpilot`.

## Completed

- Moved selected-skill history from composer-only UI into the prompt timeline item so each sent prompt can keep its own skill summary.
- Updated `sendPrompt` in desktop state to persist `selectedSkills` onto the staged prompt bubble before the composer clears its transient selection state.
- Added a prompt-level skill summary renderer in `Timeline.tsx` so the skill UI appears directly below the prompt bubble.
- Removed the separate below-composer selected-skills chip from `ChatInput.tsx` and its companion CSS.

## Verification

- `npm run typecheck --prefix apps/desktop-flowpilot` passed.
- `npm run build --prefix apps/desktop-flowpilot` passed.
- `git diff --check` passed.

## Residual Notes

- This slice preserves prompt-owned skill history during the live desktop session.
- Replayed run history still depends on the event stream payload shape and was not expanded here to carry prompt skill names.

# ---8<--- flowpilot:change-ledger
feature_key: skill-injection
source_doc_id: CA-078
change_type: feature
summary: Move Desktop Chat Selected Skills Into Prompt History
# --->8---
