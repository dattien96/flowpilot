# Task-356: LSP Platform Registry and Auto-Detection

## Metadata

- Document ID: `Task-356`
- Title: `LSP Platform Registry and Auto-Detection`
- Phase: `task`
- Status: `done`
- Owner: `Claude Sonnet MAX`
- Reviewers: `Claude agent review`
- Created: `2026-09-15`
- Last Updated: `2026-09-15`
- Parent Documents: [CP-63 P-3](../../07-Coding-Plan/done/CP-63-IDE-Grade-LSP-Runtime.md)
- Child Documents: `None`
- Related Documents: [SD-17](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md), [Task-353](../done/Task-353-Standalone-TUI-Onboarding-And-Modals.md), [Task-354](../done/Task-354-LSP-JSONRPC-Client-Core.md), [Task-355](../done/Task-355-LSP-Server-Lifecycle-Manager.md)
- Replaces: `None`
- Tags: `lsp, platform-detection, project-wizard, gopls, vtsls, pyright, rust-analyzer, code-intelligence`
- Feature Keys: `lsp-runtime`

## AI Quick View

### Summary

- Slice P-3 của CP-63: map platform → LSP server binary qua `PlatformLSPConfig` registry, và `DetectAndResolve` tự động chọn đúng language server cho workspace.
- Refactor `detectProjectPlatform` trong `project_wizard.go:38` thành shared util `DetectPlatform` (package `internal/lsp`), giữ nguyên behavior để pre-existing test `standalone_onboarding_test.go` không đổi.
- `exec.LookPath` kiểm tra binary trong PATH — nếu thiếu → trả error để caller log warning và degrade gracefully (không LSP, fallback build command).
- 7 platform mapping: `golang→gopls`, `nextjs/reactjs/node→vtsls`, `python→pyright`, `rust→rust-analyzer`, `android→kotlin-language-server`.

### Current Ask

- Implement P-3 theo CP-63 §4: `platform_registry.go` + `platform_detect.go` + refactor `project_wizard.go` + `platform_registry_test.go`. 8 test signatures phải xanh.

### Key Decisions

- `T-1` `PlatformLSPConfig` gồm `Platform`, `Binary`, `Args`, `FileExtensions`, `InitializationOptions` — đủ thông tin cho ServerManager (Task-355) spawn đúng server.
- `T-2` `DetectAndResolve(workspaceRoot string) (PlatformLSPConfig, error)` — reuse `DetectPlatform`, lookup binary bằng `exec.LookPath`, binary thiếu → descriptive error (caller log warning, tiếp tục không LSP).
- `T-3` Refactor an toàn: `internal/lsp/platform_detect.go` expose `DetectPlatform(dir string) string`; `project_wizard.go` gọi qua util thay vì implement cục bộ — behavior identical (đọc thứ tự: go.mod → gradle → Cargo.toml → python → package.json next/react → node → general).
- `T-4` `DefaultRegistry()` trả configs cho 7 platform; `android` entry chỉ "draft" ở P-3 (binary `kotlin-language-server`), finalize chi tiết ở Task-360 (P-7).

### Constraints

- Additive tests only — pre-existing `TestDetectProjectPlatform*` trong `standalone_onboarding_test.go` phải giữ nguyên và xanh (behavior parity bắt buộc).
- Không xóa/đổi `wizardPlatformOptions` trong `project_wizard.go` (danh sách platform của TUI wizard giữ nguyên).
- GitNexus impact analysis trước mỗi symbol edit — đặc biệt `detectProjectPlatform` (callers: `openProjectWizard`, tests).
- LSP server binaries không được auto-install bởi code này — chỉ check PATH.

### Open Questions

- None.

### Source Refs

- CP-63 P-3 (§4), test signatures 1–8.
- `apps/local-runner/internal/tui/app/project_wizard.go:38` (`detectProjectPlatform`), `standalone_onboarding_test.go:36-77`.

## 1. Goal

Khi Runner biết workspaceRoot, `DetectAndResolve` trả về đúng `PlatformLSPConfig` (binary + args + extensions) hoặc error rõ ràng nếu binary không có trong PATH — mở đường cho P-4/P-5 khởi động đúng LSP server mà không cần config thủ công.

## 2. Parent Links

- coding plan: `CP-63-IDE-Grade-LSP-Runtime.md` P-3
- tech design: `SD-17-Context-And-Regression-Engine.md`
- system spec: `SS-14-Code-Context-And-Regression-Safety.md`
- specific upstream ids: Task-353 (project wizard, detectProjectPlatform), Task-355 (ServerManager tiêu thụ config)

## 3. Trigger

Client (Task-354) và ServerManager (Task-355) đã sẵn sàng nhưng chưa biết workspace này là platform gì và dùng binary nào. Manual config từng project không chấp nhận được — cần auto-detect tái sử dụng logic wizard hiện có.

## 4. Exact Change

- `T-1` **`internal/lsp/platform_detect.go`** (new): `DetectPlatform(dir string) string` — copy logic từ `detectProjectPlatform` (project_wizard.go:38-81), trả về `golang|android|rust|python|nextjs|reactjs|node|general`.
- `T-2` **`internal/lsp/platform_registry.go`** (new): `PlatformLSPConfig` struct + `Registry` map + `DefaultRegistry()` — mappings: `golang→gopls`, `nextjs/reactjs/node→vtsls`, `python→pyright`, `rust→rust-analyzer`, `android→kotlin-language-server` (args cơ bản; kotlin args finalize ở Task-360).
- `T-3` **`internal/lsp/platform_registry.go`**: `DetectAndResolve(workspaceRoot string) (PlatformLSPConfig, error)` — `DetectPlatform` → lookup registry → `exec.LookPath(binary)` → error nếu thiếu.
- `T-4` **`internal/tui/app/project_wizard.go`** (refactor): `detectProjectPlatform` delegate tới `lsp.DetectPlatform` (hoặc alias) — behavior giữ nguyên, pre-existing tests xanh.
- `T-5` **`internal/lsp/platform_registry_test.go`** (new): 8 test signatures dưới đây.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/lsp/platform_registry.go` (new)
  - `apps/local-runner/internal/lsp/platform_detect.go` (new)
  - `apps/local-runner/internal/lsp/platform_registry_test.go` (new)
  - `apps/local-runner/internal/tui/app/project_wizard.go` (refactor: extract `DetectPlatform`)
- modules: `lsp` (new), `tui/app` (minor refactor)
- routes: none
- tables: none

## 6. Acceptance Check

- [ ] AC-1: `DefaultRegistry` map đủ 7 platform → binary đúng.
- [ ] AC-2: `DetectAndResolve` tìm binary trong PATH hoặc trả descriptive error khi thiếu.
- [ ] AC-3: `DetectPlatform` cho kết quả giống hệt `detectProjectPlatform` cũ (parity test trên cùng fixture dirs).
- [ ] AC-4: Pre-existing `TestDetectProjectPlatform*` trong `standalone_onboarding_test.go` vẫn xanh (không edit).
- [ ] AC-5: `FileExtensions` đúng per platform (`.go`, `.ts/.tsx/.js`, `.py`, `.rs`, `.kt/.kts`).
- [ ] AC-6: 8 tests green:
  - `TestDefaultRegistryContainsAllPlatforms`
  - `TestDetectAndResolveFindsGoplsForGolang`
  - `TestDetectAndResolveFindsVtslsForNextjs`
  - `TestDetectAndResolveReturnErrorWhenBinaryMissing`
  - `TestDetectPlatformReusesProjectWizardLogic`
  - `TestRegistryFileExtensionsCorrect`
  - `TestDetectPlatformAndroid`
  - `TestDetectPlatformPython`

## 7. Out of Scope

- Kotlin-specific initialization options (Task-360).
- Gradle fallback validation (Task-361).
- Runner hook wiring (Task-358).
- `flowpilot doctor` command check (R-1 mitigation, CP sau).

## 8. Completion Notes

- result: Implemented with three deliberate deviations from the draft (all covered by tests): (1) `ConfigForFile(platform, path)` instead of path-only lookup — extensions alone are ambiguous (.ts belongs to nextjs/reactjs/node) and map iteration is random; ownership is now resolved per platform. (2) Added `LanguageID()` helper (needed by P-4/P-5 for didOpen languageId). (3) Python binary is `pyright-langserver` (the actual LSP server binary) rather than the doc's literal `pyright` CLI. `detectProjectPlatform` now delegates to `lsp.DetectPlatform` with byte-identical behavior — pre-existing wizard tests untouched and green. All 8 test signatures green (+1 extra LanguageID test).
- follow-ups: C/C++ follow-up DONE (2026-09-15, CA-871) — `cpp` token (CMakeLists/Makefile markers), clangd registry entry, c/cpp LanguageIDs. TUI `wizardPlatformOptions` deliberately untouched.
- upstream docs updated: CP-63 P-3 marked done.