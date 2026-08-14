---
id: CA-465
feature_key: cli-tui
title: Hug You prompt box, drop assistant markdown box, code-fence panel
date: 2026-08-13
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-464-tui-codex-style-markdown
will_not_undo: CA-464 goldmark cache; CA-460 copy glued to answer; chatRows scroll cache
```

## Change

- User prompt is a complete `You` stroke box that hugs content (top/sides/bottom). No 70%-width hollow bar; no `You:` prefix on the inner line.
- One blank row only between real chat bubbles (user ↔ plain assistant). System/thinking/tool rows do not get the extra chat gap.
- Assistant replies stay styled goldmark with no outer `markdown`-labeled box. `[copy]` stays on the last answer line.
- Fenced code uses a labeled box (`go` / `code`) whose inner fill background is `#252526` so it reads against the terminal / `--bg-3`.

TUI chrome only (`internal/tui/app`). Screenshot follow-up to Task-289.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: Task-289
change_type: bugfix
summary: Hug the You prompt box, remove the assistant markdown box, and give code fences a distinct background
# --->8---
