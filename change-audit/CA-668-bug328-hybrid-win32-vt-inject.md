# CA-668 — BUG-328: hybrid Win32 reader injects F2/F4/Esc VT bytes

## Evidence (tui.log pid 8788)

- Letters and remapped `backspace` flowed on the ConPTY VT pipe.
- **Zero** `f2`/`f4`/`esc` KeyMsg; pressing F4 only logged `runes "f"`.
- `win32 console drain` never fired — with `ENABLE_VIRTUAL_TERMINAL_INPUT`
  the event-loop poll saw an empty Win32 queue even when function keys did not
  arrive as CSI on stdin.

## Fix

- Replace the 16–100ms `win32ConsolePollMsg` tick with a background goroutine
  that **blocks on `ReadConsoleInput`**, discards duplicate letter KEY_EVENTs,
  and **injects xterm VT bytes** (`\x1bOQ` F2, `\x1bOS` F4, `\x1b` Esc) into
  the stdin reader ahead of the ConPTY pipe.
- Bubble Tea still parses injected bytes via `readAnsiInputs`; `tuiMsgFilter`
  debounce dedupes VT + Win32 double delivery.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: BUG-328
change_type: bugfix
summary: Inject F2/F4/Esc via blocking Win32 reader into ConPTY VT stdin
# --->8---
