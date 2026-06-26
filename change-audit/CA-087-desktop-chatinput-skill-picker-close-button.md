# CA-087: Desktop ChatInput Skill Picker Close Button

## Scope

Desktop chat composer UI in `apps/desktop-flowpilot`, focused on the skills picker inside `ChatInput.tsx`.

## Completed

- Added a dedicated close button to the skills picker header so the picker can be dismissed without selecting a skill.
- Kept the current slash-query behavior intact: closing the picker while in `/` mode still clears the input so the picker does not immediately reopen.
- Restyled the picker header to behave like a sticky toolbar with a right-aligned close affordance.

## Verification

- `npm run typecheck --prefix apps/desktop-flowpilot` passed.
- `git diff --check` passed.

## Residual Notes

- This is a UI-only polish slice.
- No store, runner, or provider contract changed.

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: CA-087
change_type: feature
summary: Desktop ChatInput Skill Picker Close Button
# --->8---
