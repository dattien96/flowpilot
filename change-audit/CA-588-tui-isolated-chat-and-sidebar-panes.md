# CA-588: Render chat timeline and F2 sidebar as isolated panes

## What

Right sidebar (`Runner:`/`Path:`/`Project:`) kept bleeding into the chat timeline (`intent:`/`files:`/`symbols:` inside the You box, frame gãy). Prior fixes concatenated `chat + sep + sidebar` per line with `│` in the chat column — any terminal wrap or 1-column PTY delta let sidebar paint over chat.

## Why

- `View()` built one `rows` (messages+status+input) painted to `chatWidth`, then `joinRightSidebar` did `l+sep+s` per line. A long right-aligned You bubble filling `contentW` left `l` at `contentW`; Ghostty/macOS wrap of that line put `Runner:`/`Path:` into the You box (screenshot). `JoinHorizontal` still produced one long line `chat+│+sidebar`.
- `-1` in `contentWidth = fullW - sideW -1` was treated as the separator column; hitting a single `│` in chat was fragile. `safeTermWidth`/`?7l`/`width-2` only narrowed the total, never separated the rectangles.
- Hit-test `sideX = fullW - sideW` included the separator; `stripANSI(View)` tests never saw the terminal composite.

## Fix

- `apps/local-runner/internal/tui/app/session_panel.go:392` — `contentWidth` comment: `-1` is the unused last terminal column (macOS autowrap), not a separator.
- `apps/local-runner/internal/tui/app/session_panel.go:501` — delete `joinRightSidebar`. Add `renderSidebarPane(w,h)` — pad to `h`, `paintRow` each line with `styleSidebar`, `Width/MaxWidth/Height/MaxHeight` block.
- `apps/local-runner/internal/tui/app/app.go:3819` — extract `renderChatPane(w,h)` (ex-`View` body: `tuiChrome`, `renderMessages`, viewport, status, suggestions, modal, input, `padLinesTo`, `paintRow`; no outer `Width/Height` so chat-bar `barW = safeTermWidth(w)` gutter stays `<w` for narrow test). `View()` now only composes:
  ```go
  chat := m.renderChatPane(chatW, h)
  if m.useRightSidebar() { side := m.renderSidebarPane(sideW, h); chat = JoinHorizontal(Top, chat, side) }
  ```
  Two independently rendered blocks, zero shared `│` in chat rows.
- `apps/local-runner/internal/tui/app/mouse.go:63` — `sideX = m.chatWidth()` (end of left pane), not `terminalWidth - sideW`. `hitSidebarChrome` unchanged (`x < sideX` / `x-sideX`).

## Tests

- New additive `apps/local-runner/internal/tui/app/pane_isolation_test.go:10` — `TestRegression_ChatPaneDoesNotContainSidebar` at 197/120 × claude/codex/grok: chat-only prefix (`Width ≤ chatW`) never contains `Runner:`/`session`, full View still contains both, every View line `Width < terminalWidth`.
- `go test ./internal/tui/app -count=1` 13.8s pass, `go vet` clean.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: render chat timeline and F2 sidebar as isolated panes; stop concatenating a separator into chat rows
# --->8---
