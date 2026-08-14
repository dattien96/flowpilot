---
id: CA-484
feature_key: cli-tui
title: Hide empty-state [+img] chip on chat prompt
date: 2026-08-14
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-483-grok-image-path-fallback, CA-482-tui-altv-image-paste
will_not_undo: path fallback; Alt+V paste; attach still Alt+V / /image paste / click pending chip
```

## Issue

Chat input always painted `[+img]` even with zero pending attachments, which looked
like an image was already attached.

## Change

`inputAttachChipPlain()` returns empty when `pendingAttach` is empty; only
`[N img]` when N > 0. Layout/click math use the same helper. Attach remains
Alt+V, `/image paste`, or click on the pending count chip.

## Tests

Additive `input_attach_chip_test.go`. Layout/click tests updated for no permanent chip.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: Hide TUI [+img] chip unless pending images are attached
# --->8---
