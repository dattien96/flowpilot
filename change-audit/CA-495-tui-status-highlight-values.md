---
id: CA-495
feature_key: cli-tui
title: Status bar highlight model reasoning YOLO 7d skills
date: 2026-08-14
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-494-tui-skill-multi-pick-keep-open
will_not_undo: status layout F3/F4; skills chip click; formatAccountLimits plain helper
```

## Change

Bottom status model/account rows accent (bold + `colorAccent`) on:

| Segment | Highlight |
|---------|-----------|
| model id | entire value |
| reasoning | **value only** (`medium` / `high` …) — label `reasoning: ` stays dim |
| YOLO | **value only** (`ON` / `OFF` / `ON(auto)`) — `YOLO:` stays dim |
| `7d:NN%` | entire token (reset suffix dim) |
| `skills:N …` | entire chip |

Plain text via `stripANSI` unchanged for tests and mouse hit targets.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: feature
summary: TUI status bar accents model, reason value, YOLO value, 7d%, skills chip
# --->8---
