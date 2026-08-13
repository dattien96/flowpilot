---
id: CA-457
feature_key: cli-tui
title: Click-outside clears selection; boxed code; chat/status split and You/AI stroke boxes
date: 2026-08-13
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-456-tui-question-answer-select
will_not_undo: CA-454 Esc clears prompt; CA-456 Esc clears selection first
```

## Change

- Shift-select clears on left **press or release outside** the highlighted range (including status/input). Click inside keeps it.
- Fenced code renders as a labeled box (`┌─ go ┐` / `│ code │` / `└─┘`), not `` ``` ``.
- Horizontal rule + padding separates the transcript from the status/input chrome.
- User and assistant turns sit in stroke boxes (one-line messages keep side bars only so `You:` right-align tests stay valid).

TUI chrome only. Claude/Codex/Grok adapters unchanged.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: Clear shift-select on click outside, box code blocks, and split chat from status with You/AI stroke frames
# --->8---
