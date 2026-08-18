# CA-542 - step [back] overlaps the [open] column and flips the child view back

## Problem

In a running flow, clicking the `[open]` chip on a child step focused that agent,
but the focused step row immediately replaced its chip with `[back]` at the exact
same pixel column. A second click at those coordinates (very common — users click
once to open and once to dismiss, or double-click to open) hit the `[back]` chip
instead, snapping straight back to the main view. The sidebar also carried a
redundant `Steps N:` count line under the `steps` section title.

## Fix (TUI-only)

- `flowStepsPanelLines` (`session_panel.go`): dropped the `Steps N:` count line —
  the sidebar/overlay already render a `steps` section title, so the step list is
  just the step rows.
- The focused child step row now keeps its highlight (`styleStatusHi`) but no
  `[back]` chip, so the row never occupies the `[open]` pixel column with a
  different action.
- Added `stepsSectionTitle` (`session_panel.go`): the `steps` section header
  carries `styleStepAgentAction.Render("[back]")` while a child is focused. Both
  the right sidebar (`renderRightSidebar`) and the narrow-terminal overlay
  (`renderSessionPanelOverlay`) render it, and the existing hit-testing
  (`hitSidebarChrome` / `hitSessionAgentChrome`) picks up `[back]` on that line.
- Clicking `[open]` twice at the same coordinates is now idempotent: the second
  click lands on the bare step row (no chip), so the child view stays put.

## Files

- `apps/local-runner/internal/tui/app/session_panel.go`
- `apps/local-runner/internal/tui/app/tui_f2_right_sidebar_test.go` (updated
  `TestStepTodoRestyle_KeepsLegacyTokens` to assert the count line is gone)
- `apps/local-runner/internal/tui/app/tui_open_completed_steps_test.go` (updated
  `TestChatOpenedMsg_CompletedFlowStepsRenderAfterRefresh` count-line assertion)
- `apps/local-runner/internal/tui/app/tui_open_back_chip_stable_test.go` (updated
  to assert `[back]` lives on the steps header, not the row)
- `apps/local-runner/internal/tui/app/tui_step_agent_open_test.go` (updated
  `TestStepPanel_ClickBackReturnsMain` to the header-`[back]` layout)
- `apps/local-runner/internal/tui/app/ca542_steps_header_back_test.go` (new)

## Tests

Additive: `ca542_steps_header_back_test.go` covers the header hosting `[back]`
while viewing a child (3 providers), the focused row holding no chip, the count
line being gone, double-click on the `[open]` pixel not flipping back (3
providers), header-`[back]` clicking returning to main (3 providers), and the
narrow overlay path rendering a hittable header `[back]`. Layout-locking
assertions in four pre-existing tests were updated to the new layout
(operator-approved plan).

## Verification

- `go build ./...`, `go vet ./internal/tui/app/`, gofmt clean (CRLF repo).
- `go test ./internal/tui/app/ -count=1` green except the two pre-existing
  environmental `TestCmdFocusAgent_*` network failures (confirmed identical on
  the stashed pre-change tree).

## Change Ledger

```json
{
  "flowpilot:change-ledger": {
    "source_doc_id": "CA-542",
    "change_type": "bugfix",
    "feature_key": "cli-tui",
    "impacted_areas": ["tui-steps-panel", "tui-sidebar", "tui-mouse"],
    "upstream_docs": ["SS-07", "CP-05"],
    "status": "verified"
  }
}
```
