---
id: CA-458
feature_key: cli-tui
title: Click-outside clears highlight; history last-changed; chat/status pad; You box; Glamour markdown
date: 2026-08-13
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-457-tui-select-click-chat-boxes
will_not_undo: CA-454 Esc clears prompt; CA-456 Esc clears selection first; CA-457 boxed user chrome
```

## Change

- Any non-Shift left click (press, release, or Windows `Type=MouseLeft`) clears in-app highlight, then pulses `DisableMouse` → `EnableMouseCellMotion` so Windows Terminal native selection drops too.
- `/history` dump and picker show last-changed time (`updatedAt`, else `startedAt`).
- Transcript always has a blank line + horizontal rule before the status chrome (reserved `chatSepH=2`, not only unused pad).
- User bubble keeps the You frame; ask text is the query only (one-line rows keep `You:` as box chrome for the right-align contract). Assistant replies are unboxed.
- Assistant markdown uses Charm Glamour dark style (Grok CLI-style ANSI), with the previous custom parser as fallback.

TUI chrome only. Claude/Codex/Grok adapters unchanged. GitNexus MCP was unavailable.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: Clear highlight on click outside, pad chat above status, show history last-changed, unbox AI, and render markdown with Glamour
# --->8---
