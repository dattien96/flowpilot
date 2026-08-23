# CA-575: Disable TAB picker for /mode-setup (modal replaces wizard)

## What

Tắt hoàn toàn flow TAB (posture → field → value) cho `/mode-setup`. Gõ `/mode-setup` hoặc `/mode-setup ` và bấm TAB không còn hiện picker wizard; chỉ Enter mới mở modal 3-tab.

## Why

Đã có modal khi Enter `/mode-setup` (CA-574). Giữ wizard TAB song song gây nhầm lẫn, user yêu cầu tắt flow TAB trên command này.

## Fix

- `app.go:collectSuggestions` — thêm guard đầu cho `/mode-setup`: nếu input là `/mode-setup` hoặc prefix `/mode-setup ` thì không trả về `filterModeSetupSuggestions` nữa, fall through để không có picker. Typed power-user `/mode-setup <p> <field> <value>` vẫn xử lý qua `handleSlashCommand`.
- `chat_posture_provider_fallback_test.go` — 4 tests chuyển từ `collectSuggestions` sang `filterModeSetupSuggestions` trực tiếp, đồng thời kiểm tra `collectSuggestions` cho `/mode-setup` không trả về `mode-setup-*`.
- `chat_posture_test.go:TestModeSetupPicker_PostureFieldValue` — đổi sang kiểm TAB disabled (`collectSuggestions` không có `mode-setup-*`) và kiểm filter vẫn hoạt động.
- `mode_setup_wizard_test.go` — 3 tests wizard cũ đổi sang kiểm TAB disabled và filter, đồng thời kiểm modal/power-user vẫn hoạt động; bỏ import `strings`/`tea` không dùng.

## Tests

- Full `go test ./internal/tui/app -count=1` pass (13.5s).
- Các test wizard cũ đã cập nhật: `TestModeSetupBareShowsPosture`, `TestModeSetupWizardStageAndSave`, `TestModeSetupWizardBack`, `TestModeSetupPicker_PostureFieldValue`, 4 provider fallback tests.
- Modal tests `modal_scan_additive_test.go` vẫn pass (TAB cho `/mode` và `/scan` không ảnh hưởng).

## Residual

- `filterModeSetupSuggestions` vẫn tồn tại cho typed power-user path.
- `handleKey` cho `mode-setup-save/back/value` vẫn còn nhưng không còn được trigger qua TAB.

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: CP-56
change_type: feature
summary: disable TAB picker for /mode-setup, modal on Enter only
# --->8---
