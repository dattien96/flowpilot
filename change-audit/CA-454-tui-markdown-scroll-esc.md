---
id: CA-454
feature_key: cli-tui
title: TUI markdown preview, transcript scroll, Windows Ctrl+Enter newline, Esc clears prompt
date: 2026-08-13
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-453-usage-remain-percent
will_not_undo: CA-451 chat UX; CA-445 OwnsRunner skip-kill; login Esc still cancels the wizard
```

## Bugs

1. Assistant markdown showed source markers (`#`, `**`, fences) because wrap used the plain line then restyled from the **unwrapped raw** line.
2. Long transcripts could not scroll: `viewport.offset` existed but `View()` always sliced the last N lines; wheel/PgUp/Up were ignored.
3. Ctrl+Enter still sent on Windows: the console maps Ctrl/Shift+Enter to plain `KeyEnter`.
4. Esc interrupted the run and quit the TUI instead of clearing the draft prompt.

## Change

- Render markdown by stripping markers first, then wrapping the display text. Lists and `[text](url)` are included.
- Transcript scroll: wheel, ↑/↓ (when no picker), PgUp/PgDown. Offset is lines from the bottom; 0 follows the live stream.
- Windows: `GetKeyState` for Ctrl/Shift when Enter arrives as `KeyEnter`. Plain Enter still sends.
- Esc with a draft clears the input. Empty Esc does not quit (Ctrl-C / `/exit` still do). Login wizard Esc is unchanged.

TUI chrome only. Claude/Codex/Grok adapters unchanged.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: Fix TUI markdown preview, transcript scrolling, Windows Ctrl+Enter newline, and Esc clearing the draft instead of quitting
# --->8---
