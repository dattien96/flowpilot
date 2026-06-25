# CA-130 — Chat Intent Type Always Editable

Removed the post-turn locked chip from `ChatStartIntentPanel`; now always shows the 3-tab Normal/Task/Bug selector with colored borders on the active tab, disabled only while the AI is running.

## Files Changed

- `apps/desktop-flowpilot/src/components/ChatWorkspace.tsx`
  - `ChatStartIntentPanel`: removed `hasTurns` locked branch; added `runStatus` selector; added `disabled={isRunning}` to tabs and doc-ID input; added `is-${mode}` class to tab buttons
  - `ChatWorkspace`: simplified `activeChatSubMode = chatStartMode`; removed unused `timeline` and `lastTurnInput` selectors
- `apps/desktop-flowpilot/src/styles.css`
  - `.chat-start-mode-tab`: added `border: 1px solid transparent` + transition
  - `.chat-start-mode-tab.active.is-task`: green border/background
  - `.chat-start-mode-tab.active.is-bugfix`: red border/background
  - `.chat-start-mode-tab:disabled`: opacity + cursor

## Why

Task-114 locked the intent panel after first turn (locked chip, no way to change). BUG-144 reverses this — users need to change intent mid-chat and the panel should always reflect the current selection.

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: BUG-144
change_type: bugfix
summary: remove locked chat intent chip — always show 3-tab selector, disable while running
# --->8---
