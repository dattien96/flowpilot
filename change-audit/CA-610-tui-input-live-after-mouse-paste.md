# CA-610: TUI input stays live after mouse hover and paste (Windows conhost)

## What

TUI nhiều lúc không cho phép input (screenshot run-125458, P1). Composer hiện `chat` nhưng phím không vào. Gặp rất nhiều lần, không chỉ sau `/open`.

## Why

Root cause: Windows conhost `ReadConsoleInput` chung 1 hàng đợi 64 slot cho mouse + keys.

- `WithMouseCellMotion` + WT gửi `MouseMsg motion` cả khi không hold → mỗi motion `tuiLog` open/write/close + `View` 100-170ms (sidebar `composeCellBuf`) → queue đầy → KeyMsg drop ở OS.
- Sau raw paste WT, `pasteBurstSettleMsg` đã làm `Sequence(DisableMouse, EnableMouseCellMotion)` để "unstick". Trên thực tế nó drop next KeyMsg và brick phím 19-29 phút (log 18216/18700).
- Mỗi click `pulseMouseTracking()` lại `EnableMouseCellMotion`, kích thêm motion.

Log evidence (5 session thật):
- pid 18216: paste→Disable/Enable→im 29 phút chỉ có mouse, tới Esc mới gõ lại
- pid 18700: y hệt 19 phút tới `/`
- pid 13996 (screenshot): `/open` + motion flood + 3 click vào vùng trống → hết KeyMsg
- pid 22164 (không rê chuột): 173 KeyMsg, 0 freeze

Composer vẫn `chat` vì không có gate/sessionLoading lock — phím không tới `handleKey`.

## Fix

1. `app.go:tuiMsgFilter` + `tea.WithFilter`: drop hover motion (`MouseActionMotion && !mouseDrag.down && !Shift`) trước khi vào `Update`/`View`. Giữ drag-motion, click press/release, wheel, Shift+drag.
   `tuiProgramOpts` thêm `WithFilter` (AltScreen+MouseCellMotion+Filter).

2. `app.go:pasteBurstSettleMsg`: bỏ `Sequence(DisableMouse, EnableMouseCellMotion)` sau `burst collapse (windows reject)`. Chỉ reset burst, không pulse. Queue đã không đầy nhờ filter.

3. `mouse.go:pulseMouseTracking`: thành no-op `func() tea.Msg { return nil }` (giữ Batch shape cho test cũ). Không còn pulse mỗi click.

4. `app.go:Update`: không `tuiLog` cho `MouseMsg` (tránh open/write/close mỗi motion).

Không đụng: `rejectWindowsRawPaste` flood guard, Alt+V, `allowsKeyWhileViewingChild`, gate, sessionLoading.

## Tests

- `tui_program_opts_test.go`: `len(opts)` 2→3 (bắt buộc).
- `ca610_tui_input_live_test.go` (additive, claude/codex/grok matrix):
  - `TestTuiMsgFilter_DropsHoverMotion` — hover nil
  - `TestTuiMsgFilter_KeepsDragMotion` — drag còn
  - `TestTuiMsgFilter_KeepsClickAndWheel` — click/wheel còn
  - `TestTuiMsgFilter_ShiftDragMotionKept`
  - `TestPasteSettle_WindowsReject_NoMousePulse` — settle không pulse
  - `TestPulseMouseTracking_IsNoop` — no-op
  - `TestTuiProgramOpts_IncludesFilter` — 3 opts
  - `TestTypingAfterOpenAndMotion_Inserts` — gõ sau ChatOpened+motion vẫn insert

Legacy `go test ./internal/tui/app -count=1` xanh (không sửa test cũ ngoài 1 dòng len).

## Provider parity

Provider-agnostic (filter/mouse/paste không đọc providerKey). Tests chạy filter+typing trên claude/codex/grok.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CA-610
change_type: bugfix
summary: keep TUI input live by filtering hover mouse motion before Update/View and never pulsing Disable/Enable mouse after paste or click (fixes Windows conhost 64-slot queue freeze)
# --->8---
