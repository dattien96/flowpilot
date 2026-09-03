# BUG-348: Desktop posture setup modal bắt nhập text + cho chọn provider tay (lệch TUI)

## Metadata

- Document ID: `BUG-348`
- Title: `Desktop posture setup modal bắt nhập text + cho chọn provider tay (lệch TUI)`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-02`
- Last Updated: `2026-09-02`
- Feature Keys: `chat-history`
- Parent Documents: [CP-59: Chat SSOT](../../07-Coding-Plan/done/CP-59-Test-Steps.md)
- Child Documents: `none`
- Related Documents: [SD-26](../../06-System-Tech-Design/SD-26-Chat-Continuity-Ssot.md), [CA-724](../../../change-audit/CA-724-desktop-posture-model-picker.md)
- Replaces: `none`
- Tags: `chat-history, desktop, posture-setup, model-picker, tui-parity`

## AI Quick View

### Summary

- Desktop `ChatPostureSetupModal` có Provider `<select>` riêng + Model là text `<input>` bắt gõ tay — user phải biết model ID, dễ typo, provider/model lệch nhau (pin bare model không provider là case E7 phải derive).
- TUI `/mode-setup` không có provider picker: Model là picker preload từ `allModelsAcrossProviders` (registry + detected + current), chọn model auto-infer provider (`providerForModel`), `(inherit)` clear cả 3 field, reasoning gated theo catalog efforts của model (`reasoningOptionsForModel`, disabled khi chưa pick model).
- Fix: bỏ Provider select, Model thành `<select>` grouped theo provider (registry enabled + detected + current pin fallback), chọn model auto-set provider, hiện `Provider auto: <key>` read-only, reasoning options theo catalog + disabled khi chưa pick model.

### Current Ask

- Live (Desktop): mở gear Posture → Model dropdown liệt kê sẵn models theo provider; chọn model → provider tự set; reasoning chỉ enable sau khi pick model.

## Symptoms

- Operator E7: "UI desktop không ok — không cho chọn provider, provider luôn auto set khi chọn models; mở config modal cần load sẵn list model để bấm chọn như TUI chứ không bắt nhập text".

## Root Cause

- `ChatPostureSetupModal` (`apps/desktop-flowpilot/src/components/ChatPosturePanel.tsx`) implement form tự do (provider select + model text input), không follow TUI `mode_setup_modal.go` (`modalPickerOptions`/`applyModalPickerSelection`).

## Fix

- New pure module `src/components/postureModelPicker.ts`: `inferPostureProvider` (= TUI `providerForModel`), `applyPostureModelPick` (= `applyModalPickerSelection` model kind), `buildPostureModelGroups` (= `allModelsAcrossProviders`), `postureReasoningOptions` (= `reasoningOptionsForModel` + `isReasoningEnabled`), `DEFAULT_POSTURE_REASONING_OPTIONS` (= TUI `reasoningEffortOptions`, gồm `minimal` mà modal cũ thiếu).
- Modal: Model `<select>` + `<optgroup per provider>`; `Provider auto:` read-only; reasoning select theo catalog, disabled khi chưa pick model, giữ current value nếu ngoài list.
- Tests: `src/components/postureModelPicker.test.ts` 10/10 pass (standalone `node --test`; project `tsc` không thêm lỗi mới — lỗi duy nhất `store.chat-mode-persist.test.ts(82,58)` là pre-existing, verify bằng stash).

## Verification

- `node --test postureModelPicker.test.js`: 10 pass / 0 fail.
- Live retest E7 với UI mới: pending operator.
