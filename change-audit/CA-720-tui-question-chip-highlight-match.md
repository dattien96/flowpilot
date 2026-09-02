# CA-720 — TUI single-select question: highlight khớp option (1 chip/option)

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: BUG-346
change_type: bugfix
summary: single-select question bar renders ONE chip per option ("1) Label") so Tab highlightIdx matches actionRingItems (qopt:i) — the old number+label split painted "2) An" at idx 3 but Enter chose option 4; squeeze/ellipsis and multi-select unchanged
# --->8---

## Problem

- Operator report (screenshot): form question-answer 4 option, Tab highlight
  lần lượt `1 - Nam - 2 - An` — highlight "An" (label option 2) nhưng Enter
  gửi `qopt:3` = option 4 ("Other").
- `renderQuestionBar` (single-select) cố ý tách **2 chip mỗi option**
  (`"1)"` + `"Nam"`) từ BUG-333 follow-up (giữ fill khi squeeze), nhưng
  `chipIdx` đếm 2 lần/option trong khi `actionRingItems` chỉ có 1 item/option
  (`qopt:i`) và `ringHighlightFor("question")` trả `actionRingIdx` 0..3 trực
  tiếp — highlight và giá trị Enter lệch nhau.

## What changed (`app.go` `renderQuestionBar`, additive tests)

- Single-select: mỗi option render **1 chip** `"<n>) <label>"`,
  `highlightIdx == i` trùng khớp `actionRingItems`. Enter trên chip highlight
  luôn chọn đúng option đang sáng.
- Budget squeeze mới: `prefixW + hintW + 2*(n-1) + 2n`, label `fit` vào
  `budget - width("n) ")` — vẫn fit width, fill và hint sống (BUG-333 contract).
- Multi-select (`[x] mark` + `[Submit]`) không đổi.
- Mouse hit-test giữ nguyên: `"n)"` và label vẫn nằm trong stripped line nên
  `hitQuestionChrome` nhận diện `qopt:i` như cũ.

## R1 — old-suite regression evidence

- `TestBug333UX_QuestionBarKeepsFillWhenLong`, `...ShortKeepsFullLabels`,
  `...MultiSelectKeepsSubmitAndFill`, `TestBug333_QuestionChipFillFollowsSelection`,
  `TestQuestionBarIsClickable` PASS nguyên vẹn — không sửa test cũ nào.
- Full `go test ./internal/tui/... -count=1` → ok.

## R2 — provider parity

- Renderer là UI layer thuần, không branch theo provider.

## R3 — new coverage (`bug346_question_chip_highlight_test.go`)

- idx 1 → fill cả chip `2) An` (không phải "Nam"), đúng 1 chip filled.
- idx 3 → fill `4) Other` (khớp `qopt:3`).
- 4 idx × label dài → bar fit 100, đúng 1 fill mỗi idx, hint sống.

## Honest gaps

- Chưa verify live trên terminal thật; test dựa trên `stripANSI` + fill
  `48;5;62` giống legacy suite.

## Prior CA not undone

- CA-643 (part-color question render) giữ — chip renderer `renderActionRingChip`
  không đổi.
- CA-691/shared filled-chip style giữ — `styleRingSelected` không đổi.
- BUG-333 (fill survive squeeze) giữ — squeeze path vẫn giữ fill, chỉ gộp chip.