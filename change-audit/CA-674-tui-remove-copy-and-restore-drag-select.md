# CA-674 — TUI: remove hint line, remove [copy] trailing, restore drag-select

## Context

User request (build mode, 3 items):
1. Xóa dòng chữ `F2/click session panel · F3/click skills chip` (hint line).
2. Xóa `[copy]` cuối mỗi prompt và answer.
3. Bug khi bôi đen thì đoạn đó không được auto copied và thiếu dòng thông báo `Copied` — code cũ có.

## Changes

1. **Hint line (`app.go:726`):** `Ready — type / for commands · F2/click session panel · F3/click skills chip.` → `Ready — type / for commands.` (remove F2/F3 hint).

2. **Remove [copy] trailing (`app.go:4870`, `chat_box.go:319`):**
   - `chat_box.go:youBox` – ignore `copyOn`, remove dedicated `[copy]` row (was `│...[copy]│`).
   - `app.go:buildChatRows` – `showCopy` trailing `styleLink.Render(copyChip)` removed; `row.Copy` now only for `copyFence` (per-fence code copy kept, trailing answer copy removed).
   - Tests updated to expect NO trailing `[copy]`: `chat_codex_markdown_test.go`, `chat_copy_table_test.go`, `chat_ux_actions_test.go`, `you_box_crlf_clamp_test.go`, `you_box_prompt_clamp_test.go`, `you_box_lines_test.go`, `prompt_overflow_test.go`, `tui_user_prompt_clamp_test.go`, `you_view_dump_test.go`.

3. **Restore drag-select auto-copy:**
   - `app.go:tuiProgramOpts` re-enabled `tea.WithMouseCellMotion()` (was disabled by BUG-328 to avoid Windows focus steal, but user explicitly wants drag-select back).
   - `autoCopySelectionOnDragEnd` already copies `selectionPlainText()` on `MouseActionRelease` and `CopiedMsg` handler shows `Copied selection.` toast via `flashToast` → `renderBottomNotice` (bottom line outside frame, CA-672).
   - Updated `tui_program_opts_test.go`, `bug328_program_opts_test.go`, `ca610_tui_input_live_test.go` to expect 3 opts (AltScreen+MouseCellMotion+Filter).

## Verification

- `go test ./internal/tui/app -count=1` green (7.3s).
- Drag tests: `TestDragRelease_CopiesSelectionAndShowsToast`, `TestShiftDragRelease_CopiesSelection`, `TestDragRelease_SelectionClearsAfterCopiedMsg` pass.
- Bottom notice tests: `TestBottomNotice_ShowsCopiedOutsideFrame` still shows `Copied selection` after `CopiedMsg`.
- Manual: View no longer contains `[copy]` on You/answer trailing rows; per-fence `[copy]` still on code fences; Ready line without F2/F3 hint.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: Task-311
change_type: bugfix
summary: Remove F2/F3 hint and trailing [copy], restore drag-select auto-copy with bottom toast
# --->8---
