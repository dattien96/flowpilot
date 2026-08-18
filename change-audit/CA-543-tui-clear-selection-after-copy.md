# CA-543 - drag-selection highlight stays after a successful copy

## Problem

After drag-selecting transcript text and copying it (drag-release auto-copy on
Terminal.app, or Ctrl+C), the "Copied selection." toast appeared but the drag
highlight stayed armed on screen. The transcript remained in "selected" mode
until the next mouse press, which is confusing — the copy already happened and
the UI should return to normal read mode.

## Fix (TUI-only)

- `CopiedMsg` handler (`app.go`): on a successful copy whose kind is `selection`
  (empty kind defaults to `selection`, the existing contract), clear
  `m.mouseSel = mouseSelect{}` before showing the "Copied selection." toast.
- Copy failures keep the selection armed so Ctrl+C remains a working fallback.
- Non-selection kinds (`code`, `answer`, ...) never touch the drag highlight.
- The Ctrl+C path already cleared the selection before issuing the copy; this now
  also covers the drag-release auto-copy path (CA-515) that previously left the
  highlight up.

## Files

- `apps/local-runner/internal/tui/app/app.go`
- `apps/local-runner/internal/tui/app/ca543_clear_selection_after_copy_test.go`
  (new)

## Tests

Additive: `ca543_clear_selection_after_copy_test.go` covers selection success
clears the highlight (3 providers), empty-kind defaults to selection and clears
(3 providers), copy failure keeps the highlight (3 providers), non-selection
kinds leave it untouched, and the full drag→release→`CopiedMsg` flow clearing the
highlight end-to-end. The pre-existing `tui_drag_copy_test.go` assertions (which
checked the selection stayed armed right after release, before the message was
handled) still pass unchanged.

## Verification

- `go build ./...`, `go vet ./internal/tui/app/`, gofmt clean (CRLF repo).
- `go test ./internal/tui/app/ -count=1` green except the two pre-existing
  environmental `TestCmdFocusAgent_*` network failures (confirmed identical on
  the stashed pre-change tree).

## Change Ledger

```json
{
  "flowpilot:change-ledger": {
    "source_doc_id": "CA-543",
    "change_type": "bugfix",
    "feature_key": "cli-tui",
    "impacted_areas": ["tui-mouse-select", "tui-copy"],
    "upstream_docs": ["SS-07", "CP-05"],
    "status": "verified"
  }
}
```
