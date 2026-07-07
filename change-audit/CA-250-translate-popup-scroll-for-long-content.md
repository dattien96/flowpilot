# CA-250: Cap Translate Popup Result Height And Add Viewport-Aware Positioning

## What changed

- `apps/desktop-flowpilot/src/styles.css`: `.translate-result-text` now sets `max-height` (fallback `240px`, overridden inline once JS measures available space), `overflow-y: auto`, and `overscroll-behavior: contain`. Removed the unconditional `transform: translateX(-50%)` from `.translate-popover` since positioning is now computed in JS.
- `apps/desktop-flowpilot/src/components/TranslatePopup.tsx`: replaced the single-shot `{x, y}` selection anchor with a `useLayoutEffect` that measures the rendered popover and its header, then computes a viewport-clamped `left`/`top` and a dynamic `textMaxHeight` for the scrollable result text — flipping the popover above the selection when there isn't room below, and clamping horizontally within the window. Anchors to a fixed viewport edge rather than a height-dependent offset so the far edge stays within bounds regardless of the box's actual rendered size.

## Why

User-reported UI bug: translating a long text selection made the result popover extend off-screen with no scroll. A capped-height-only fix (first pass) still left the popover mispositioned for selections near the viewport edges — reported as "the popup only shows once, is too small, and can't be dragged into view." See [BUG-252](../requirements/09-BugFix/done/BUG-252-Desktop-Translate-Popup-Result-Missing-Scroll-For-Long-Content.md).

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: BUG-252
change_type: bugfix
summary: make the translate popup's result text scrollable and its position/size viewport-aware so it never renders off-screen
# --->8---
