# CA-668 — BUG-328: remove Win32 input pump/drain; enable bracketed paste

## Evidence (tui.log 2026-08-28)

- pid 8788 (13:29–13:31, hybrid-pump build): Ctrl+V paste arrived with its
  head missing ("roidx.databinding.viewbinding)" without the 24-char prefix) —
  the ReadConsoleInput pump stole the first records from the ConPTY queue.
  After the final paste at 13:30:27: **zero KeyMsg for 45s+** while the event
  loop stayed alive (`sessionLoadTimeoutMsg`, watchdog tick). F2/F4/Esc/Ctrl+V
  never reached `Update` again.
- `win32 hybrid inject` log lines: **0** across every session that ran the
  pump (CA-665/666/667 builds). `win32 console drain` lines: **0**. The drain
  tick fired 3233 times and injected nothing.
- pid 15192 (16ms drain): F2×11 / F4×10 spam arrived via the VT pipe and the
  session never stalled — proving the pipe alone delivers function keys.
- pid 19296/14012 (native coninput reader, no pump): keys flowed whenever the
  pane had focus; every "stall" was an unfocused terminal, recovered by a
  click. No app-side Win32 machinery ever fixed anything.

## Root cause

Three input readers raced one ConPTY queue: Bubble Tea's VT pipe reader
(ReadFile + ENABLE_VIRTUAL_TERMINAL_INPUT), the background pump
(ReadConsoleInput), and the event-loop drain tick (ReadConsoleInput). The
pump/drain stole KEY_EVENT records the pipe would have delivered and, for
special keys, queued VT bytes into a buffer the reader only checks after
waking from `stdin.Read` — so stolen F2/F4/Esc/Ctrl+V were lost forever once
the pipe was empty. The 80ms `win32KeyDeb` filter dedupe also silently dropped
rapid repeated F2/Esc. The raw (non-bracketed) Ctrl+V flood was the trigger
that started the stealing episode: the app never enabled `?2004h`, so
Windows Terminal / Cursor delivered pastes as raw record floods.

## Fix

- **Delete** `windows_hybrid_input.go`, `windows_special_keys.go`,
  `windows_special_keys_poll.go` (+ their WIP tests). Input is one reader only:
  `conptyVTInput{os.Stdin}` (VT pipe, `readAnsiInputs`).
- **Enable bracketed paste** `\x1b[?2004h` in `primeConsoleBeforeProgram`
  (Cursor CLI parity) so Ctrl+V arrives as one `Paste=true` KeyMsg the app
  already handles — no raw flood, no reject-truncate, no steal window.
- `tuiMsgFilter` keeps only the ConPTY ctrl+h→Backspace remap; the debounce
  dedupe is gone.
- F2/F4 toggle notices compose onto the base state label ("ready · F4: details
  hidden") instead of replacing it — fixes pre-existing red
  `TestStatusBar_F4CollapsesDetailsKeepsLine0` (broken by c121324) with a
  production change, no test edit.

## Verification

- `go test ./internal/tui/app/ -count=1` green on Windows (whole suite).
- `GOOS=linux go build ./internal/tui/app/` green.
- New additive tests: `bug328_clean_vt_input_test.go` (rapid F2/Esc pass the
  filter; `?2004h` primed at startup).
- Binary rebuilt: `apps/local-runner go build -o flowpilot.exe ./cmd/flowpilot`.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: BUG-328
change_type: bugfix
summary: Remove Win32 input pump/drain that stole ConPTY keys; enable bracketed paste
# --->8---