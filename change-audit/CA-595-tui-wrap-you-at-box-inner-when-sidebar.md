# CA-595: Wrap You prompt at box inner width when F2 sidebar is on

## What

You box with prompt `[Change Contract] / feature: calc-core / intent: them ham SubtractWithGuard vao calc.go tra ve error khi b > a` broke as `tra│` / `│ ve error` in the screenshot, while the box was full pane. The wrap was at the wrong width.

## Why

- `buildChatRows` set `rightAlign=true` for all `user`, so `contentWidth = width*7/10` (~70%) even when the box was later made full-pane (`hugBoxWidth` returns `maxW` when `F2` on, CA-594). `wrapText(msg.Content, contentWidth)` broke at 70% (`tra` / ` ve error`), but `strokeChatRows` drew the box at 100% (`width`), so the `│` border landed mid-word.
- AI/code boxes wrap at their rendered width, so they never showed the mismatch.

## Fix

- `apps/local-runner/internal/tui/app/app.go:4290` — detect `isFullPaneBox = boxed && m.useRightSidebar()`. When true, force `rightAlign=false` and re-derive `contentWidth = width -4 - copyChip` (full inner width) after the initial 7/10 block, so `wrapText` and the box use the same width.
- No change to `joinPanes`/`paintRow`/`stripANSI` (CA-592/593).

## Tests

- New additive `apps/local-runner/internal/tui/app/change_contract_prompt_box_test.go:10` — exact prompt text `tra ve error khi b > a` at 197 F2 on + 80 no-sidebar × claude/codex/grok: plain view contains all parts, no `tra│`, box border `┌`/`│`/`└` starts at chat pane, `Runner:` stays `>=chatW-1` when F2 on. Logs full `View` 197 for visual check.
- `go test ./internal/tui/app -count=1` 13.7s pass, `go vet` clean.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: wrap You prompt at box inner width when F2 sidebar is on so box and text do not split mid-word
# --->8---
