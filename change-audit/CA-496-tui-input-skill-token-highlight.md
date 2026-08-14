---
id: CA-496
feature_key: cli-tui
title: Highlight [skill-name] tokens in chat input
date: 2026-08-14
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-495-tui-status-highlight-values
will_not_undo: CA-492 [name] insert; CA-494 multi-pick; status styleStatusHi
```

## Change

Chat input renders attached skill mentions with the same accent highlight as the
status bar (`styleStatusHi`):

```text
abc [coding] [review] def
     ^^^^^^^^ ^^^^^^^^   accent bold
```

Only tokens for **currently selected** skills are highlighted; other `[brackets]`
in the draft stay normal input color. Auth email/password lines are never styled.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: feature
summary: TUI input accents attached [skill-name] prompt tokens
# --->8---
