CA-691 TUI flow-mode sidebar selected step uses the shared filled-chip selection style

## Problem

User feedback on the TUI Flow-mode right sidebar steps view: the selected
(focused child) step row was highlighted with `styleStatusAgent` — teal
`#2dd4bf` bold text. That hue did not stand out against the unselected rows
(dim `#9b9b9b` / status-colored text), so it was hard to tell which step was
being viewed.

It also broke style parity: every other select/unselect surface in the TUI
(action ring Approve/Deny/gate/question chips, attention bar, /mode-setup tab
row) uses the BUG-333 "filled chip" selection language — bold white (`255`)
on the `62` blue background — not a colored-text highlight.

## Change

- `apps/local-runner/internal/tui/app/app.go`: new `styleStepSelected`
  (bold, fg `255`, bg `62`) — same selection language as `styleRingSelected`
  and the /mode-setup `tabActive` style.
- `apps/local-runner/internal/tui/app/session_panel.go`
  (`flowStepsPanelLinesMax`): the focused-child step row now renders through
  `styleStepSelected` with chip padding inside the fill (leading/trailing
  space, like `renderActionRingChip`), keeping the `▸`/`>` selection marker.
  The inline `· agent: <name>` chip keeps `styleStatusAgent` (it is a label,
  not the selection).

## Tests (additive)

- `apps/local-runner/internal/tui/app/tui_step_selected_style_test.go`:
  - `TestSelectedStepRow_UsesFilledChipStyle` — focused step row renders
    through `styleStepSelected` (claude/codex/grok).
  - `TestSelectedStepRow_NoFocus_NoSelectionMarker` — no selection marker
    without focus.

Pre-existing tests untouched and green: full `go test ./internal/tui/app/`
passes (includes CA-542 marker assertions, which check the `>`/`▸` marker,
not the color).
