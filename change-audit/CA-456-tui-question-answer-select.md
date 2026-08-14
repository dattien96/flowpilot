---
id: CA-456
feature_key: cli-tui
title: TUI question answer POST + clickable Approve/Deny + clear shift selection
date: 2026-08-13
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-455-tui-usage-row-markdown-preview
will_not_undo: CA-451 approval /decision path; Esc clears prompt when no selection
```

## Hang after "Approved."

Tool `permission_required` POST `/decision` worked and showed `Approved.`. The next event was `user_question_required` (the demo git-commit gate with 1/2/3). TUI rendered `[QUESTION]` but never called `POST /client/questions/{id}/answer`. Typing approve/deny started a new turn instead. The original turn stayed `waiting_question` → hang.

## Change

- Route typed `approve`/`deny`/`/approve`/`/deny`/option numbers to `AnswerQuestion` while a question is pending. Do not clear until `QuestionResolvedMsg`.
- Input bar shows highlighted clickable **Approve** / **Deny** (and numbered question options).
- Shift+drag selects transcript rows; click outside or Esc clears that highlight (does not quit).

TUI chrome only. Claude/Codex/Grok adapters unchanged. Question/approval HTTP paths are provider-agnostic.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: POST question answers from TUI, clickable Approve/Deny, and clear shift-select on outside click
# --->8---
