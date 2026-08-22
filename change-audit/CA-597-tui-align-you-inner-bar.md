# CA-597: Align You inner bar to top bar when F2 on

## What

Rebuilt image still showed `┌ You` wide but `│ intent: …tra` with `│` mid-screen and `Runner:` inside the gap. Top `┐` at `chatW-1`, inner `│` at ~60.

## Why

- `strokeChatRows` full-pane rebuilt inner rows as `│` + `styleUser.Render(base)` + `│`. Ghostty counted the `38;2;R;G;B` SGR with `:` differently than `lipgloss.Width`, so `padVisualANSI` left the inner at text width and the right `│` landed mid-pane. `stripANSI` + `lipgloss` on the same string stayed green, so the 1-column delta was invisible in tests.

## Fix

- `apps/local-runner/internal/tui/app/chat_box.go:321` — when `fullPane` (F2 on), build `base` plain padded to `innerW` (and `innerW-chip` when `[copy]`), then `line = "│" + base + "│"` without `styleUser` SGR. Top/bottom already plain, so `lastBoxGlyphCol` (`│`/`┐`/`┘`) is identical for every You row on Ghostty. Color loss on inner rows while F2 is on is the minimal visual tradeoff.
- Test `you_box_bar_align_test.go` asserts `lastBoxGlyphCol` of `intent:`/`[copy]` rows equals that of `┌ You` and `Runner:` never appears before `topCol` inside the box.

## Tests

- New `you_box_bar_align_test.go` 197/160 × claude/codex/grok: `intent:`/`[copy]` bar column == top `┐` column, `Runner:` mid-screen is rejected.
- `go test ./internal/tui/app -count=1` 13.7s pass.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: align You inner bar to top bar when F2 on so styled width does not pull sidebar into box
# --->8---
