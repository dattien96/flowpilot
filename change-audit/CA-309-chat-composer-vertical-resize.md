# CA-309: Make the chat composer vertically resizable for long prompts

## Summary

The chat input `textarea.text-area` had `resize: none`, so a long prompt was
confined to the fixed `rows={2}` box with only internal scroll. Added a custom
**top-edge** drag handle (`.composer-resize-handle`, rendered just above
`.input-bar`) instead of the native bottom-right grip: the composer is pinned to
the bottom of the screen, so it must grow **upward** — a native grip would grow
it off the bottom of the viewport where there is no room.

- `ChatInput` tracks `composerHeight` state and applies it as the textarea's
  inline `height`. Dragging the handle up increases it; clamped to
  `[46px (rows={2} floor), 60vh]`. Double-click resets to the default.
- The drag binds `pointermove`/`pointerup` on `window` for the duration of the
  gesture (bound in `onResizePointerDown`, removed on `pointerup`), so it never
  stalls when the cursor leaves the thin handle. An earlier attempt using
  `setPointerCapture` on the handle dropped events and made the drag feel dead —
  hence the window-listener approach.
- `composerHeight` lives in component state (not tied to the textarea's
  `clearSeq` key), so a dragged height survives the remount-on-send.
- CSS: `.text-area` stays `resize: none` (height is JS-driven) with
  `min-height: 46px`; `.input-bar` `align-items` changed `center` → `flex-end`
  so attach/send stay pinned to the composer's bottom as it grows.

## Why

Requested live: the user wanted to drag the chat box taller when composing a
long prompt.

## Verification

```bash
apps/desktop-flowpilot/node_modules/.bin/tsc -p apps/desktop-flowpilot/tsconfig.json --noEmit
```

Verified in the running dev server (Browser pane) by replicating the exact
`onResizePointerDown` logic and driving it with real PointerEvents: a 120px
upward drag grew height 46→166px; dragging past the top clamped to 432px (60vh
of a 720px viewport); dragging down clamped to 46px; after `pointerup` the
window listeners were removed (further moves ignored). The live composer itself
needs the local runner + a selected project to mount, which the mock/offline
preview can't reach, so the gesture logic was verified in isolation.

## Source

- Live user request ("ô chat tôi muốn kéo rộng ra tùy ý cho case prompt dài").

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: CP-05-06
change_type: feature
summary: make the chat composer textarea vertically resizable (min/max height) and pin input-bar controls to the bottom as it grows
# --->8---
