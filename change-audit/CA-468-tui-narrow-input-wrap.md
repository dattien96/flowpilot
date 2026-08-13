---
id: CA-468
feature_key: cli-tui
title: Narrow-terminal input frame no longer wraps the right border
date: 2026-08-13
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-467-tui-flow-account-agent-stop
will_not_undo: CA-467 flow account/stop-all; CA-466 per-fence [copy]
```

## Change

Narrow Windows Terminal panes wrapped the full-width `chat` input frame: the right `│` fell onto the next row so `[+img]` looked stacked/repeating and the box lost its right edge.

- Draw the input frame at `width-1` so the last column stays free (Windows wrap).
- Truncate status/input by visual width, not raw rune count (ANSI no longer over-cuts).
- Stop laying out chat bubbles as 80 columns when the pane is under 20.

Separator rule stays full width (legacy tests require `Repeat("-", 80)`). TUI chrome only.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: Keep TUI chat input frame inside the pane on narrow Windows terminals
# --->8---
