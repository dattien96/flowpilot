# CA-719 — TUI drag bôi-đen copy restored dưới wheel-only mouse (?1000h)

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: BUG-345
change_type: bugfix
summary: restore in-app drag-select copy under wheel-only mouse (?1000h) — press→release over different cells arms the selection and auto-copies without motion events; wheel scroll, native-off mouse mode and BUG-328 hang-safety unchanged
# --->8---

## Problem

- Operator report: "không bôi đen text để copy được nữa" — regression từ chuỗi
  fix scroll (`5bfb4a5a`/`4a954f95`/`82f29baa`/`b65c46b2`).
- `82f29baa` bật `?1000h` (wheel-only, không 1002/1003). `?1000h` gửi
  press/release nhưng **không gửi motion** → `handlePlainLeftMouse` chỉ arm
  selection trên `MouseActionMotion`, nên release sau một drag thật bị coi là
  click: highlight bị xoá (`m.mouseSel = mouseSelect{}`), không copy.
- Đồng thời `?1000h` chặn native bôi-đen của host (CA-677 contract), nên cả
  hai đường copy đều chết.

## What changed (`mouse.go`, additive tests)

- `handlePlainLeftMouse` `case MouseActionRelease`: khi `mouseDrag.down &&
  !moved` mà release khác cell với press → coi là drag: arm
  `mouseSelect{x0,y0 → x1,y1}` và trả `autoCopySelectionOnDragEnd()` (cùng
  affordance CA-515: copy khi thả, selection giữ armed cho Ctrl+C fallback).
- Press+release cùng cell (click) vẫn xoá highlight, không copy (hành vi cũ).
- Không đổi `tuiProgramOpts`, `console_input*.go`, Windows hang path.

## R1 — old-suite regression evidence

- `TestDragRelease_*`, `TestShiftDragRelease_*`, `TestPlainDrag_*`,
  `TestShiftDrag_*`, `TestWheelOnly_*`, `TestDragSelectionTracksRenderedRows`
  PASS nguyên vẹn — không sửa test cũ nào.
- Full `go test ./internal/tui/... -count=1` → ok (app 7.0s, client 37.4s,
  config/prefs/runnerboot/desktopboot ok).

## R2 — provider parity

- Mouse handling là UI layer thuần (Bubble Tea `MouseMsg`), không có branch
  theo provider; hành vi copy như nhau trên Claude/Codex/Grok.

## R3 — new coverage (`bug345_wheel_drag_copy_test.go`)

- press→release khác cell, không motion → `CopiedMsg` + selection armed + span
  đúng (x1=40, y1=panelH).
- press+release cùng cell → không arm, không `CopiedMsg`.
- Shift+click không motion → copy cả dòng, `selectionPlainText` chứa đúng line.

## Honest gaps

- Chưa verify live trên terminal thật (macOS Terminal.app/iTerm) — nhưng
  `?1000h` press/release là wire format chuẩn xterm, test mô phỏng đúng shape.
- Native bôi-đen của host vẫn tắt khi TUI chạy (do `?1000h`); in-app
  highlight + auto-copy là đường copy chính, `/copy` fallback giữ nguyên.

## Prior CA not undone

- CA-677 (mouse-off opts) giữ — opts vẫn `AltScreen+Filter` 2 phần tử.
- CA-515 (drag-release copy) giữ — motion path vẫn hoạt động, chỉ thêm
  fallback không-motion.
- BUG-328 hang class không mở lại: không `WithMouseCellMotion`, không 1002/1003.