# CA-077: Fix Desktop Chat Turn Skills Summary Visibility

## Scope

Desktop chat composer UI in `apps/desktop-flowpilot`, focused on the selected turn-skills summary introduced by Task-053.

## Completed

- Fixed the slash-picker interaction in `ChatInput.tsx` so selecting a skill from `/` command mode clears the slash query and closes the picker, allowing the selected-skills summary below the input to become the visible state.
- Preserved grouped skill-button multi-select behavior by only closing the picker for slash-triggered selection.
- Added a compact selected-skill preview to the collapsed summary chip, showing the first selected skill names and a remaining-count suffix when needed.
- Strengthened the `turn-skills-*` CSS so the chip is readable against the composer background and visually closer to the existing timeline tool-call summary pattern.

## Verification

- `npm run typecheck --prefix apps/desktop-flowpilot` passed.
- `npm run build --prefix apps/desktop-flowpilot` passed.
- `git diff --check` passed.
- GitNexus impact checks for `ChatInput`, `pickSkill`, and `removeSkill` reported LOW risk.

## Residual Notes

- Browser smoke testing reached the Vite renderer, but the local desktop bootstrap required auth before the chat composer, so authenticated visual verification was not completed in this session.
- No runner, provider, store contract, or data model changes were made.
