# Task-361: Android Gradle Build Fallback

## Metadata

- Document ID: `Task-361`
- Title: `Android Gradle Build Fallback`
- Phase: `task`
- Status: `done`
- Owner: `Claude Sonnet MAX`
- Reviewers: `Claude agent review`
- Created: `2026-09-15`
- Last Updated: `2026-09-15`
- Parent Documents: [CP-63 P-8](../../07-Coding-Plan/todo/CP-63-IDE-Grade-LSP-Runtime.md)
- Child Documents: `None`
- Related Documents: [SD-17](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md), [Task-358](../todo/Task-358-LSP-Post-Write-Diagnostics-Hook.md), [Task-360](../todo/Task-360-Kotlin-Language-Server-Integration.md)
- Replaces: `None`
- Tags: `lsp, gradle, android, kotlin, build-fallback, code-intelligence`
- Feature Keys: `lsp-runtime`

## AI Quick View

### Summary

- Slice P-8 của CP-63: `GradleFallbackValidator` bù gap của kotlin-language-server (R class, Compose generated types không được LSP diagnostics bao phủ).
- Trigger chính xác: platform `android` VÀ LSP diagnostics count == 0 → chạy `./gradlew compileDebugKotlin` (hoặc `assembleDebug --dry-run`) để validate sâu; parse compiler output thành `GradleError{file, line, message}`.
- Missing `gradlew` / timeout / non-android → graceful, không block.
- Đây là slice cuối cùng của CP-63 — đóng Kotlin/Android extension path.

### Current Ask

- Implement P-8 theo CP-63 §4: `gradle_fallback.go` + `gradle_fallback_test.go`. 7 test signatures phải xanh.

### Key Decisions

- `T-1` `ShouldRunGradleFallback(diagnostics []FileDiagnostic, platform string) bool` — true chỉ khi `platform == "android"` && `len(diagnostics) == 0` (LSP sạch nhưng có thể miss R class).
- `T-2` `RunGradleValidation(ctx, workspaceRoot) ([]GradleError, error)` — chạy `./gradlew compileDebugKotlin` (ưu tiên) với timeout 120s; parse stderr/stdout compiler output.
- `T-3` `GradleError` struct: `File`, `Line`, `Message` — parse pattern `path:line: error: message` (Kotlin compiler format) + `error: message` (unresolved).
- `T-4` Missing `gradlew` → return error rõ ràng, caller log warning, tiếp tục (không block Agent, không retry).
- `T-5` `FormatGradleErrorsForAgent(errors []GradleError) string` — format `app/src/.../MainActivity.kt:12: error: unresolved reference: R`.
- `T-6` Timeout/context cancel → trả timeout error, không giết turn — hook (Task-358) đã có cơ chế skip riêng; fallback này chạy sau LSP diagnostics pass.

### Constraints

- Additive tests only — không edit pre-existing tests.
- Chỉ chạy cho android platform — mọi platform khác không bao giờ trigger gradle.
- Không chạy full `assembleDebug` (mặc định) — `compileDebugKotlin` đủ nhanh cho validation; `--dry-run` chỉ khi config yêu cầu.
- Graceful degradation: mọi lỗi Gradle infra (missing gradlew, no android plugin, timeout) → log + continue, không block.
- GitNexus impact analysis trước mỗi symbol edit.

### Open Questions

- None.

### Source Refs

- CP-63 P-8 (§4), test signatures 1–7.
- Task-360 (kotlin-language-server known gaps — R class, Compose).

## 1. Goal

Android workspace với LSP diagnostics "0 errors" vẫn được xác nhận sâu bằng Gradle compile — bắt được R class / Compose generated errors mà kotlin-language-server bỏ sót, và báo chúng cho Agent dưới dạng structured errors.

## 2. Parent Links

- coding plan: `CP-63-IDE-Grade-LSP-Runtime.md` P-8
- tech design: `SD-17-Context-And-Regression-Engine.md`
- system spec: `SS-14-Code-Context-And-Regression-Safety.md`
- specific upstream ids: Task-358 (hook pipeline), Task-360 (kotlin config)

## 3. Trigger

kotlin-language-server (Task-360) không phát hiện được Android-specific errors (R class chưa generate, Compose compiler errors) — nếu chỉ dựa vào LSP diagnostics, Android Agent sẽ chạy test/build thất bại tốn token. Gradle fallback đóng lỗ hổng này trước khi chạy test.

## 4. Exact Change

- `T-1` **`internal/lsp/gradle_fallback.go`** (new): `GradleFallbackValidator` struct + `ShouldRunGradleFallback(diagnostics []FileDiagnostic, platform string) bool`.
- `T-2` **`internal/lsp/gradle_fallback.go`**: `RunGradleValidation(ctx context.Context, workspaceRoot string) ([]GradleError, error)` — `exec.CommandContext` với timeout 120s, `./gradlew compileDebugKotlin`, parse output.
- `T-3` **`internal/lsp/gradle_fallback.go`**: `FormatGradleErrorsForAgent(errors []GradleError) string`.
- `T-4` **`internal/lsp/gradle_fallback_test.go`** (new): 7 test signatures dưới đây — dùng fake gradlew script trả output mẫu.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/lsp/gradle_fallback.go` (new)
  - `apps/local-runner/internal/lsp/gradle_fallback_test.go` (new)
- modules: `lsp`
- routes: none
- tables: none

## 6. Acceptance Check

- [ ] AC-1: `ShouldRunGradleFallback` true khi platform android + 0 diagnostics.
- [ ] AC-2: False khi non-android (kể cả khi 0 diagnostics).
- [ ] AC-3: False khi có diagnostics (LSP bắt được lỗi rồi — không cần fallback).
- [ ] AC-4: `RunGradleValidation` parse compiler output thành `GradleError` list đúng (file, line, message).
- [ ] AC-5: Missing `gradlew` → error graceful (không panic, không block).
- [ ] AC-6: Timeout 120s → trả timeout error, không treo.
- [ ] AC-7: `FormatGradleErrorsForAgent` đúng format `file:line: error: message`.
- [ ] AC-8: 7 tests green:
  - `TestShouldRunGradleFallbackTrueForAndroidCleanDiagnostics`
  - `TestShouldRunGradleFallbackFalseForNonAndroid`
  - `TestShouldRunGradleFallbackFalseWhenDiagnosticsExist`
  - `TestRunGradleValidationParsesCompilerOutput`
  - `TestRunGradleValidationHandlesMissingGradlew`
  - `TestFormatGradleErrorsForAgentOutput`
  - `TestGradleFallbackTimesOut`

## 7. Out of Scope

- Full `assembleDebug` build pipeline (chỉ `compileDebugKotlin`).
- Gradle daemon lifecycle management.
- JetBrains Kotlin LSP migration (Q-1).
- Thay đổi hook pipeline (Task-358) — fallback gọi từ hook hoặc runner theo quyết định implement sau impact analysis.

## 8. Completion Notes

- result: Implemented as a library per CP-63 scope (no runner call-site — deferred follow-up below). Parses both `e: file:line:col msg` and `file:line: error: msg` shapes, skips warnings/task headers, repairs Windows drive prefixes. Non-zero exit with output = parsed result (not infrastructure error); missing wrapper/timeout = error. All 7 test signatures green. Cross-platform fake gradlew (.bat/sh) with batch-special-char escaping.
- follow-ups: call-site wiring DONE (2026-09-15, CA-871) — `CheckFiles` collects structured errors and runs the Gradle fallback when android LSP is clean (via new `AfterFileWriteErrors`); unavailable server counts as unknown, not clean.
- upstream docs updated: CP-63 P-8 marked done.