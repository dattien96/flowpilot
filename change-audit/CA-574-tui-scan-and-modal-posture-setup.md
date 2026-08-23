# CA-574: TUI /scan command và modal 3-tab cho posture setup

## What

1. Thêm `/scan` làm slash command riêng (như `/mode scan`). Khi gõ `/`, `/s`, `/sc` đều hiện `/scan`. Gõ `/com` không hiện `/scan` (prefix-only, đúng).
2. Thêm picker cho `/mode `: gõ `/mode ` hiện `scan | plan | code` để chọn, Tab/Enter áp dụng.
3. Thay wizard tmp-steps của `/mode-setup` bằng modal overlay 3-tab (Scan | Plan | Code). Mỗi tab hiện Model, Reasoning, YOLO; bấm Save hoặc Enter khi focus ở Save mới thực sự PUT một lần. Esc đóng modal, Esc trong picker đóng picker trước.

## Why

- `/scan` chưa có trong `knownSlashCommands` nên không hiện khi gõ `/` hay `/s` (user báo "không xuất hiện trong list khi tôi gõ /com").
- Wizard cũ (`/mode-setup <p> <field> <value>` + staged draft + back/save rows) nhiều bước tạm, không ổn định. User yêu cầu modal 3-tab, mỗi tab show options, save/enter mới save.
- Reasoning phụ thuộc model: chỉ khi model được chọn mới enable reasoning. YOLO độc lập.

## Fix

- `model.go:543` — thêm `{"/scan", "Switch to scan posture — read-only (like /mode scan)"}` vào `knownSlashCommands`.
- `app.go:collectSuggestions` — thêm nhánh `/mode ` picker (filter `postureOrder` theo query, kind `mode`, không hijack `/mode-setup`).
- `app.go:suggestionAcceptValue` + `applySuggestion` — thêm `case "mode"` để Tab/Enter chạy `/mode <name>`.
- `app.go:renderSuggestions` — thêm `case "mode"` → `postures:` header.
- `app.go:handleSlashCommand` — thêm `case "/scan"` → `apply:scan` (như `/mode scan`); sửa `case "/mode-setup"` để bare (`/mode-setup`) và single-posture (`/mode-setup scan`) mở modal (`pending modal:` / `modal:scan`), typed 3-arg vẫn giữ immediate PUT.
- `app.go:handleKey` — ưu tiên modal: nếu `modeSetupModalOpen` thì delegate sang `handleModeSetupModalKey`.
- `app.go:View` — khi modal mở, ẩn slash suggestions và render `renderModeSetupModal` phía trên input; `w` đã được tính trước nên dùng luôn.
- `model.go:320` — thêm fields `modeSetupModalOpen`, `Tab`, `Draft`, `Focus`, `PickerOpen/Kind/Idx`.
- `chat_posture.go:122` — thêm `case "modal:"` trong `chatPostureCmdFromPending` để `openModeSetupModal`.
- `mode_setup_modal.go` (new) — modal state, `openModeSetupModal`/`closeModeSetupModal`, `reasoningOptionsForModel` (per-model `SupportedReasoningEfforts`), `isReasoningEnabled` (model != ""), `handleModeSetupModalKey` (←→ đổi tab, 1/2/3, ↑↓ đổi field, Enter mở picker hoặc Save/Cancel, Esc đóng picker rồi modal, Tab/Shift-Tab skip disabled reasoning), `modalPickerOptions`/`applyModalPickerSelection` (model auto-pin provider via `providerForModel`, reasoning disable khi chưa có model, yolo on/off/inherit độc lập), `renderModeSetupModal` (lipgloss box, tab active/inactive, field focus, provider auto hint, picker list 6 dòng, Save/Cancel, legend).
- `chat_posture_test.go:164` — bare `/mode-setup` đổi từ `pending "show"` + `View` check sang `pending "modal:"` + `modeSetupModalOpen` (spec mới).
- `app_extended_test.go:213` + `app_test.go:147` — `/help` test: tăng `Height` qua `WindowSizeMsg` và kiểm thêm `/scan` trong help (do thêm `/scan` làm help dài hơn, cần viewport cao để không clip).

## Provider parity

- Scan là posture, không phụ thuộc provider — `/scan` áp dụng cho mọi provider qua `apply:scan`.
- Model → provider auto-pin đã được kiểm qua `providerForModel` cho cả 3 provider trong test `TestModalCrossProviderModelInference` (claude opus/sonnet, codex o3, grok-4.5). YOLO và reasoning đều test cho cả 3.
- Reasoning gating: chỉ enable khi model non-empty, áp dụng chung cho mọi provider; YOLO independent đã lock qua `TestModalYoloIndependentAcrossTabs`.

## Tests (additive, không sửa suite cũ trừ 1 test outdated)

- `modal_scan_additive_test.go` (new, 12 tests):
  - `TestScanSlashAppears` — `/`, `/s`, `/sc`, `/scan` hiện `/scan`; `/com` không hiện (prefix).
  - `TestScanCommandSwitchesToScanPosture` — `/scan` → `apply:scan`.
  - `TestScanCommandOnlyInChat` — ngoài chat báo lỗi.
  - `TestModePickerShowsPostures` + `TestModePickerTabAccept` — `/mode ` picker và Enter.
  - `TestModeSetupBareOpensModal` / `WithPosture` — pending modal và tab.
  - `TestModalTabsSwitch` — ←→, 1/2/3.
  - `TestModalReasoningGatedOnModel` — disabled khi chưa chọn model, YOLO vẫn mở, chọn model → reasoning enable, provider auto-pin, reasoning options per-model.
  - `TestModalYoloIndependentAcrossTabs` — yolo mỗi tab riêng, reasoning vẫn disabled nếu không có model.
  - `TestModalCrossProviderModelInference` — 4 models (opus/sonnet/o3/grok-4.5) → provider đúng.
  - `TestModalSaveAndCancel` — Save trả PUT + saving flag, Cancel discard, Esc picker trước rồi modal.
  - `TestModalViewShowsTabsAndFocus` — View chứa SCAN/PLAN/CODE/Model/Reasoning/YOLO/Save/Cancel.
- Đã sửa `TestSlashModeSetup_ShowListsPostures` (outdated `show` → `modal`) vì bare giờ mở modal, không còn summary.
- Đã sửa `TestA8_5...` và `TestA8e...` để tăng height và kiểm `/scan` (help dài hơn trước).

## Residual

- Typed `/mode-setup <p> <field> <value>` vẫn giữ immediate PUT cho power-user.
- Wizard staged draft (`modeSetupDraft`) vẫn tồn tại nhưng bare giờ mở modal; các test wizard cũ (`mode_setup_wizard_test.go`) vẫn pass vì chúng test `/mode-setup plan model ` picker, không phải bare.
- Desktop modal không đổi.

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: CP-56
change_type: feature
summary: add /scan slash and 3-tab modal for posture setup with model-gated reasoning and independent YOLO
# --->8---
