# CA-603: You box shows every prompt line (drop the 4-line clamp)

## What

Operator pasted the 5-line prompt `[Change Contract] / feature: calc-core / intent: … khi b > a / files: calc.go, calc_test.go / symbols: Subtract` and the box only showed a few lines. The old 4-line clamp (`maxUserPromptLines = 4` + `....` + click-expand) was still collapsing long prompts.

## Why

- `maxUserPromptLines` predates the full-width You box (CA-600/601/602). With the clamp gone there is nothing to collapse — the prompt is a plain text box.

## Fix

- `apps/local-runner/internal/tui/app/app.go` — boxed-user branch in `buildChatRows`: remove clamp/ellipsis/expand; `lines := trimEmptyEdges(wrapText(msg.Content, width-3))` → `youBox(lines, width, m.asciiMode, showCopy, mi)`. Removed `maxUserPromptLines`, `userPromptEllipsis`, `userPromptExpanded`, `toggleUserPrompt`, `chatRow.PromptExpandKey`, and the `expandedUserPrompts` hash from `chatRowsSig`.
- `apps/local-runner/internal/tui/app/chat_box.go` — `youBox` drops the `expandKey` param.
- `apps/local-runner/internal/tui/app/mouse.go` — removed the `user-prompt-expand:` target and `hitUserPromptChrome`.
- `apps/local-runner/internal/tui/app/model.go` — removed `expandedUserPrompts`.

## Tests

- Rewrote `tui_user_prompt_clamp_test.go` → no-clamp expectations: all lines visible, no `....`, no expand target, copy chip present.
- Updated `prompt_overflow_test.go` — `TestRegression_UserPromptShowsCopyWithoutOverflow` (no `....` requirement).
- New `TestRegression_YouBoxUserPastedPromptAllLines` in `you_box_lines_test.go` — the exact pasted 5-line prompt at 197/80 × claude/codex/grok: every line present in order, `[copy]` rides the `symbols:` row.
- `go test ./internal/tui/app -count=1` 14s green; `go vet ./internal/tui/...` clean. Manual dump at 197 shows all 5 lines with `[copy]` flush before the right `│`.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: drop the 4-line user-prompt clamp so the You box renders every prompt line
# --->8---