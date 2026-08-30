# CA-675 — TUI: Shift+click copies line; motion no longer gated on isLeftMouse

## Context

User (macOS Terminal.app): drag-select produced NO toast at all. Root cause:
Terminal.app does not reliably forward mid-drag mouse **motion** events, and SGR
1006 encodes drag motion/release with `Button=None`, so `handleMouse` dropped them
(`if msg.Shift && isLeftMouse(msg)` / `if isLeftMouse(msg) && !msg.Shift`). The
drag therefore looked like a plain click → `moved=false` → no copy. Separately, a
Shift+**click** (zero-area selection, no motion) selected a 0-width range →
`selectionPlainText` empty → "nothing to copy", no toast.

## Changes (`mouse.go`)

- `handleMouse` rewritten:
  - Shift branch no longer requires `isLeftMouse`; handles Press/Motion/Release
    regardless of `Button` (SGR 1006 motion/release arrive as `Button=None`).
  - Shift **release** with zero-area selection (`x0==x1 && y0==y1`) expands to the
    whole clicked line (`x0=0, x1=1<<20`) so Shift+click copies the line.
  - Plain `MouseActionMotion` routed to `handlePlainLeftMouse` when `mouseDrag.down`
    (so plain drag copies where the terminal does forward motion).
- `autoCopySelectionOnDragEnd` unchanged; `selectionPlainText`/`extractVisualColumns`
  already clamp the wide `x1` via the `toEOL` break.

## Tests

- `tui_drag_copy_test.go::TestShiftClick_CopiesWholeLine` — Shift+click copies the
  full line and toasts "Copied selection.".
- Full `go test ./internal/tui/app` green.

## Verification (Terminal.app)

- **Shift+click** a transcript line → copies the line, toast "Copied selection." at
  chat bottom (always visible). This is the reliable affordance when drag motion is
  not delivered.
- **Shift+drag** a region → copies the region (works where motion arrives).
- **Plain drag** still copies where the terminal forwards motion.
- On clipboard failure the toast is replaced by a "Copy failed: …" system line.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: Task-311
change_type: bugfix
summary: Shift+click copies line; motion events no longer dropped by isLeftMouse gate
# --->8---
