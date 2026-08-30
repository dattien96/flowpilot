# CA-662 — BUG-328: startup "hang" was unfocused terminal, not a dead reader

## Evidence (tui.log pid 19296)

- 11:33:15 start, CA-661 Init, session ready 11:33:21.
- Event loop stayed alive (`sessionLoadTimeoutMsg` at 11:34:00).
- Zero `KeyMsg` until 11:34:57 — first input was `mouse click x=36 y=19 target=""`.
- F2 then arrived twice (11:35:00.279, 11:35:00.988): two toggles cancel out, so the
  panel looks unchanged.

## Fix

- F2/F4 write a visible status (`steps shown/hidden`, `details shown/hidden`).
- Watchdog recovery duration uses `inputExpectedSince` when `lastInputAt` is zero
  (pid 19296 logged `2562047h` because it subtracted Go's zero time).

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: BUG-328
change_type: bugfix
summary: Show F2/F4 toggle status; fix watchdog recovery duration at startup
# --->8---
