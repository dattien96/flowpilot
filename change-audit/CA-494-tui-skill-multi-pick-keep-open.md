---
id: CA-494
feature_key: cli-tui
title: Skill Tab multi-pick keeps picker open
date: 2026-08-14
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-493-tui-skill-tab-closes-slash
will_not_undo: CA-492 [name] in prompt; chip inject; CA-490 Enter strip
```

## Change

Operator needs multi-select without retyping `/skill` each time.

| Key | Behavior |
|-----|----------|
| **Tab** | Tick/untick, insert/remove `[name]`, **keep** `/skill` open |
| **Enter** | Close picker (strip `/skill…`), keep draft + tokens + chip |
| **Esc** | Existing clear-input path (unchanged) |

Reverts CA-493 “close on Tab”. While picking: `abc [a] [b] /skill `. After Enter: `abc [a] [b] `.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: TUI skill Tab multi-picks with picker open; Enter closes list
# --->8---
