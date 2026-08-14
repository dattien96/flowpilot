---
id: CA-487
feature_key: cli-tui
title: TUI pending-image panel with detach (Desktop chip parity)
date: 2026-08-14
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-484-tui-hide-empty-img-chip, CA-483-grok-image-path-fallback
will_not_undo: empty chip hidden; Alt+V paste; Grok path-fallback on send
```

## Issue

Once images were attached in the TUI there was no detach UI (only `/image clear`).
Desktop shows chips with × remove.

## Change

- Click **`[N img]`** opens a manage panel (list + **`[x]`** per row).
- Click **`[x]`** removes that pending attachment and deletes its temp file under
  `%TEMP%/flowpilot-tui-pending/` (materialized on attach).
- `/image rm <n>`, `/image clear`, Esc closes panel.
- Send/clear/new also release pending temp files.

## Tests

Additive `attach_panel_test.go`.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: feature
summary: TUI click [N img] panel to detach pending images and delete temp files
# --->8---
