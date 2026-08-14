---
id: CA-489
feature_key: cli-tui
title: Slash command list opens mid-draft
date: 2026-08-14
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-488-tui-image-slash-picker
will_not_undo: CA-488 /image Tab picker + panel [open]/[x]; CA-487 attach panel
```

## Change

Slash suggestions required the **entire** input to start with `/`. Leftover
draft text forced the operator to backspace to column 0.

1. **Word-boundary `/`** (Desktop `findActiveSlash`): space/tab/newline or
   start of input. `hello /mo` opens the `/model` list.
2. **Typing `/` after letters** inserts ` /` so command mode starts without
   deleting the draft. Auth email/password and bulk paste are unchanged.
3. Tab/Enter still replace the line with the chosen command (draft dropped).
4. URL slashes (`https://…`) do not open the list.

Provider-agnostic TUI chrome. Claude/Codex/Grok adapters untouched.

## Tests

`slash_mid_input_test.go` — mid-draft `/`, Tab/Enter, `/image` + `/flow`
nested pickers, URL negative, loading, password, caret-at-0 fallback.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: TUI slash command list opens on mid-draft / without backspacing to start
# --->8---
