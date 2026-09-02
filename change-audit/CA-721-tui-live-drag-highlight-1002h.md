# CA-721 — TUI live drag bôi-đen highlight: enable ?1002h drag motion

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: BUG-345
change_type: bugfix
summary: enable 1002h drag-only motion alongside wheel 1000h so the in-app selection paints live while dragging (operator: "kéo tới đâu không thấy"); hover 1003h stays off — BUG-328 wedge class untouched
# --->8---

## Problem

- BUG-345 follow-up (operator live feedback): copy chạy rồi nhưng **không có
  highlight trong lúc kéo** — `?1000h` chỉ gửi press/release, selection chỉ
  được arm tại release, nên màn hình không vẽ gì khi con trỏ đang kéo.
- Code paint (`applyMouseSelection` + `highlightVisualColumns`, chạy mỗi View
  khi `mouseSel` non-empty) và motion handler (`handlePlainLeftMouse` motion
  case) đã có sẵn từ CA-674 era — thiếu đúng mỗi terminal gửi motion.

## What changed

- `console_input.go` `wheelMouseOnANSI`:
  `\x1b[?1002l\x1b[?1003l\x1b[?1000h\x1b[?1006h` →
  `\x1b[?1003l\x1b[?1000h\x1b[?1002h\x1b[?1006h`.
  - `1002h` = motion **chỉ khi button đang giữ** (drag) — highlight chạy live
    theo con trỏ; không gửi gì khi idle.
  - `1003h` (hover) vẫn OFF — đây là class wedge BUG-328.
  - `tuiMsgFilter` đã pass motion khi `mouseDrag.down` (hoặc `Shift`), nên
    không cần sửa filter; motion lúc không kéo bị drop.
- Comments đồng bộ: `console_input_windows.go` `maybeRearmConsoleInput`,
  `app.go` `Init()` log + `tuiProgramOpts` doc.
- Không đổi `tuiProgramOpts` (vẫn AltScreen+Filter, không CellMotion/1003).

## R1 — old-suite regression evidence

- `TestWheelOnly_*`, `TestDrag*`, `TestShiftDrag*`, `TestPlainDrag*`,
  `TestDragSelectionTracksRenderedRows` PASS nguyên vẹn — không sửa test cũ.
- Full `go test ./internal/tui/... -count=1` → ok (app 7.3s).

## R2 — provider parity

- ANSI + motion handler là UI layer thuần, không branch theo provider.

## R3 — new coverage (thêm vào `bug345_wheel_drag_copy_test.go`)

- `TestWheelMouseANSI_DragMotionOnHoverOff` — `wheelMouseOnANSI` chứa
  `?1000h` + `?1002h` + `?1003l`, không chứa `?1003h`.
- `TestWheelOnly_DragMotionPaintsLiveHighlight` — press → motion (release chưa
  xảy ra): `mouseSel.x1` update live, `applyMouseSelection` paint reverse-video
  (`\x1b[7m`) trên row bị kéo.

## Honest gaps

- Chưa verify live trên terminal thật (máy operator: macOS Terminal.app/iTerm).
  Nếu Windows conhost có dấu hiệu wedge khi drag dài, gating theo GOOS (giữ
  1000h-only trên Windows) là fallback đã có pattern từ CA-676.

## Prior CA not undone

- CA-719 (press→release copy) giữ — release path không đổi.
- CA-677/opts giữ — `tuiProgramOpts` vẫn 2 phần tử AltScreen+Filter.
- BUG-328 hang class không mở lại: 1003 hover vẫn tắt ở mọi nơi.