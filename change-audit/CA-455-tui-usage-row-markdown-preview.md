---
id: CA-455
feature_key: cli-tui
title: TUI token/context usage row + Desktop markdown preview
date: 2026-08-13
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-454-tui-markdown-scroll-esc
will_not_undo: CA-448 remaining chip first; CA-453 remain % format; CA-454 Esc/scroll/newline
```

## Token / context not visible

`formatContextLimits` was on status line 1. Width trim drops second-to-last chips, so `ctx … 128.0k left` disappeared behind provider/YOLO. Desktop ChatInput has a dedicated usage line.

Usage is now its own status row (after the project line): window remain %, used/window/left, last turn, in/out. Catalog window still shows before the first usage event (`ctx 128.0k window`).

## Markdown still raw

Neither TUI nor Desktop has a markdown library. Desktop `Timeline.tsx` is a custom subset (headings, fences, HR, tables, lists, inline bold/italic/code). TUI had a weaker line-by-line pass that still leaked markers and skipped tables.

TUI now ports that Desktop parser. No glamour (optional in CP-56, not in go.mod). Headings lose `#`, fences render as `│` code lines, lists as bullets/numbers.

Provider-agnostic TUI chrome. Claude/Codex/Grok adapters unchanged.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: Pin token/context usage on its own TUI row and port Desktop Timeline markdown preview
# --->8---
