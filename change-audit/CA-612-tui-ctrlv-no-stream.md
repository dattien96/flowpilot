# CA-612: Ctrl+V streams into composer despite Alt+V hint

## What

Ctrl+V hiện toast `Use Alt+V for paste (text + image)` đúng, nhưng đoạn paste vẫn chảy từng ký tự vào ô chat (log pid 11764: `Them ham ClampChecked(n, lo, hi int) (int, error) vao format.go...`). Sau đó user backspace ~45 lần rồi Alt+V mới được.

## Why

Root cause: reject chỉ nuốt khi `chainLen >=8` **và** gap <25ms, và settle chỉ `resetPasteBurst()` không wipe. Với flood dài:

- 8 ký tự đầu `Them ham` (varied, 3ms gap) -> arm `rejectArmed` + wipe 8 + hint đúng.
- Mỗi ký tự sau vẫn `noteBurstRune` rồi mới check reject -> nhưng khi View chậm do per-rune `Tick(150ms)` storm, gap đo bằng `pasteNow()` lúc xử lý `handleKey` chứ không phải lúc arrival, nên gap phình 3ms -> 29ms -> 50ms -> 160ms (>=25ms).
- gap >=25ms -> `chainLen` reset về 1 -> `if active && chainLen>=8` fail -> tail `tra error khi lo > hi...` **insert** lại.
- settle ở `inputLen 31` rồi `40` chỉ `resetPasteBurst()` không xóa leaked tail -> stream ở lại.

Bằng chứng log 08:24:39: `T(39.532) -> h(39.535) -> e(39.537) -> m(39.540) -> space(39.544) -> ... -> m(39.552)` hint, sau đó `format.go:` vẫn tiếp tục `t(39.795) -> r...`, rồi `burst collapse (windows reject) inputLen=31` không wipe, tiếp `r(42.548)...` và settle 43.177 `inputLen=40`. Không có `KeyCtrlV` nào, chỉ `KeyRunes` raw flood (WT steal).

## Fix

- `chat_paste.go:pasteBurst`: thêm `rejectArmed bool` (giữ armed qua 25ms reset cho tới settle) và `settlePending bool` + `cmdPasteBurstSettleOnce()` dedup Tick (tránh storm làm View chậm).
- `app.go:KeyRunes/KeySpace`: sau `noteBurstRune`, nếu `rejectArmed` -> swallow mọi ký tự/space (+ `lastRuneAt=now`, `return settleOnce`) kể cả khi chain reset. Khi `chainLen>=8 && varied && !singleRepeat` lần đầu -> set `rejectArmed=true`, wipe về `start`, hint.
- `app.go:isBurstNewlineKey`: nếu `rejectArmed` thì swallow newline luôn (không insert \n).
- `app.go:KeyCtrlV` (windows+reject): arm `rejectArmed` ngay (phòng WT có gửi KeyCtrlV trước flood).
- `app.go:pasteBurstSettleMsg`: `settlePending=false` đầu, nếu `rejectArmed` thì wipe về `start` trước `resetPasteBurst()` (xóa 31/40 leaked), dedup early reschedule qua `Once`.

Không đụng: CA-611 ngưỡng 8 + `isSingleRepeatedRuneChain` + Backspace reset, CA-610 mouse filter.

## Tests

- `ca612_ctrlv_no_stream_test.go` (mới, additive):
  - `TestCA612_RejectStaysArmedAcrossGap` — 8 varied -> hint+empty, tail 4ms + 29ms + 50ms vẫn empty.
  - `TestCA612_SettleWipesWhenRejectArmed` — arm + injected leaked residue -> settle wipe về 0, sau đó gõ `h` được.
  - `TestCA612_KeyCtrlVArmsReject` — KeyCtrlV arm + runes sau bị nuốt.
  - `TestCA612_SettleDedup` — dedup tick.
  - `TestCA612_AltVAfterReject` — Alt+V vẫn paste sau reject.

`go test ./internal/tui/app -count=1` xanh (20s). `go vet ./internal/tui/app` clean. Không sửa test cũ.

## Provider parity

Provider-agnostic (paste/IME không đọc providerKey).

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CA-612
change_type: bugfix
summary: keep Windows Ctrl+V flood from streaming into composer by arming reject across gaps, deduping settle ticks and wiping leaked tail on settle
# --->8---
