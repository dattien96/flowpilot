# CA-660: BUG-328 disable mouse tracking and keyboard action ring

Disable `tea.WithMouseCellMotion` in `tuiProgramOpts` so Windows conhost no longer enables application mouse mode and steals keyboard focus from the host terminal.

Add a keyboard action ring (`←` `→` `Enter`, `1`–`9`, `Space` for multi-select questions) for approval, question, gate, attention, and parked-flow chips. F2 step picker uses `[` `]` `o` when the session panel is expanded. Attach panel uses Up/Down/Enter/`x`/`d`. Input watchdog no longer arms `lastInputAt` on the first tick without a real key. On startup and periodically on Windows, re-send mouse-off ANSI and disable QuickEdit so host clicks do not wedge conhost input.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: BUG-328
change_type: bugfix
summary: Disable TUI mouse tracking and add keyboard action ring for decision chips (BUG-328)
# --->8---
