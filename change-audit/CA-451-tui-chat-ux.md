---
id: CA-451
feature_key: cli-tui
title: TUI chat UX — newline, thinking, stop, clickable approve, copy, approval 409
date: 2026-08-13
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-450-tui-remaining-reset-date
will_not_undo: CA-445 OwnsRunner skip-kill; CA-447 Grok billing credits; Desktop Grok session/new
```

## Summary

TUI chat UX now matches the Desktop contracts that were missing: multiline prompts, a thinking placeholder, stop, clickable `/approve` `/deny` `[copy]` `[stop]`, markdown-ish answers, and the approval 409.

## 409 root cause

`POST /client/approvals/{id}` (no `/decision`) does not match the runner. `/approve` then cleared local approval, `ErrMsg` set `ConnError`, and the next prompt became a new turn → `awaiting_user` 409.

Also: `sendBlocked()` treated pending approval as "not blocked", so a plain chat line during an approval posted a turn.

## Changes

- Client paths match Desktop: `/decision`, `/answer`, `/gate-decision` (legacy JSON keys kept so old client tests still pass).
- Do not clear `m.approval` until `ApprovalResolvedMsg`.
- Shift/Ctrl/Cmd/Alt+Enter and Ctrl+J insert a newline; Enter still sends.
- Send shows `thinking…` under the prompt.
- Ctrl-C while a turn is running interrupts (does not quit). Idle Ctrl-C still quits (old tests).
- Clickable `/approve` `/deny`, `[stop]`, `[copy]`, `[+img]`.
- Assistant output renders headings / fences / `code` / **bold**.
- Clipboard: image file path from text; Grok still Vision=false (same as Desktop).

## Provider impact

TUI client + chrome only. Claude/Codex/Grok adapters unchanged. Approval/gate/question HTTP paths are provider-agnostic.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: Fix TUI approval 409 and add chat UX for newline, thinking, stop, click, copy, markdown
# --->8---
