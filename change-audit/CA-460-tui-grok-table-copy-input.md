---
id: CA-460
feature_key: cli-tui
title: Grok-like boxed tables; copy glued to answer; rounded input stroke
date: 2026-08-13
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-459-tui-chat-gap-glamour-tokyo
will_not_undo: CA-459 prompt/answer gap; Tokyo Night headings; unboxed AI
```

## Change

- `[copy]` sits on the next row after the last answer line. Two-row padding is only inserted before the next chat bubble, not between answer and copy. Trailing blank markdown lines are trimmed.
- Markdown tables render as a full Grok-style box (`┌┬┐` / `│` / `└┴┘`) because stock Glamour hides outer borders.
- Chat input uses a rounded stroke frame (`╭ chat ─╮` / `╰ model ╯`) matching Grok CLI.

TUI chrome only. Claude/Codex/Grok adapters unchanged.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: Glue copy to the answer, box markdown tables like Grok CLI, and stroke the input with a rounded frame
# --->8---
