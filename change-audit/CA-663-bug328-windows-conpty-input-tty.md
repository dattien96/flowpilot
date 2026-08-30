# CA-663 — BUG-328: Windows ConPTY keys via CONIN$ / WithInputTTY

## Evidence (tui.log pid 14012)

Operator clicked the terminal then typed / pressed F2. Event loop stayed alive
(`sessionLoadTimeoutMsg` at 11:39:45). **Zero `KeyMsg` and zero `mouse click`**
until watchdog at 11:40:00. Same class as pid 19296 before the first click that
happened to arrive as a Win32 mouse record.

Bubble Tea v1.3.10 on Windows uses `PeekConsoleInput` on `STD_INPUT_HANDLE`.
Cursor / Windows Terminal ConPTY often never surfaces `KEY_EVENT` on that path.
Cursor CLI reads VT bytes instead.

## Fix

- `tea.WithInputTTY()` on Windows so input is `CONIN$` + VT (`readAnsiInputs`),
  not the coninput peek loop.
- Flush the console input buffer before `NewProgram`.
- Tests with `FLOWPILOT_TUI_SKIP_MODE_RESTORE` no longer append to the operator
  `tui.log` (pid 2516 had been writing fake `mouse click x=2 y=3` into the live log).

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: BUG-328
change_type: bugfix
summary: Read Windows TUI keys from CONIN$ VT instead of PeekConsoleInput
# --->8---
