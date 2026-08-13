---
id: CA-446
feature_key: cli-tui
title: TUI skill picker Tab-tick/Enter-apply + statusline attached skills; drop duplicate [chat]
date: 2026-08-13
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-445-repair-incomplete-tui-bugfixes
will_not_undo: CA-445 shutdown/OwnsRunner; CA-443 /skill slash toggle; Grok runner ACP unchanged
```

## Summary

TUI-only UX: skill picker matches “tick then apply”, statusline shows attached skills, and chat mode no longer duplicates the input `chat` label. No runner change.

## Changes

- Chat statusline omits `[chat]` (input already shows `chat` / `next`). `[flow]` / `[step]` kept.
- Statusline chip `skills:N ▸` (collapsed). F3 expands names on extra status rows (max 4, then `+N more`).
- F2/F3 also toggle via left-click on the session panel and `skills:N` chip (`tea.WithMouseCellMotion`).
- `/skill` picker: Tab ticks/unticks in place; Enter closes picker and keeps the set. Slash `/skill <name>` still toggles with a chat line (CA-443).
- Grok mid-run `/model` still does not switch the live ACP session (Desktop has the same runner path). Out of scope.

## Tests added

- `internal/tui/app/skill_picker_status_test.go`

## Provider impact

Provider-agnostic TUI chrome (statusline + slash picker). Claude/Codex/Grok adapters untouched.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: TUI skill Tab-tick stays in place; F3 collapsible skill names on status
# --->8---
