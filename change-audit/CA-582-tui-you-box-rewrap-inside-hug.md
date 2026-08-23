# CA-582: Re-wrap You box inner lines inside hug width to prevent mid-box truncate

## What

Long user prompts with pipes/paths were still cut mid-box after the initial wrap+rightAlign fix. `strokeChatRows` now re-wraps any inner line that exceeds the final `innerW` (after hug) into multiple box rows, instead of truncating with `…`. This fixes the screenshot where `|Path:` was lost and the box appeared as `tra ve| |Runner:`.

## Why

- Initial wrap used `contentWidth` (chatWidth*0.7) then `hugBoxWidth` capped to same 70%, but `innerW` could still be slightly smaller than a wrapped line (due to title/`+1` and `copy` chip). The old `padVisualANSI`+`truncateVisual` then cut the line mid-box (e.g. `|Path:` lost), and `joinRightSidebar`'s `truncateVisual(l, contentW)` cut the box border, making `││` look like sidebar bleeding into prompt.
- User reopened chat at 197×57 and still saw `intent: … tra ve error` + `symbols: Subtract [copy]` with `Runner:`/`Run:` on same visual row as the box border, i.e. the box was truncated, not wrapped.

## Fix

- `apps/local-runner/internal/tui/app/chat_box.go:219` — `strokeChatRows` after `hugBoxWidth` re-wraps any inner row whose plain width `> innerW-1` (minus leading space and copy chip) via `wrapText(plain, textW)`. Each wrapped piece becomes a new `chatRow` with same `MsgIdx`/`PromptExpandKey`; only the last piece of the last inner keeps `Copy`. Recompute `boxW`/`innerW` after re-wrap, then `strokeLine`+`rightAlignPlain` as before. Preserves `styleUser` for user rows.
- Keeps prior CA-579 fixes: `rightAlignPlain` truncate, `hugBoxWidth` visual, `helpers.go` visual wrap, `app.go` ellipsis reservation, `prompt_overflow_test.go` 17 subcases + `SidebarDoesNotCutPromptBox` 3 widths, all green.

## Tests

- `go test ./internal/tui/app -run TestRegression_UserPrompt -count=1` 2/2 pass (including pipe-heavy, no-spaces, Vietnamese, very-long at 60/80/100/120 and `SidebarDoesNotCutPromptBox` at 100/120/140).
- `go test ./internal/tui/app -count=1` 13.7s green.
- Debug `TestDebugView` at 197×57 now shows `│ intent: them ham SubtractWithGuard vao calc.go tra ve error│` + `│ symbols: Subtract [copy]│` correctly within `┌ You ─┐`/`└─┘`, sidebar `Runner:`/`Path:` on separate right column, no `travis` truncation.

## Residual

- Box remains hug-width, right-aligned, 4-line clamp + `....` tail; no change to Desktop or composer.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: re-wrap You box inner lines inside hug width to prevent mid-box truncate and sidebar bleed
# --->8---
