---
id: CA-459
feature_key: cli-tui
title: Pad prompt/answer gaps; Grok-like Glamour (Tokyo Night, no hash headings)
date: 2026-08-13
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-458-tui-click-outside-chat-pad-glamour
will_not_undo: CA-458 click-outside, history last-changed, status rule, You box / unboxed AI
```

## Change

- Two blank rows between consecutive chat bubbles (prompt → answer, answer → next prompt).
- Assistant markdown uses Tokyo Night Glamour (Grok CLI palette): headings are color/bold without leftover `#` / `##`, paragraph spacing, inline-code background, chroma code blocks. `**` inside code no longer forces the custom fallback. Copy chip sits on its own row so wrap width is not stolen.

TUI chrome only. Claude/Codex/Grok adapters unchanged.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: Add prompt/answer padding and render assistant markdown with Grok-like Glamour Tokyo Night
# --->8---
