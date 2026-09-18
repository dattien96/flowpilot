# Task-360: Kotlin Language Server Integration

## Metadata

- Document ID: `Task-360`
- Title: `Kotlin Language Server Integration`
- Phase: `task`
- Status: `done`
- Owner: `Claude Sonnet MAX`
- Reviewers: `Claude agent review`
- Created: `2026-09-15`
- Last Updated: `2026-09-15`
- Parent Documents: [CP-63 P-7](../../07-Coding-Plan/todo/CP-63-IDE-Grade-LSP-Runtime.md)
- Child Documents: `None`
- Related Documents: [SD-17](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md), [Task-356](../todo/Task-356-LSP-Platform-Registry-And-Auto-Detection.md), [Task-361](../todo/Task-361-Android-Gradle-Build-Fallback.md)
- Replaces: `None`
- Tags: `lsp, kotlin, android, gradle, kotlin-language-server, code-intelligence`
- Feature Keys: `lsp-runtime`

## AI Quick View

### Summary

- Slice P-7 của CP-63: finalize `android` platform config trong `platform_registry.go` (Task-356 đã đăng ký draft entry) — binary `kotlin-language-server`, args `--stdio`, extensions `.kt`/`.kts`, initialization options cho Gradle/Android project.
- `kotlin_config.go` thêm `KotlinInitializationOptions` (Gradle settings, classpath hints) + `DetectAndroidSDK` đọc `local.properties` (`sdk.dir`) fallback `ANDROID_HOME`.
- Document known gaps: kotlin-language-server diagnostics có thể thiếu cho Android-specific types (R class, Compose) — Gradle fallback (Task-361) bù gap này.
- `Q-1` mở: JetBrains Kotlin LSP chính thức — nếu release trước khi CP-63 done, re-evaluate strategy (chỉ note, không block).

### Current Ask

- Implement P-7 theo CP-63 §4: `kotlin_config.go` + update `platform_registry.go` + `kotlin_config_test.go`. 6 test signatures phải xanh.

### Key Decisions

- `T-1` Android config chính thức: `Platform = "android"`, `Binary = "kotlin-language-server"`, `Args = ["--stdio"]`, `FileExtensions = [".kt", ".kts"]`, `InitializationOptions = KotlinInitializationOptions`.
- `T-2` `KotlinInitializationOptions` struct: `CompilerPlugins` (Compose), `GradleDependencyPath` (classpath hints), `GradleHome`, `SdkPath` — populate từ `DetectAndroidSDK` + đọc `gradle.properties`.
- `T-3` `DetectAndroidSDK(workspaceRoot string) (string, error)` — ưu tiên `local.properties` `sdk.dir`, fallback `ANDROID_HOME` env, cả hai thiếu → error (degrade gracefully — caller log, LSP kotlin vẫn chạy không cần SDK path).
- `T-4` Kotlin server startup failure (kotlin-language-server không có trong PATH hoặc crash) → graceful degradation như mọi platform khác: log warning, fallback Gradle (Task-361) + build command.
- `T-5` Known gap documentation: ghi rõ trong code comment + task completion notes rằng R class / Compose generated types có thể thiếu trong diagnostics của kotlin-language-server — không cố sửa ở slice này.

### Constraints

- Additive tests only — không edit pre-existing tests.
- Không thay đổi behavior của các platform khác trong registry (chỉ finalize android entry).
- Không auto-install kotlin-language-server — chỉ check PATH (R-1 mitigation thuộc `flowpilot doctor`).
- GitNexus impact analysis trước mỗi symbol edit.

### Open Questions

- `Q-1` JetBrains Kotlin LSP server chính thức — nếu release trước khi CP-63 done, re-evaluate P-7 strategy (CP-63 §Open Questions).

### Source Refs

- CP-63 P-7 (§4), test signatures 1–6.
- Task-356 (registry draft entry android).

## 1. Goal

Android workspace được auto-detect → Runner spawn `kotlin-language-server` với init options đúng Gradle/Android context, SDK path biết được từ `local.properties` hoặc env — sẵn sàng cho diagnostics pipeline (P-5/P-6) trên Kotlin files.

## 2. Parent Links

- coding plan: `CP-63-IDE-Grade-LSP-Runtime.md` P-7
- tech design: `SD-17-Context-And-Regression-Engine.md`
- system spec: `SS-14-Code-Context-And-Regression-Safety.md`
- specific upstream ids: Task-356 (registry), Task-361 (Gradle fallback — bù gap)

## 3. Trigger

Registry (Task-356) đã map android → kotlin-language-server nhưng chưa có args/init options cụ thể và chưa biết lấy Android SDK path từ đâu. Không finalize thì Android flow không thể nhận LSP diagnostics.

## 4. Exact Change

- `T-1` **`internal/lsp/kotlin_config.go`** (new): `KotlinInitializationOptions` struct + `DetectAndroidSDK(workspaceRoot string) (string, error)` (local.properties → ANDROID_HOME → error).
- `T-2` **`internal/lsp/kotlin_config.go`**: `BuildKotlinInitOptions(workspaceRoot string) (map[string]interface{}, error)` — gộp SDK path, Gradle settings từ `gradle.properties`, Compose compiler plugin flag.
- `T-3` **`internal/lsp/platform_registry.go`** (update): finalize android entry — binary `kotlin-language-server`, args `--stdio`, extensions `.kt`/`.kts`, init options từ T-2.
- `T-4` **`internal/lsp/kotlin_config_test.go`** (new): 6 test signatures dưới đây.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/lsp/kotlin_config.go` (new)
  - `apps/local-runner/internal/lsp/kotlin_config_test.go` (new)
  - `apps/local-runner/internal/lsp/platform_registry.go` (update — android entry)
- modules: `lsp`
- routes: none
- tables: none

## 6. Acceptance Check

- [ ] AC-1: `KotlinInitializationOptions` bao gồm Gradle settings (sdk path, dependency path).
- [ ] AC-2: `DetectAndroidSDK` đọc được `sdk.dir` từ `local.properties`.
- [ ] AC-3: `DetectAndroidSDK` fallback `ANDROID_HOME` env khi không có local.properties.
- [ ] AC-4: `DetectAndroidSDK` trả error khi cả hai nguồn đều thiếu.
- [ ] AC-5: Registry có android entry hoàn chỉnh (binary, args, extensions).
- [ ] AC-6: File extensions `.kt`/`.kts` đúng.
- [ ] AC-7: Known gap (R class / Compose types) được ghi trong code comment.
- [ ] AC-8: 6 tests green:
  - `TestKotlinInitOptionsIncludesGradleSettings`
  - `TestDetectAndroidSDKFromLocalProperties`
  - `TestDetectAndroidSDKFromEnvVar`
  - `TestDetectAndroidSDKReturnsErrorWhenMissing`
  - `TestKotlinPlatformConfigInRegistry`
  - `TestKotlinFileExtensionsCorrect`

## 7. Out of Scope

- Gradle build fallback validation (Task-361).
- Auto-install kotlin-language-server.
- JetBrains official Kotlin LSP migration (chỉ theo dõi Q-1).
- Compose compiler full support (known gap — document only).

## 8. Completion Notes

- result: Implemented as specified plus thin handshake wiring so the options actually reach the server (not dead config): `ServerManager.InitOptions` (sent by WaitReady, nil-safe for all P-2 tests) + `ServerSet.InitOptionsFor` hook + `KotlinInitOptionsFor` adapter. Known gap documented in code (R class / Compose generated types → covered by Task-361). SDK missing is graceful (key omitted, no error). All 6 test signatures green (+2: handshake capture, hook shape).
- follow-ups: Q-1 (official JetBrains Kotlin LSP) still open — re-evaluate if released.
- upstream docs updated: CP-63 P-7 marked done.