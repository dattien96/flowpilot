# CA-615: Alt+V timeout so composer cannot hang (run-127174 10:06)

## What

Gõ nhanh 10:05 bình thường (dfg mash ~50-200ms, no clipboard I/O). `Alt+V` lúc 10:06 treo cả TUI: log dừng ở `10:05:40.610`, không có `KeyMsg alt+v` flush, ô chat không nhận phím.

## Why

- Alt+V **bắt buộc đọc clipboard hệ thống**. `cmdClipboardPasteWithFallback` gọi `readClipboardText()` → `clipboard.Read(FmtText)` native **không timeout**, rồi `readClipboardImageBytes()` → `clipboard.Read(FmtImage)` cũng không timeout. Trên Windows, `golang.design/x/clipboard` chạy trên STA và dễ deadlock khi clipboard bị app khác giữ (Word/browser/screenshot) hoặc clipboard lớn.
- CA-541 chỉ bọc timeout cho nhánh PowerShell `runClipboardPS` (3s). Nhánh native của Alt+V vẫn treo, và treo cả process Bubble Tea nên TUI không ghi log thêm. Gõ tay không đụng clipboard nên 10:05 vẫn ổn.
- Có ảnh trong clipboard chỉ nặng thêm khi text rỗng mới phải đọc ảnh (image branch cũng native không timeout), nhưng treo gốc là ở **text native** nếu clipboard bị lock.

## Fix

- `apps/local-runner/internal/tui/app/clipboard_paste.go`:
  - Thêm `clipboardNativeTimeout = 2s` và helper `readWithTimeout[T](d, fn) (T,bool)` (goroutine + select, test được).
  - `readClipboardText()`: Windows chỉ dùng `readWindowsClipboardTextPS` (PS 3s), **không gọi native** để tránh deadlock STA. Darwin/Linux giữ native nhưng bọc `readWithTimeout(clipboardNativeTimeout)` rồi fallback PS.
  - `readClipboardImageBytes()`: Windows **bỏ** `clipboard.Read(FmtImage)` native, chỉ PS `GetImage` + `GetFileDrop` (mỗi cái 3s). Darwin/Linux giữ native bọc timeout. Đã có text thì không gọi nhánh ảnh (giữ như cũ).
  - Tổng budget Alt+V ≤ ~6s (text PS 3s + image PS 3s khi cần), hết giờ trả `ClipboardPasteMsg.Err` toast `clipboard read timed out` thay vì treo.
- Không đụng `handleKey` gõ/Ctrl+V thô (vẫn không đọc clipboard), `cmdAttachImagePath`, CA-612 reject, CA-610 mouse filter.
- Provider-agnostic: không nhánh `ProviderKey` trên I/O clipboard; test matrix `claude/codex/grok` như `ca541_*`.

## Tests

- New `ca615_clipboard_read_timeout_test.go` (additive, no old edit):
  - `TestCA615_ReadWithTimeout_SlowFnTimesOut` — fn ngủ 500ms, timeout 50ms → `ok=false` <400ms
  - `TestCA615_ReadWithTimeout_FastFnOk` / `GenericBytesOk` — fn nhanh → ok
  - `TestCA615_CmdClipboardPaste_TextDoesNotNeedImage_ClaudeCodexGrok` — `cmdClipboardPasteWithFallback("fallback")` kết thúc <6s cho cả 3 provider (không treo)
  - `TestCA615_AltV_StillAsyncCmd` — Alt+V không insert `v`, vẫn trả `tea.Cmd` async cho 3 provider
- Old suite green: `go vet ./internal/tui/app`, `go test ./internal/tui/app -run TestCA615|TestCA541|TestBracketedPaste|TestAltV` pass, `go test ./internal/tui/app -count=1` 20s pass.

## Provider parity

Agnostic — `grep -R ProviderKey clipboard_paste.go` chỉ thấy `provider` cho `SupportsImages` check đã được matrix 3 provider trong test mới; clipboard I/O không nhánh provider.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: timeout native clipboard read on Alt+V so Windows TUI cannot hang (CA-615)
# --->8---
