# CA-667 — BUG-328: stop Win32 console poll from starving KeyMsg

## Evidence (tui.log pid 15192)

- After CA-666, `win32ConsolePollMsg` fired every **16ms** (~60/s).
- Each tick logged `Update win32ConsolePollMsg …` (open/write/close log file),
  matching the CA-621 stall pattern: event loop alive, **zero KeyMsg** while
  F4 spam wedged input.

## Fix

- Skip `win32ConsolePollMsg` in the high-frequency Update log switch (same as
  `cursorTickMsg`).
- Adaptive poll: **100ms** when the Win32 queue is empty, **16ms** while
  ConPTY is duplicating printable keys into the buffer.
- Peek queue depth once before draining; no per-tick struct dump.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: BUG-328
change_type: bugfix
summary: Throttle Win32 console poll logging and use adaptive idle/busy intervals
# --->8---
