# CA-070: Desktop Chat Turn Skills Summary Chip

## Scope

Desktop chat composer (`apps/desktop-flowpilot`). Normal chat mode only.

## Completed

Added a collapsible skills summary chip that appears directly below the chat input bar (`input-bar`) when one or more skills are selected for the current turn.

**`ChatInput.tsx`**
- Added `skillSummaryExpanded` local state (default `false`).
- Rendered a new `turn-skills-bar` block between `input-bar` and `input-note`. The block is visible only when `isChatMode && selectedSkills.length > 0`.
- Collapsed state shows a single toggle button: "N skill(s) selected" with a caret icon.
- Expanded state shows each selected skill as a monospace pill with a remove button (✕) wired to the existing `removeSkill` handler. Remove is disabled while the runner is blocked.

**`styles.css`**
- Added CSS classes: `turn-skills-bar`, `turn-skills-summary`, `turn-skills-summary-open`, `turn-skills-caret`, `turn-skills-title`, `turn-skills-body`, `turn-skill-row`, `turn-skill-name`, `turn-skill-remove`. Visual language follows the `tool-group-summary` / `tool-group-body` pattern in the Timeline component.

## Verification

- TypeScript typecheck (`npx tsc --noEmit`) passes with no errors.
- No store, runner, or provider contracts changed.

## Residual Notes

- No expanded-state persistence across sessions.
- The chip is only in normal chat mode; workflow/step mode is unaffected.
