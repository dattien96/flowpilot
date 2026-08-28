# CA-661 — BUG-328: stop SetConsoleMode after bubbletea input reader starts

## Problem

Session pid 4768 (tui.log): BUG-328 build loaded, `SessionDefaultsMsg` arrived, async runner
msgs kept flowing, but **zero `KeyMsg` for the entire session** — F2/F4 never reached `Update`.

Root cause: `cmdEnsureConsoleInputReady()` in `Init()` and throttled `maybeRearmConsoleInput()`
called `disableConsoleQuickEdit()` → `SetConsoleMode` on stdin **after** bubbletea v1.3.10 had
started the Windows `coninput` reader. That wedges `ReadConsoleInput` while the event loop stays
alive.

## Fix

- `primeConsoleBeforeProgram()` — QuickEdit + mouse-off ANSI **once** in `Run()` before `tea.NewProgram`.
- `Init()` only runs `cmdEnsureMouseTrackingOff()` (stdout ANSI, no `SetConsoleMode`).
- `maybeRearmConsoleInput` (Windows) and `ClipboardPasteMsg` re-arm mouse-off ANSI only.
- Watchdog: `inputExpectedSince` set on first `SessionDefaultsMsg`; flags startup stall when session
  is ready but no keys arrived for 45s.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: BUG-328
change_type: bugfix
summary: Stop SetConsoleMode after bubbletea reader starts; detect startup input wedge
# --->8---
