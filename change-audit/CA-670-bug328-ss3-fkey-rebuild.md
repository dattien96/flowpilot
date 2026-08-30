# CA-670 — BUG-328: rebuild SS3 F-keys mangled by ConPTY; drop bracketed paste

## Evidence (tui.log pid 18400, 14:20:16–14:21:36, native coninput reader)

- F2 pressed at 14:20:41 arrived as `KeyMsg runes "O"` then `"Q"`; F4 at
  14:20:45 as `"O"`,`"S"` — Cursor sends F2/F4 as vt100 SS3 `\x1bOQ`/`\x1bOS`,
  ConPTY's VT→record translation drops the ESC, and the record reader yields
  the suffix as plain runes typed into the composer (operator: "F2/F4 nhận
  nhầm thành input trong ô chat").
- The mangled pairs then tripped the raw-paste burst guard
  (`burst collapse (windows reject)`), and afterwards the host delivered
  nothing: watchdog fingerprint now shows **`pendingConsoleEvents=0`** — the
  console queue is empty, proving the wedge is host-side (Cursor stops
  delivering keys after the SS3/IME episode), not the app reader.
- Contrast: pid 19296 (Windows Terminal) delivered real `KeyF2` VK records —
  the mangling is Cursor-specific.

## Fix

- `ss3_fkeys.go`: hold a bare `'O'` rune up to 80ms and rebuild
  F1/F2/F3/F4 from `O`+`P/Q/R/S` before the rune reaches the composer or the
  burst guard. Non-suffix keys and expired windows flush the held `'O'` as
  text. Deterministic against the mangling on the record path.
- Reverted `?2004h` bracketed paste (CA-668): on the record path the 200~/201~
  markers arrive as plain records and would be typed as "[200~" garbage; the
  raw-flood guard already rejects Ctrl+V floods.
- Watchdog banner now tells the operator the queue is empty and clicking the
  terminal pane resets the terminal's key/IME state (the only proven
  host-side recovery, pid 19296/14012).

## Verification

- `go test ./internal/tui/app/ -count=1` green (Windows), `GOOS=linux` build
  green. New `bug328_ss3_fkeys_test.go` (pairs, flush, expiry, paste).
- Binary rebuilt 14:31:55; stale instances killed.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: BUG-328
change_type: bugfix
summary: Rebuild SS3 F-keys mangled by ConPTY; drop bracketed paste on record path
# --->8---