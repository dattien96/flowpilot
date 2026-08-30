# CA-666 — BUG-328: replace Win32 background pump with console drain tick

## Evidence (tui.log pid 14988)

- Pump started before `Run()`; F2/F4 arrived via VT, pump never logged `vk=`.
- After 4 function keys, **zero KeyMsg for 48s** while `sessionLoadTimeoutMsg` still fired.

Root cause: ConPTY duplicates printable keys as Win32 `KEY_EVENT` records on the
console buffer while the VT pipe carries the real input. The background pump
**left letter duplicates unconsumed**, filling the 64-slot queue. `p.Send` from a
goroutine also races the VT reader on Bubble Tea's unbuffered `msgs` channel.

## Fix

- Remove background `p.Send` pump.
- `cmdWin32ConsolePoll` every 16ms on the event loop: **drain the entire Win32
  queue** (discard letter duplicates), inject the first debounced special key.
- `acceptIncomingSpecialKey` in `tuiMsgFilter` dedupes VT + Win32 double delivery.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: BUG-328
change_type: bugfix
summary: Drain Win32 console queue on event-loop tick instead of background pump
# --->8---
