# CA-600: You prompt box is full chat-pane width, left aligned (drop 7/10 hug + right-align)

## What

Operator changed the requirement: the `You` prompt box must be **full chat-pane width and left aligned** — always, with F2 sidebar on or off. Previously the box was only full-pane when the F2 sidebar was on (CA-594); without the sidebar it hugged content at 7/10 width and right-aligned (`rightAlignPlain`), which made the box look narrow and the `[copy]` chip not flush to the chat pane's right edge.

## Why

- `buildChatRows` set `rightAlign=true` for every `user` message, so `contentWidth = width*7/10`; `strokeChatRows` only went full-pane when `useRightSidebar()`. The layout looked broken when F2 was off.
- Requirement change supersedes `TestRenderMessages_UserAlignedRight` / `TestRegression_YouBoxRightAlignedWhenSidebarOff` / `TestRenderMessages_UserPromptHugsContent` — user approved updating those tests.

## Fix

- `apps/local-runner/internal/tui/app/chat_box.go` — `strokeChatRows(inner, width, ascii)`: removed the `user`/`alignRight` params; box is always `boxW = width` (the safeTermWidth'd chat pane), left aligned, no `rightAlignPlain`, no 7/10 cap. Inner rows keep their styled text (mention/file highlights, CA-558) but are padded with plain spaces after the styled span, so the style never bleeds across the box (CA-598) and every row's right border sits at exactly `innerW` (CA-599 alignment). Re-wrap still never truncates. Deleted `hugBoxWidth`, `userBoxGutter`, `strokeLine`.
- `apps/local-runner/internal/tui/app/app.go` — removed `rightAlign`/`isFullPaneBox`; `contentWidth = width - chip - 3` (left border + right border + leading space) so `wrapText` matches the box inner text width exactly (the old `-4` wrapped one column early, e.g. `…khi b >` / `a`).
- F2 geometry (`composeCellBuf`, `renderGrid`, `safeTermWidth` gutter) unchanged — the full-width box still leaves the last column free and never touches the sidebar separator.

## Tests

- Updated old guards (user-approved requirement change): `chat_layout_test.go` `TestRenderMessages_UserFullWidthLeftAligned`, `you_box_left_when_sidebar_test.go` `TestRegression_YouBoxFullWidthWhenSidebarOff`, `chat_prompt_codebox_test.go` `TestRenderMessages_UserPromptFullWidth`, `change_contract_prompt_box_test.go` word-level wrap assertions.
- New `apps/local-runner/internal/tui/app/you_box_full_width_test.go` — exact screenshot prompt `[Change Contract]`/`intent: … tra ve error khi b > a`/`symbols: Subtract` at 80 (F2 off) + 197/160 (F2 on) × claude/codex/grok: You top border starts at column 0, `lastBarCol ≥ width-2`, every `intent:`/`symbols:`/`[copy]` row bar column == top bar column.
- `go test ./internal/tui/app -count=1` 15s green; `go vet ./internal/tui/...` clean. (`internal/changecontract` failures are pre-existing at HEAD, unrelated.)

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: make You prompt box full chat-pane width and left aligned in all sidebar states, wrap exactly at box inner width
# --->8---