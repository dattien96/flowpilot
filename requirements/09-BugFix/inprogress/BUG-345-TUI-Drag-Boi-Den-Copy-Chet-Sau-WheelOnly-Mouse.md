# BUG-345: TUI drag bôi-đen copy chết sau khi bật wheel-only mouse (?1000h)

## Metadata

- Document ID: `BUG-345`
- Title: `TUI drag bôi-đen copy chết sau khi bật wheel-only mouse (?1000h)`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-02`
- Last Updated: `2026-09-02`
- Feature Keys: `cli-tui`
- Parent Documents: [CP-56: Terminal TUI Chat And Flow Client](../../07-Coding-Plan/done/CP-56-Terminal-TUI-Chat-And-Flow-Client.md)
- Child Documents: `none`
- Related Documents: [CA-677](../../../change-audit/CA-677-tui-revert-mouse-on-keep-bug328.md), [BUG-328](./BUG-328-TUI-Input-Dies-With-Zero-Key-Events-And-Cannot-Self-Recover.md), [CA-515](../../../change-audit/CA-515-tui-done-settle-drag-copy.md)
- Replaces: `none`
- Tags: `cli-tui, mouse, drag-copy, wheel-scroll, regression`

## AI Quick View

### Summary

- Commit `82f29baa` (wheel-only mouse `?1000h`) giữ được wheel scroll nhưng chặn native bôi-đen của terminal và đồng thời giết in-app drag-copy: `?1000h` chỉ gửi press/release, **không gửi motion**, nên `handlePlainLeftMouse` không bao giờ set `moved` — release luôn bị coi là click, highlight bị xoá, không copy.
- Operator report: "không bôi đen text để copy được nữa" sau chuỗi commit fix scroll (`5bfb4a5a` → `4a954f95` → `82f29baa` → `b65c46b2`).
- Fix: press→release trên hai cell khác nhau = drag-select + auto-copy (không cần motion). Giữ nguyên `?1000h`, wheel scroll và `tuiProgramOpts` không đổi, không mở lại hang class BUG-328.

### Current Ask

- Bôi đen được (in-app highlight) và copy được khi scroll chuột vẫn hoạt động.

### Key Decisions

- `D-1` Không tắt `?1000h` (giữ wheel scroll), không bật `WithMouseCellMotion` (không mở lại BUG-328 hang).
- `D-2` Release với tọa độ khác press (trong khi drag đang down) → arm `mouseSel` + `autoCopySelectionOnDragEnd` — cùng affordance CA-515 (copy khi thả chuột, selection giữ armed cho Ctrl+C fallback).
- `D-3` Press+release cùng cell (click) → không copy, xoá highlight (hành vi cũ giữ nguyên).

### Constraints

- additive-tests-only: không sửa test cũ (tất cả legacy drag/wheel tests PASS nguyên vẹn).
- Không đổi `tuiProgramOpts`, `console_input*.go`, Windows hang path, multi-select question.

## 1. Issue Summary

Sau khi bật `?1000h` (wheel-only) để scroll, drag bôi-đen không còn copy được. `?1000h` gửi press/release nhưng không gửi motion; code cũ chỉ arm selection trên `MouseActionMotion`.

## 2. Root Cause

`handlePlainLeftMouse` (`apps/local-runner/internal/tui/app/mouse.go`):

- Press: `mouseDrag = {down: true, x0, y0}`.
- Release: `dragged := m.mouseDrag.moved` — chỉ `MouseActionMotion` mới set `moved=true`. Với `?1000h` không có motion → `dragged=false` → `m.mouseSel = mouseSelect{}` (xoá highlight, không copy).

## 3. Fix

`case tea.MouseActionRelease` trong `handlePlainLeftMouse`: nếu `mouseDrag.down && !moved` mà `(msg.X,msg.Y) != (x0,y0)` thì coi là drag: arm `mouseSelect{x0,y0 → x1,y1}` và trả `autoCopySelectionOnDragEnd()`.

## 4. Tests

New file `bug345_wheel_drag_copy_test.go` (additive):

- `TestWheelOnly_PressReleaseDifferentCellCopiesWithoutMotion` — press(4,y)→release(40,y), không motion → `CopiedMsg`, selection armed, span đúng.
- `TestWheelOnly_SameCellClickDoesNotCopy` — press+release cùng cell → không arm, không copy.
- `TestWheelOnly_ShiftReleaseWithoutMotionCopiesLine` — Shift+click không motion vẫn copy cả dòng (CA-515 path giữ).

## 5. Verification

- `go test ./internal/tui/app/ -count=1 -run 'TestWheelOnly_|TestDrag|TestShiftDrag|TestPlainDrag'` → PASS.
- `go test ./internal/tui/... -count=1` → toàn bộ suite green (không sửa test cũ nào).