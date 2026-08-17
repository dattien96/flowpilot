---
id: CA-532
feature_key: cli-tui
title: Opencode-style canvas — dark overall bg, lighter elevated panels
date: 2026-08-17
status: COMPLETE
---

## Problem

CA-531 lifted the code-block fill to `#2e2e2e` but never painted the **overall**
canvas, so the block color was judged against whatever the user's terminal
background happened to be. On a light-ish terminal the block read **darker**
than its surroundings — the exact inverse of opencode, where the whole window
canvas is the darkest layer (`--bg #0d0d0d`) and the elevated panels (code
blocks, right sidebar, chat bar) are progressively lighter grays.

## Fix (TUI-only)

New opencode-style hierarchy (tokens in `app.go`):

| Layer | Token | Color | Used by |
|---|---|---|---|
| Canvas (darkest) | `colorCanvas` | `#0d0d0d` | whole chat column |
| Right sidebar | `colorBg2` | `#161616` | `styleSidebar` |
| Chat bar | `colorBg3` | `#1e1e1e` | `styleChatBar` |
| Code blocks (lightest) | `colorCodeBg` | `#2e2e2e` | `styleMdCodeBox` |

1. **Canvas painting** (`app.go` `View()`): every row of the chat column is now
   padded and painted with `styleCanvas` so the whole window is `#0d0d0d`
   regardless of terminal bg. The chat-bar (input) rows get `styleChatBar`
   instead, and keep one free last column (`safeTermWidth`) so Windows Terminal
   never wraps the composer.
2. **Right sidebar painting** (`session_panel.go` `joinRightSidebar`): the
   separator column and every sidebar row are painted with `styleSidebar`
   (`#161616`), making the right column a solid elevated panel.
3. **`paintRow` helper** (`chat_box.go`): pads a row to width and paints the
   background in two segments (text + trailing padding), so a lipgloss reset
   inside already-styled text (e.g. code panels with their own `#2e2e2e` bg)
   cannot leak the terminal background behind the row.
4. **Not changed:** message text styles, You-box chrome, hit-testing geometry.
   Painting happens only in the final `View()` assembly / `joinRightSidebar`;
   `renderMessages`, `renderRightSidebar`, `renderInputLine`, `tuiChrome` all
   keep their pre-CA-532 output so mouse click/drag targeting is unaffected.

## Provider impact

Provider-agnostic (Case 1): pure rendering, no `providerKey` read. The
canvas/sidebar/chatbar/code painting test runs × claude/codex/grok.

## Tests

New additive file `app/tui_canvas_bg_test.go`:

- `TestCanvasBg_IsDarkestLayer` — luminance ordering: canvas < sidebar < chat
  bar < code block.
- `TestPaintRow_FillsAndKeepsInnerBg` — padding fills the row, an inner code-bg
  segment survives the canvas wrap, trailing padding is canvas.
- `TestView_PaintsCanvasSidebarChatbarAndCode` — View output carries all four
  backgrounds (forced TrueColor) × claude/codex/grok with the wide right
  sidebar engaged.
- `TestView_PaintsCanvasWithoutSidebar` — narrow layout paints canvas + chat bar
  + code without a sidebar.

## Verification

- `go build ./...` clean; `go vet ./internal/tui/app/` clean; `gofmt` clean on
  all changed files (pre-existing unformatted test files untouched).
- `go test ./internal/tui/... ./internal/cli/... -count=1` all `ok` — legacy
  `TestView_NarrowWidthDoesNotOverflowInputRows`,
  `TestView_ChatSeparatedFromStatusByRule`,
  `TestView_PadsBlankLineAndRuleBeforeStatus` untouched and green.
- `go test ./internal/tui/app -race -count=1` clean.

## Out of scope / residual

- Status line, suggestions and top chrome stay on the canvas (not elevated).
- User/assistant bubbles not repainted (only code blocks, sidebar, chat bar).
- No syntax highlighting.