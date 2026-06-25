# CA-129 — Chat Area Mode-Aware Stroke Border

Added a 2px stroke border to the desktop chat area (`workspace-main`) that reflects the active mode: green for Task, red for Bug, gradient for Flow, none for Normal.

## Files Changed

- `apps/desktop-flowpilot/src/components/ChatWorkspace.tsx`
  - `ChatWorkspace`: reads `chatMode`, `chatStartMode`, `timeline`, `lastTurnInput` from store
  - derives `chatAreaClass` from active mode; applies to `<main>` element
- `apps/desktop-flowpilot/src/styles.css`
  - added `.chat-area-task`, `.chat-area-bug`, `.chat-area-flow` after `.workspace-main`

## Why

Task-114 introduced the chat mode selector (Normal / Task / Bug) but did not add a visual cue to the primary chat region. Users had no glanceable indicator of the active mode while composing. BUG-143 tracks this gap.

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: BUG-143
change_type: bugfix
summary: add mode-aware stroke border to desktop chat area (green=task, red=bug, gradient=flow, none=normal)
# --->8---
