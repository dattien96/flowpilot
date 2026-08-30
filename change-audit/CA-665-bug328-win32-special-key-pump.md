# CA-665 — BUG-328: Win32 pump for F2/F4/Esc after ConPTY IME drops VT

## Evidence (tui.log pid 264)

- 13:10:09 F2 arrived twice (first presses worked).
- After Vietnamese typing + `burst collapse (windows reject)`, **zero** `f2`/`f4`/`esc`
  while letters and remapped `backspace` kept flowing.
- Ctrl+C still arrived (0x03 on the pipe). Esc/F-keys did not.

ConPTY keeps sending printable bytes + 0x08 on the stdin pipe, but after IME it
often delivers F2/F4/Esc only as Win32 `KEY_EVENT` on the console buffer, which
the VT `Read` path never sees.

## Fix

Background pump `ReadConsoleInput`, drains the buffer, and `Program.Send`s Esc,
F2–F4, arrows, Home/End/PgUp/PgDn/Delete. Same-key debounce 80ms so the first
VT CSI and the Win32 event do not double-toggle F2.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: BUG-328
change_type: bugfix
summary: Inject F2/F4/Esc from Win32 KEY_EVENT when ConPTY VT drops them
# --->8---
