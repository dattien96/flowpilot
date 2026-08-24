# CA-611: Vietnamese Telex IME not mistaken for paste flood

## What

Gõ tiếng Việt Telex `d-d -> đ` rồi thêm 1 ký tự nữa thì composer tự xóa và hiện toast `Use Alt+V for paste (text + image)`. Không có Ctrl+C.

## Why

Root cause: `rejectWindowsRawPaste` coi 2 ký tự nhanh trong 25ms là WT Ctrl+V flood.

- Windows IME Telex `dd -> đ` bắn: `d` -> `Backspace` (xóa preview) -> `d` -> `d` trong 3-5ms mỗi bước (log pid 4332: `backspace 07:43:56.641 -> d 07:43:56.645 (4ms) -> d 07:43:56.648 (3ms)`).
- `noteBurstRune` arm `pasteBurst.active` khi `chainLen >=2`, rồi `app.go:2303` `chainLen >=2` -> wipe 2 ký tự + hint Alt+V + `Copy` toast (thực ra không phải Ctrl+C, là `burst collapse`).
- Lặp lại ở 07:43:58, 07:44:03, 07:44:23. Không có `KeyCtrlC` nào trong log; `CopiedMsg {selection}` 07:44:30 là kéo chuột auto-copy.
- Comment "human typing >>25ms" sai với IME (0-5ms). Non-ASCII `đ` gần như không bao giờ tới app (IME chỉ gửi ASCII `d`+`d`).
- Giữ phím `s` cho tone cũng tạo `ssss` 12-16 ký tự nhanh -> tương tự.

## Fix

- `app.go:KeyBackspace`: `resetPasteBurst()` — IME rewrite phải cắt chain, không nối với `d` sau.
- `chat_paste.go`: thêm `isSingleRepeatedRuneChain()` — chuỗi 1 ký tự lặp (ssss) không phải paste.
- `app.go:handleKey` KeyRunes: ngưỡng reject `chainLen >=2` -> `>=pasteCollapseMinRunes (8)` và `!isSingleRepeatedRuneChain(chainBuf)`. IME 2-4 event không đủ dài; paste thật 20 ký tự varied vẫn reject.
- Hint revert: `chainLen ==2` -> `==pasteCollapseMinRunes` (revert về `pasteBurst.start`).

Không đụng: CA-610 mouse filter, Alt+V, CA-541 burst logic.

## Tests

- `tui_windows_paste_reject_test.go`: 2 tests sửa (cho phép):
  - `TestWindowsReject_RawFlood_BlockedAndHints`: payload `strings.Repeat("a",20)` -> `"abcdefghijklmnopqrst"` (varied, single-repeat exempt).
  - `TestWindowsReject_HijackNotWipedByTrailingFlood`: arm `"ab"` (2) -> `"abcdefgh"` (8) để đạt ngưỡng mới.
- `ca611_ime_not_paste_flood_test.go` (mới, additive):
  - `TestIME_TelexDd_NotRejected` — replay `d -> Backspace -> d -> d -> a` 3-5ms: không hint, không wipe, `a` còn.
  - `TestIME_BackspaceResetsBurst` — Backspace cắt chain.
  - `TestIME_SingleRepeatedChain_NotRejected` — 12x `s` không hint, giữ `s`.
  - `TestWindowsReject_LongVariedPasteStillRejected` — 20 varied vẫn reject + hint.
  - `TestIME_ShortVaried_NotRejectedBeforeThreshold` — 4 ký tự không hint.
  - `TestIsSingleRepeatedRuneChain` — helper.

`go test ./internal/tui/app -count=1` xanh (19s). `go vet ./internal/tui/app` clean.

## Provider parity

Provider-agnostic (paste/IME không đọc providerKey).

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CA-611
change_type: bugfix
summary: fix Vietnamese Telex IME false-positive as paste flood by resetting burst on Backspace, raising reject threshold to 8 and exempting single-repeated chains
# --->8---
