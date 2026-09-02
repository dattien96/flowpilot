# BUG-346: TUI single-select question — Tab highlight lệch option so với giá trị Enter

## Metadata

- Document ID: `BUG-346`
- Title: `TUI single-select question — Tab highlight lệch option so với giá trị Enter`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-02`
- Last Updated: `2026-09-02`
- Feature Keys: `cli-tui`
- Parent Documents: [CP-56: Terminal TUI Chat And Flow Client](../../07-Coding-Plan/done/CP-56-Terminal-TUI-Chat-And-Flow-Client.md)
- Child Documents: `none`
- Related Documents: [BUG-333](./BUG-333-Approval-Gate-Tab-Ring-Dead.md), [CA-643](../../../change-audit/CA-643-question-render-part-colors.md), [CA-691](../../../change-audit/CA-691-tui-selected-sidebar-step-shared-fill-style.md)
- Replaces: `none`
- Tags: `cli-tui, question, action-ring, highlight, regression`

## AI Quick View

### Summary

- Form question-answer single-select render **2 chip mỗi option** (`"1)"` + `"Nam"`), nhưng action ring (`actionRingItems`) chỉ có **1 item mỗi option** (`qopt:i`). `actionRingIdx` 0..3 truyền thẳng vào `highlightIdx` → highlight đếm theo chip (0..7), lệch hẳn với giá trị Enter.
- Operator report (screenshot): 4 option thì Tab highlight lần lượt `1 - Nam - 2 - An`; lúc highlight "An" (label của option 2) mà Enter lại chọn **option 4** ("Other") vì `actionRingIdx=3` → `qopt:3`.
- Fix: gộp lại **1 chip mỗi option** (`"1) Nam"`), `highlightIdx == i` trùng khớp `actionRingItems` — highlight gì, Enter chọn đúng cái đó.

### Current Ask

- Tab highlight đúng option, Enter chọn đúng option đang được highlight.

### Key Decisions

- `D-1` Một chip `"<n>) <label>"` mỗi option, highlight toàn chip khi `highlightIdx == i`.
- `D-2` Giữ nguyên `actionRingItems` (4 item `qopt:0..3` cho 4 option) — chỉ sửa renderer cho khớp.
- `D-3` Multi-select không đổi (`[x] mark` + `[Submit]` path giữ nguyên). Squeeze/ellipsis giữ.

### Constraints

- additive-tests-only: không sửa test cũ (các `TestBug333UX_QuestionBar*` PASS nguyên vẹn).
- Không đổi mouse hit-test shape: `"1)"` và label vẫn nằm trong stripped line nên `hitQuestionChrome` giữ nguyên.

## 1. Issue Summary

`renderQuestionBar` (single-select) tách mỗi option thành 2 chip để giữ fill khi squeeze (BUG-333 follow-up), nhưng `chipIdx` đếm 2 lần mỗi option trong khi `actionRingIdx` đếm 1 lần → highlight và giá trị Enter lệch nhau.

## 2. Root Cause

`apps/local-runner/internal/tui/app/app.go` `renderQuestionBar`:

```go
hiNum := highlightIdx == chipIdx   // chipIdx += 1
hiLabel := highlightIdx == chipIdx // chipIdx += 1
```

4 option → 8 chip, `highlightIdx` 0..3 phủ `1)` `Nam` `2)` `An`; Enter dùng `actionRingIdx=3` → `qopt:3` = option 4. Highlight "An" nhưng chọn "Other".

## 3. Fix

Single-select: mỗi option render **1 chip** `strconv.Itoa(i+1) + ") " + label`, `highlightIdx == i`. Budget mới: `prefixW + hintW + 2*(n-1) + 2n`, label được `fit` vào `budget - width(num)`.

## 4. Tests

New file `bug346_question_chip_highlight_test.go` (additive):

- `TestQuestionBarUnifiedChip_HighlightFollowsOption` — idx 1 fill cả chip `2) An`, không fill chip khác, đúng 1 chip filled.
- `TestQuestionBarUnifiedChip_LastOptionFill` — idx 3 fill `4) Other` (khớp `qopt:3`).
- `TestQuestionBarUnifiedChip_LongLabelsKeepFillAndFit` — squeeze vẫn fit width, đúng 1 fill, hint sống.

## 5. Verification

- `go test ./internal/tui/app/ -count=1 -run 'TestQuestionBar|TestBug333UX_Question|TestBug333_Question'` → PASS.
- `go test ./internal/tui/... -count=1` → toàn bộ suite green (không sửa test cũ nào).