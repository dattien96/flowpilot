---
id: CA-491
feature_key: cli-tui
title: Tab slash completion keeps pre-slash draft
date: 2026-08-14
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-490-tui-skill-enter-preserve-prompt
will_not_undo: CA-490 Enter skill strip; CA-489 word-boundary /; skill Tab tick
```

## Change

**Bug (operator):** After CA-490, Enter on skills kept the draft — but **Tab** still
wiped it. Typing `abc /skill` then Tab ran `applySuggestion` for `kind=cmd` and set
`inputValue = "/skill "`, dropping `abc`. Same for any mid-draft command completion
(`draft /pro` → `/provider `).

**Fix:** `replaceActiveSlashWith` + `setInputPreservingDraftPrefix` — Tab/nested
picker fill only replaces the active word-boundary `/…` fragment; prefix draft stays.
Skill Tab tick path already left input alone; completion path now matches.

## Tests

- Update CA-489 `TestSlashAfterDraft_TabReplacesWithCommand` → `draft /provider `
- `TestSkillPicker_TabCompletesCommandKeepsDraft`, `TestReplaceActiveSlashWith`

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: TUI Tab slash completion preserves pre-slash draft text (skill repro)
# --->8---
