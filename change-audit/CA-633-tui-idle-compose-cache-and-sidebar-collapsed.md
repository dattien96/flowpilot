# CA-633: TUI hang residual — cold-start sidebar + per-tick compose rebuild

## What

TUI chat "treo" lặp lại sau CA-631/632 (log pid 24144, binary mới — `input guards: rejectWindowsRawPaste=true` đã chạy): `SessionDefaultsMsg` xong lúc +4s, sau đó **0 KeyMsg** đến khi timeout 45s và cửa sổ bị kill. Không có paste, không `View slow` — phím không bao giờ tới `Update`. Cùng pattern pid 16512.

## Why

1. `ConnectedMsg` (và cả zero-value `sessionPanel.Collapsed=false`) mở sidebar F2 ngay khi connect — `hasContent()` true vì RunnerURL vừa set, terminal 126×50 ≥ 100 → `useRightSidebar()` luôn true trên màn hình rộng.
2. Mỗi `cursorTickMsg` (530ms) `View()` chạy lại toàn bộ `composeCellBuf` 126×50 (cellbuf + renderGrid, 100–357ms). Mouse motion + phím share 1 queue 64-slot của conhost → queue đầy → phím drop ở OS.
3. Test cũ không bắt được: CA-610 inject KeyMsg thẳng vào `Update` (không qua queue OS); CA-621 đo View với `hasContent=false` (sidebar tắt) nên `composeCellBuf` không chạy. Hành vi "idle tick rebuild compose" chưa từng có coverage.

## Fix

- **`app.go` `ConnectedMsg`**: `sessionPanel.Collapsed = true` — cold start sidebar đóng; F2 vẫn mở/đóng, flow auto-open (`agents_focus.go`) vẫn bung khi có steps/agents.
- **`app.go` `cursorTickMsg`**: blink chỉ khi caret có nghĩa (`cursorBlinkRelevant()`: loading, auth, modal, child view, live turn, blocked, drive/restore, draft, selection); idle thì pin caret steady (không toggle).
- **`app.go` `View()`**: `composeCellBuf` là hàm thuần → cache theo (chatRaw, side, dims). Idle tick (caret steady → chatRaw byte-identical) bỏ qua merge nặng; state thay đổi → recompose. Không cache chatRaw/side — View luôn fresh (giữ contract test cũ mutate field rồi View).
- **`model.go`**: thêm `lastCompose*` + `composeOut` + `composeBuilds` (instrumentation).

Will not undo: CA-631 reject guard, CA-632 sidebar height-aware + blocked reason, CA-610/621 filter/throttle, F2 toggle, flow auto-open.

## Tests (additive, no old edit)

- `tui/app/ca633_idle_frame_cache_and_sidebar_test.go` (matrix claude/codex/grok):
  - `ConnectedMsg_StartsSidebarCollapsed` — cold start `!useRightSidebar()`; F2 vẫn toggle.
  - `KeyAfterSessionReady_Inserts` — connect → defaults → phím tới composer + View hiện `a`.
  - `IdleCursorTick_SkipsComposeCache` — sidebar ON: View compose 1×; pin caret 1×; các tick sau `composeBuilds` không tăng, frame bằng nhau.
  - `TypingRebuildsCompose` — gõ → recompose + hiển thị.
  - `LiveCursorStillRecomposes` — turnStream → blink → recompose mỗi tick.
  - `BlockedBar_StillRenderedAfterCompose` — park + gate → bar có `[Continue]` + reason sau recompose.
- Verify: `go vet ./internal/tui/app ./internal/tui/client` clean; `go test ./internal/tui/app -count=1` xanh 13.3s (old suite untouched); `go test ./internal/tui/client -count=1` xanh.

## Provider parity

Agnostic — không nhánh `providerKey`; matrix 3 provider trong test mới.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CA-633
change_type: bugfix
summary: TUI idle hang — cold start sidebar collapsed (F2/flow vẫn mở) + composeCellBuf pure-cache + caret blink chỉ khi live để idle tick không rebuild cellbuf 126×50 (logs 16512/24144: 0 KeyMsg)
# --->8---