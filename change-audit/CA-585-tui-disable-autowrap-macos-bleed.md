# CA-585: Disable terminal autowrap to stop macOS sidebar bleed into chat

## What

Sidebar `Runner:`/`Path:` still bled into the You box on macOS (Terminal.app/Ghostty/iTerm) after width-1/gutter fixes. Windows was fine. The desync was at the **terminal layer**, not the View string.

## Why

- `joinRightSidebar` already capped to `safeTermWidth(fullW)` (CA-584), but macOS autowrap (`DECAWM ?7`) is still on. Any line that touches the last column — or a styled SGR that widens the raw width — can wrap and shift the cursor so bubbletea's diff paints the next frame over the chat (You box broken, ` intent:` / `files:` / `symbols:` sharing a row with `Runner:`/`Run:`/`Path:` in screenshot).
- Existing tests only compared `stripANSI(View)` width, so `paintRow`'s `stripANSI` check missed SGR-wide rows; `go test` never sees terminal wrap.
- Earlier fixes only touched You-box wrap/gutter, never the terminal mode.

## Fix

- `apps/local-runner/internal/tui/app/app.go:160` — `decawmOff/on = "\x1b[?7l/h"`, `cmdSetAutoWrap(bool)` writes the CSI. `Init()` is now `tea.Sequence(cmdSetAutoWrap(false), tea.Batch(cmdConnect(), tickCursor()))` so autowrap is off after alt-screen, before first frame. `Run()` restores `?7h` after `p.Run()`.
- `apps/local-runner/internal/tui/app/app.go:192` — `tea.WindowSizeMsg` now returns `tea.ClearScreen` to wipe stale cells after resize/desync.
- `apps/local-runner/internal/tui/app/chat_box.go:142` — `paintRow` now checks `lipgloss.Width(s)` and `lipgloss.Width(out)` (ANSI-aware raw width, not `stripANSI`), truncating after `Render` so no SGR makes the raw line wider than `width`.
- No change to `strokeChatRows`/`hugBoxWidth`; CA-584 join cap is kept.

## Tests

- New additive `apps/local-runner/internal/tui/app/autowrap_terminal_test.go:10` — `TestInit_DisablesAutoWrapFirst`, `TestWindowSize_ClearsScreen`, `TestPaintRow_RawWidthNeverExceeds` (layout-only, provider-agnostic).
- `go test ./internal/tui/app -count=1` 13.7s pass, `go vet ./internal/tui/...` clean.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: disable terminal autowrap and measure paintRow after Render so macOS does not desync sidebar into chat
# --->8---
