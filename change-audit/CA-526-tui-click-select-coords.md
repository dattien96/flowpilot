---
id: CA-526
feature_key: cli-tui
title: Align click hit-test and drag-select coordinates with the sidebar wrap
date: 2026-08-17
status: COMPLETE
---

## Problem

Three user-visible symptoms, all from one root cause introduced with the F2
right sidebar (CA-524):

1. **Tool-group click hit-box off by a row.** Clicking the `click` hint on
   `▸ 6 tool calls` did not toggle — clicking the message line *above* it did.
2. **Drag highlight drifted from the cursor.**
3. **Copying at the top of a scrolled transcript auto-jumped to the bottom.**

### Root cause

`View()` temporarily mutated `m.width = m.contentWidth()` while painting the
chat column with the sidebar active, then restored `m.width = fullW`. The
transcript therefore wrapped at `contentWidth` when painted but at full terminal
width when hit-tested afterwards:

- `clickTargetAt` (mouse.go) ran `chatRows()` at the full width → fewer wrapped
  rows → the tool-group summary resolved one row *above* its painted position.
- `selectionPlainText` / `applyMouseSelection` used the same mismatched rows, so
  the highlight drifted from the pointer.
- Every frame `View()` called `clampViewport`; when the line count differed from
  what the selection was made against (width flip, tool-group collapse, live
  stream), an offset at the top of history was clamped toward the bottom —
  the "copy at top → scroll to bottom" jump.

## Fix (TUI-only)

**A. One canonical wrap width for paint + hit + select**

- New `chatWidth()` (`session_panel.go`): `contentWidth()` when the sidebar is
  active, terminal width otherwise — the same value `joinRightSidebar` uses to
  truncate the left column (`sideX - 1`).
- `View()` no longer mutates `m.width` (removed both the narrow and the restore).
  All render paths that previously read `m.width` now read `chatWidth()`:
  `buildChatRows`, `chatRowsSig`, the transcript rule, `renderInputLine`,
  `renderStatusLine`, the attach-panel width in `tuiChrome()`, and
  `tryPlaceInputCursor`. `m.width` stays the real terminal width (sidebar
  geometry / mouse bounds are unchanged).

**B. Freeze the viewport while selecting**

- New `selecting()` helper (`viewport.go`): `mouseDrag.down || !mouseSel.empty()`.
- `View()` and `selectionPlainText()` skip `clampViewport` while `selecting()`, so
  a line-count change mid-drag cannot yank the transcript or permanently clamp a
  top-of-history offset to the bottom. The offset re-clamps once the selection is
  cleared.

## Provider impact

Provider-agnostic (Case 1): all touched logic is layout/geometry; the
render/click/drag tests parameterize Claude / Codex / Grok.

## Tests

New additive file `app/tui_click_select_coords_test.go`:

- Wide (120) + F2 expanded: `clickTargetAt` on the *visually painted* summary row
  returns the `tool-group:<key>` target and a real click expands the group ×
  claude/codex/grok (fails on the pre-fix code — the summary was hit a row off).
- Narrow (80, no sidebar): aligned click still resolves (CA-525 regression guard)
  × 3 providers.
- Viewport freeze: at top of history with an armed selection, collapsing a tool
  group (line-count shrink) leaves the model offset unchanged; after the
  selection clears it clamps to the new top.
- Drag on a painted assistant row (wide + sidebar) selects that same row's text
  (no row drift) × 3 providers.

## Verification

- `go build ./...` clean; `go vet ./internal/tui/app/` clean.
- `gofmt` clean on all changed files.
- `go test ./internal/tui/... ./internal/cli/... -count=1` all `ok` (legacy
  untouched and green, incl. CA-524 `tui_f2_right_sidebar_test.go`,
  CA-525 `tui_tool_group_collapse_test.go`, and the drag/char-select suites).
- `go test -race ./internal/tui/app/ -count=1` clean.

## Out of scope / residual

- Desktop untouched.
- `renderSuggestions` still uses fixed-width rows (not wrapped to `chatWidth`);
  it predates this bug and does not affect click/select alignment.
- Edge-scroll (auto-scrolling while dragging past the viewport edge) not added.
- While a frozen selection's frame line count is smaller, `sliceViewport` still
  locally clamps that one frame; the model offset is preserved and re-clamps on
  selection end — no permanent jump.