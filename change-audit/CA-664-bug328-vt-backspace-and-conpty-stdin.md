# CA-664 — BUG-328: Windows VT Backspace is ctrl+h; read ConPTY stdin not CONIN$

## Evidence (tui.log pid 2692)

Letters/digits reached `Update`. Backspace arrived as `KeyMsg Type=ctrl+h` (0x08),
never `backspace` (0x7F), so `handleKey` did not delete. F2/F4 produced **no
KeyMsg** — `WithInputTTY` reads `CONIN$`, which gets printable bytes but not
ConPTY function-key CSI.

## Fix

- Remap `KeyCtrlH` → `KeyBackspace` in `tuiMsgFilter` and `handleKey`.
- Windows input is wrapped stdin (no `Fd()`), so Bubble Tea uses `readAnsiInputs`
  on the ConPTY pipe. Stdin is put in raw VT mode before `NewProgram` (MakeRaw
  is skipped when input is not a `term.File`).

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: BUG-328
change_type: bugfix
summary: Map Windows VT ctrl+h to Backspace; read ConPTY stdin for F2/F4
# --->8---
