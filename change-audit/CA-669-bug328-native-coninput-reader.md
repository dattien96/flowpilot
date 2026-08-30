# CA-669 — BUG-328: revert to native coninput reader; stamp stall fingerprint

## Evidence (tui.log pid 11948, 14:08:59–14:10:19)

- New CA-668 build (pure VT pipe, no pump/drain): IME commit arrived mangled
  as `KeyMsg runes="A,\u0091"` — the `localereader` (CP_ACP decode) + ConPTY
  record→VT translation corrupted the Vietnamese commit bytes.
- Letters/backspace kept flowing (14:09:21–14:09:30, including "windows
  reject" burst collapses on IME bursts), then **zero f2/f4/esc for 49s**
  while the event loop stayed alive (watchdog 14:10:19). ESC-prefixed VT
  sequences die after IME activity; plain ASCII does not.
- Earlier proof the native record path works: pid 19296/14012 (bubbletea
  default coninput reader, no WithInput wrapper) delivered mouse click + F2 +
  "/e" + enter reliably once the pane had focus.

## Root cause

The VT-pipe wrapper (CA-663/664, kept in CA-668) makes every key depend on
ConPTY's record→VT byte translation and decodes the pipe through
`go-localereader`. Both are lossy under Vietnamese IME: commits get mangled
("A,\u0091") and ESC-prefixed sequences (ESC, F2=ESC[12~, F4=ESC[14~) stop
being emitted/translated. The native coninput reader reads INPUT_RECORDs
directly — runes come from `Char`, function keys from `VirtualKeyCode` — no
VT bytes, no locale decoding, no translation step to lose sequences.

## Fix

- `tuiRunProgramOpts()` = `tuiProgramOpts()` only: no `WithInput` wrapper, so
  Bubble Tea uses its Windows `conInputReader`/`readConInputs` (the
  19296/14012 config). Deleted `windows_input.go` / `windows_input_other.go`.
- Watchdog stall fingerprint now stamps `pendingConsoleEvents=N`
  (`GetNumberOfConsoleInputEvents`) so the next stall is attributable:
  records piling up = app reader stopped consuming; N=0 = host stopped
  delivering keys.
- Kept from CA-668: bracketed paste `?2004h`, ctrl+h→Backspace remap
  (harmless on the record path: VK_BACK already maps), QuickEdit-off + flush
  before NewProgram, no SetConsoleMode after reader start.

## Verification

- `go test ./internal/tui/app/ -count=1` green (Windows), `GOOS=linux` build
  green.
- WIP test updated: `TestBug328_RunOptsDoNotWrapWindowsInput`.
- Binary rebuilt `flowpilot.exe` 14:19:05; stale instances killed.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: BUG-328
change_type: bugfix
summary: Revert to native coninput reader; watchdog stamps pending console event count
# --->8---