# CP-63: IDE-Grade LSP Runtime

## Metadata

- Document ID: `CP-63`
- Title: `IDE-Grade LSP Runtime`
- Phase: `coding_plan`
- Status: `draft`
- Owner: `FlowPilot Architecture`
- Reviewers: `Claude agent review`
- Created: `2026-09-15`
- Last Updated: `2026-09-15`
- Parent Documents: [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: [Task-354](../../08-Task/todo/Task-354-LSP-JSONRPC-Client-Core.md) (P-1), [Task-355](../../08-Task/todo/Task-355-LSP-Server-Lifecycle-Manager.md) (P-2), [Task-356](../../08-Task/todo/Task-356-LSP-Platform-Registry-And-Auto-Detection.md) (P-3), [Task-357](../../08-Task/todo/Task-357-LSP-Document-Sync-And-Diagnostics-Collection.md) (P-4), [Task-358](../../08-Task/todo/Task-358-LSP-Post-Write-Diagnostics-Hook.md) (P-5), [Task-359](../../08-Task/todo/Task-359-LSP-Diagnostics-Context-Source.md) (P-6), [Task-360](../../08-Task/todo/Task-360-Kotlin-Language-Server-Integration.md) (P-7), [Task-361](../../08-Task/todo/Task-361-Android-Gradle-Build-Fallback.md) (P-8)
- Related Documents: [CP-55: Flow-First Preflight Contract](../done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md), [CP-44: Pluggable Context Source Registry](../done/CP-44-Pluggable-Context-Source-Registry.md), [CP-35: Context And Regression Engine Rollout](../done/CP-35-Context-And-Regression-Engine-Rollout.md)
- Replaces: `None`
- Tags: `lsp, language-server, diagnostics, code-intelligence, compiler-feedback`
- Feature Keys: `lsp-runtime`
- Implementation Owner: `Claude Sonnet MAX`

---

## AI Quick View

### Summary

- FlowPilot hiện dùng GitNexus cho code intelligence (blast radius, execution flows), nhưng GitNexus là đồ thị tĩnh — không có live compiler diagnostics, bị stale ngay khi Agent sửa file.
- OMP (Oh-My-Pi) đã chứng minh rằng tích hợp LSP trực tiếp vào Agent runtime cho phép phát hiện lỗi compiler trong <200ms, tiết kiệm 80% token so với chạy full build.
- CP-63 xây dựng kiến trúc Two-Tier Intelligence: GitNexus (Macro Plane — blast radius, architecture gating) + LSP (Micro Plane — live diagnostics, goto-definition, find-references).
- Phase 1 đạt OMP-parity (Go, TS, Python, C/C++), Phase 2 thêm Kotlin cơ bản, Phase 3 thêm Gradle integration cho Android.
- LSP client nhúng trong Go Runner qua JSON-RPC stdio, auto-detect project platform, tự động khởi tạo đúng language server.

### Current Ask

- Implement P-1 through P-8 in order. P-1 → P-4 là core LSP infrastructure. P-5 → P-6 là wiring vào Flow pipeline. P-7 → P-8 là Kotlin/Android extension.
- Preserve all existing GitNexus behavior — LSP bổ sung, không thay thế.

### Key Decisions

- `P-1` LSP client dùng JSON-RPC over stdio (giống OMP), không dùng TCP/WebSocket — đơn giản, cross-platform, không cần port management.
- `P-2` Mỗi workspace chỉ chạy 1 LSP server instance per language — managed lifecycle (start/stop/restart) bởi `ServerManager`.
- `P-3` Platform auto-detection reuse `project_wizard.go` logic, mở rộng để map platform → LSP server binary.
- `P-4` Pre-gate diagnostics hook: sau mỗi file write, Runner tự query `textDocument/publishDiagnostics` → nếu có compiler error → auto-reprompt Agent trước khi chạy test.
- `P-5` LSP diagnostics là một context source mới cắm vào CP-44 registry, không hardcode vào runner loop.

### Constraints

- Không xóa hoặc disable GitNexus — LSP bổ sung ở Micro Plane, GitNexus giữ nguyên ở Macro Plane.
- LSP server binaries (`gopls`, `typescript-language-server`, `pyright`, `clangd`, `kotlin-language-server`) phải được cài riêng bởi user hoặc auto-install script — Runner chỉ check PATH.
- Không block Agent nếu LSP server không available — graceful degradation, log warning, fallback về build command.
- Additive tests only — không edit pre-existing tests.
- GitNexus impact analysis trước mỗi symbol edit.

### Open Questions

- `Q-1` JetBrains Kotlin LSP server chính thức — nếu release trước khi CP-63 done, cần re-evaluate P-7 strategy.
- `Q-2` DAP (Debug Adapter Protocol) integration — OMP có, nhưng deferred cho CP sau.

### Source Refs

- SS-14; SD-17; CP-44 (context source registry); CP-55 (flow-first contract).
- OMP architecture reference: `gopls` (Go), `vtsls` (TS), `pyright` (Python), `clangd` (C/C++).
- Existing skill: `apps/local-runner/internal/skillpack/flow-pack/golang/golang-gopls/SKILL.md`.

---

## 1. Goal

Nhúng một LSP client nhẹ vào Go Runner để FlowPilot có khả năng code intelligence tức thì (live diagnostics, semantic navigation) giống OMP, đồng thời giữ nguyên GitNexus cho blast radius và execution flow analysis.

Kiến trúc mục tiêu — Two-Tier Intelligence:

```text
User issue
    ↓
GitNexus (Macro Plane): Blast radius, execution flows, architecture gating
    ↓
Flow preflight contract (CP-55)
    ↓
LSP (Micro Plane): Live diagnostics after each file edit
    ↓
Agent code → write file → LSP diagnostics → auto-fix if error → repeat
    ↓
./gradlew test (hoặc go test, npm test...) chỉ khi LSP clean
    ↓
Gate → Review → Accept
```

### 1.1 Product boundary

| Surface | LSP diagnostics | LSP navigation | GitNexus blast radius | Build command fallback |
|---|---:|---:|---:|---:|
| Flow mode | Required (auto-reprompt on error) | Available | Required (per AGENTS.md) | Fallback if LSP unavailable |
| Normal chat | Best-effort (log warning, no block) | Available | Best-effort | Always available |
| TUI standalone | Same as above (depends on runner mode) | Available | Same as above | Always available |

---

## 2. Input Documents

### 2.1 Governing documents

- SS-14 định nghĩa yêu cầu về context correctness và regression safety.
- SD-17 định nghĩa context sources, history retrieval và regression engine.
- CP-44 định nghĩa pluggable context source registry.

### 2.2 Implemented foundations

- CP-55 / Task-263–271: Flow-first preflight contract, scope freeze, terminal acceptance.
- CP-44: Pluggable context source registry — LSP diagnostics sẽ là một source mới.
- `skillpack/flow-pack/golang/golang-gopls/SKILL.md`: Existing gopls skill dùng MCP/CLI — reference cho server interaction patterns.

### 2.3 Latest relevant implementation history

- `cli-tui`: CA-867 (standalone TUI onboarding — project wizard auto-detects platform).
- `context-regression-engine`: CA-423 (shared retrieval locus builder).
- `agent-flow-engine`: CA-431 (built-in flow migration with preflight).

### 2.4 Current code locations

| Concern | Current location |
|---|---|
| Platform detection | `apps/local-runner/internal/tui/app/project_wizard.go` (`detectPlatform()`) |
| Context source registry | `apps/local-runner/internal/runner/` (CP-44 registry) |
| Flow gate hooks | `apps/local-runner/internal/runner/gate_hook.go` |
| File write tool handler | `apps/local-runner/internal/runner/` (tool execution) |
| GitNexus integration | `apps/local-runner/internal/structure/gitnexus.go` |
| Skill packs | `apps/local-runner/internal/skillpack/flow-pack/` |

---

## 3. Implementation Strategy

### 3.1 Current behavior

Sau khi Agent sửa file:
1. Không có compiler feedback tức thì.
2. Agent phải chạy `go test`, `npm run build`, hoặc `./gradlew assembleDebug` để phát hiện lỗi.
3. Full build cycle: 5–120 giây tùy project.
4. Mỗi lỗi = 1 turn mới = token cost.

### 3.2 Target behavior (học từ OMP)

Sau khi Agent sửa file:
1. Runner gửi `textDocument/didChange` tới LSP server.
2. LSP trả về diagnostics trong <200ms.
3. Nếu có error → Runner auto-reprompt: "Lỗi tại dòng X: Y. Sửa lại."
4. Agent sửa → didChange → diagnostics → repeat cho đến clean.
5. Chỉ khi diagnostics clean → mới chạy test command.

```text
Agent calls write_file("foo.go", content)
    ↓
Runner writes file to disk
    ↓
Runner sends textDocument/didChange to gopls
    ↓
gopls returns publishDiagnostics
    ↓
  ┌─ 0 errors → proceed to next tool call
  └─ N errors → Runner injects diagnostic message into conversation
                 → Agent auto-fixes → repeat from didChange
```

### 3.3 LSP Client architecture

```text
┌─────────────────────────────────────────┐
│ apps/local-runner/internal/lsp/         │
│                                         │
│ ┌─────────────┐  ┌──────────────────┐   │
│ │  client.go   │  │ server_manager.go│   │
│ │ (JSON-RPC    │  │ (lifecycle:      │   │
│ │  over stdio) │  │  start/stop/     │   │
│ │              │  │  restart/health) │   │
│ └──────┬───────┘  └────────┬─────────┘  │
│        │                   │             │
│ ┌──────▼───────────────────▼──────────┐ │
│ │         protocol.go                  │ │
│ │  (LSP message types: Initialize,    │ │
│ │   DidOpen, DidChange, Diagnostic,   │ │
│ │   Definition, References, Hover)    │ │
│ └─────────────────────────────────────┘ │
│                                         │
│ ┌─────────────────────────────────────┐ │
│ │     platform_registry.go            │ │
│ │  (platform → server binary mapping) │ │
│ │  golang  → gopls                    │ │
│ │  nextjs  → vtsls                    │ │
│ │  reactjs → vtsls                    │ │
│ │  python  → pyright                  │ │
│ │  rust    → rust-analyzer            │ │
│ │  node    → vtsls                    │ │
│ │  android → kotlin-language-server   │ │
│ └─────────────────────────────────────┘ │
│                                         │
│ ┌─────────────────────────────────────┐ │
│ │     diagnostics_hook.go             │ │
│ │  (post-write diagnostics collector, │ │
│ │   error formatter, reprompt builder)│ │
│ └─────────────────────────────────────┘ │
└─────────────────────────────────────────┘
```

### 3.4 Graceful degradation

| Condition | Behavior |
|---|---|
| LSP server binary not in PATH | Log warning, skip diagnostics, proceed normally |
| LSP server crashes | Auto-restart once, then disable for session with warning |
| LSP server timeout (>5s) | Skip this diagnostics cycle, log, proceed |
| Unsupported platform | No LSP, full fallback to build commands |

---

## 4. Work Breakdown

### P-1: LSP JSON-RPC Client Core

**Status: done** — Task: [Task-354](../../08-Task/todo/Task-354-LSP-JSONRPC-Client-Core.md) + [CA-869](../../../change-audit/CA-869-CP-63-IDE-Grade-LSP-Runtime-Implementation.md)

**Production changes**
- `internal/lsp/protocol.go` (**new**): LSP protocol message types — `InitializeParams`, `InitializeResult`, `DidOpenTextDocumentParams`, `DidChangeTextDocumentParams`, `PublishDiagnosticsParams`, `Diagnostic`, `Position`, `Range`, `Location`, `TextDocumentIdentifier`, `TextDocumentItem`, `VersionedTextDocumentIdentifier`, `ContentChangeEvent`.
- `internal/lsp/client.go` (**new**): JSON-RPC client over stdio — `Client` struct with `NewClient(stdin io.Writer, stdout io.Reader)`, methods `Initialize`, `Shutdown`, `DidOpen`, `DidChange`, `DidClose`. Internal goroutine reads stdout, demuxes responses/notifications by ID/method. Thread-safe request/response via channel map.
- `internal/lsp/client.go`: `handleNotification(method string, params json.RawMessage)` dispatches `textDocument/publishDiagnostics` to a registered callback.
- JSON-RPC message framing: `Content-Length: N\r\n\r\n{...}` per LSP spec.

**Candidate files**
- `apps/local-runner/internal/lsp/protocol.go`
- `apps/local-runner/internal/lsp/client.go`
- `apps/local-runner/internal/lsp/client_test.go`

**Test signatures**
```go
func TestClientSendsInitializeRequest(t *testing.T)
func TestClientReceivesInitializeResponse(t *testing.T)
func TestClientSendsDidOpenNotification(t *testing.T)
func TestClientSendsDidChangeNotification(t *testing.T)
func TestClientReceivesDiagnosticsNotification(t *testing.T)
func TestClientHandlesMalformedResponse(t *testing.T)
func TestClientHandlesConcurrentRequests(t *testing.T)
func TestClientShutdownSendsExitNotification(t *testing.T)
func TestProtocolMessageFraming(t *testing.T)
func TestProtocolParseContentLength(t *testing.T)
```

**Exit condition**
- `Client` can exchange Initialize handshake with a mock stdio server.
- `DidOpen`/`DidChange` notifications serialize correctly per LSP spec.
- Diagnostics callback fires on `textDocument/publishDiagnostics`.
- Concurrent requests don't deadlock or lose responses.
- All 10 tests green.

---

### P-2: Server Lifecycle Manager

**Status: done** — Task: [Task-355](../../08-Task/todo/Task-355-LSP-Server-Lifecycle-Manager.md) + [CA-869](../../../change-audit/CA-869-CP-63-IDE-Grade-LSP-Runtime-Implementation.md)

**Production changes**
- `internal/lsp/server_manager.go` (**new**): `ServerManager` struct — manages OS process lifecycle for LSP servers. Methods: `Start(ctx, binary string, args []string, workspaceRoot string) error`, `Stop() error`, `Restart() error`, `IsRunning() bool`, `Client() *Client`. Pipes stdin/stdout to `Client`. Stderr captured to log.
- `internal/lsp/server_manager.go`: Health check goroutine — pings with `$/alive` or re-initialize every 30s, auto-restart on crash (max 1 restart per session).
- `internal/lsp/server_manager.go`: `WaitReady(ctx context.Context, timeout time.Duration) error` — blocks until Initialize handshake completes or timeout.

**Candidate files**
- `apps/local-runner/internal/lsp/server_manager.go`
- `apps/local-runner/internal/lsp/server_manager_test.go`

**Test signatures**
```go
func TestServerManagerStartSpawnsProcess(t *testing.T)
func TestServerManagerStopKillsProcess(t *testing.T)
func TestServerManagerRestartRecreatesClient(t *testing.T)
func TestServerManagerAutoRestartOnCrash(t *testing.T)
func TestServerManagerMaxOneRestartPerSession(t *testing.T)
func TestServerManagerWaitReadyTimesOut(t *testing.T)
func TestServerManagerWaitReadySucceeds(t *testing.T)
func TestServerManagerIsRunningReflectsState(t *testing.T)
```

**Exit condition**
- `ServerManager` can start/stop a real `gopls` process (integration) or mock binary (unit).
- Auto-restart fires once on crash, then disables with warning.
- `WaitReady` blocks correctly with timeout.
- All 8 tests green.

---

### P-3: Platform Registry and Auto-Detection

**Status: done** — Task: [Task-356](../../08-Task/todo/Task-356-LSP-Platform-Registry-And-Auto-Detection.md) + [CA-869](../../../change-audit/CA-869-CP-63-IDE-Grade-LSP-Runtime-Implementation.md)

**Production changes**
- `internal/lsp/platform_registry.go` (**new**): `PlatformLSPConfig` struct (`Platform string`, `Binary string`, `Args []string`, `FileExtensions []string`, `InitializationOptions map[string]interface{}`). `Registry` map keyed by platform. `DefaultRegistry()` returns configs for: `golang→gopls`, `nextjs/reactjs/node→vtsls`, `python→pyright`, `rust→rust-analyzer`, `android→kotlin-language-server`.
- `internal/lsp/platform_registry.go`: `DetectAndResolve(workspaceRoot string) (PlatformLSPConfig, error)` — reuses `detectPlatform()` logic from `project_wizard.go` (refactored to shared util), looks up binary in PATH via `exec.LookPath`, returns error if not found (graceful degradation — caller logs warning, continues without LSP).
- `internal/lsp/platform_detect.go` (**new**): Extracted shared `DetectPlatform(dir string) string` from `project_wizard.go` into a reusable package-level function.

**Candidate files**
- `apps/local-runner/internal/lsp/platform_registry.go`
- `apps/local-runner/internal/lsp/platform_detect.go`
- `apps/local-runner/internal/lsp/platform_registry_test.go`

**Test signatures**
```go
func TestDefaultRegistryContainsAllPlatforms(t *testing.T)
func TestDetectAndResolveFindsGoplsForGolang(t *testing.T)
func TestDetectAndResolveFindsVtslsForNextjs(t *testing.T)
func TestDetectAndResolveReturnErrorWhenBinaryMissing(t *testing.T)
func TestDetectPlatformReusesProjectWizardLogic(t *testing.T)
func TestRegistryFileExtensionsCorrect(t *testing.T)
func TestDetectPlatformAndroid(t *testing.T)
func TestDetectPlatformPython(t *testing.T)
```

**Exit condition**
- `DefaultRegistry` maps all 7 platforms to correct binaries.
- `DetectAndResolve` finds binary in PATH or returns descriptive error.
- `DetectPlatform` gives same results as `project_wizard.go` logic.
- All 8 tests green.

---

### P-4: Document Sync and Diagnostics Collection

**Status: done** — Task: [Task-357](../../08-Task/todo/Task-357-LSP-Document-Sync-And-Diagnostics-Collection.md) + [CA-869](../../../change-audit/CA-869-CP-63-IDE-Grade-LSP-Runtime-Implementation.md)

**Production changes**
- `internal/lsp/doc_sync.go` (**new**): `DocumentSyncManager` — tracks open documents, version numbers. Methods: `OpenDocument(uri, languageID, content string)`, `ChangeDocument(uri, newContent string)`, `CloseDocument(uri string)`. Each method delegates to `Client.DidOpen`/`DidChange`/`DidClose` with correct versioning.
- `internal/lsp/diagnostics_collector.go` (**new**): `DiagnosticsCollector` — receives `publishDiagnostics` via callback registered on `Client`, stores latest diagnostics per URI. Methods: `GetDiagnostics(uri string) []Diagnostic`, `GetAllErrors() []FileDiagnostic` (filters Severity == Error), `HasErrors() bool`, `WaitForDiagnostics(ctx, timeout) error` (waits until diagnostics arrive or timeout).
- `internal/lsp/diagnostics_collector.go`: `FormatDiagnosticsForAgent(diagnostics []FileDiagnostic) string` — renders human-readable error report: `file.go:42:10 error: undefined: Foo`.

**Candidate files**
- `apps/local-runner/internal/lsp/doc_sync.go`
- `apps/local-runner/internal/lsp/diagnostics_collector.go`
- `apps/local-runner/internal/lsp/doc_sync_test.go`
- `apps/local-runner/internal/lsp/diagnostics_collector_test.go`

**Test signatures**
```go
func TestDocSyncOpenSendsDidOpen(t *testing.T)
func TestDocSyncChangeIncrementsVersion(t *testing.T)
func TestDocSyncCloseSendsDidClose(t *testing.T)
func TestDocSyncDoubleOpenIsIdempotent(t *testing.T)
func TestDiagnosticsCollectorStoresByURI(t *testing.T)
func TestDiagnosticsCollectorGetAllErrorsFiltersWarnings(t *testing.T)
func TestDiagnosticsCollectorHasErrorsReturnsFalseWhenClean(t *testing.T)
func TestDiagnosticsCollectorWaitForDiagnosticsTimesOut(t *testing.T)
func TestDiagnosticsCollectorWaitForDiagnosticsSucceeds(t *testing.T)
func TestFormatDiagnosticsForAgentOutput(t *testing.T)
```

**Exit condition**
- `DocumentSyncManager` correctly tracks version per URI.
- `DiagnosticsCollector` stores and filters diagnostics by severity.
- `FormatDiagnosticsForAgent` produces `file:line:col error: message` format.
- All 10 tests green.

---

### P-5: Runner Integration — Post-Write Diagnostics Hook

**Status: done** — Task: [Task-358](../../08-Task/todo/Task-358-LSP-Post-Write-Diagnostics-Hook.md) + [CA-869](../../../change-audit/CA-869-CP-63-IDE-Grade-LSP-Runtime-Implementation.md)

**Production changes**
- `internal/lsp/runner_hook.go` (**new**): `PostWriteDiagnosticsHook` — called by Runner after `write_file` or `replace_file_content` tool execution. Sequence: (1) `DocSync.ChangeDocument(uri, newContent)`, (2) `Collector.WaitForDiagnostics(5s)`, (3) if `Collector.HasErrors()` → return formatted diagnostic string for injection into conversation, (4) if clean → return nil.
- `internal/runner/` (existing file TBD after impact analysis): Wire `PostWriteDiagnosticsHook` into tool execution pipeline — after file write completes, call hook, if diagnostic string returned → inject as system message before next Agent turn.
- Graceful degradation: if `ServerManager.IsRunning()` is false → skip hook entirely.

**Candidate files**
- `apps/local-runner/internal/lsp/runner_hook.go`
- `apps/local-runner/internal/lsp/runner_hook_test.go`
- `apps/local-runner/internal/runner/` (existing — exact file determined by impact analysis)

**Test signatures**
```go
func TestPostWriteHookReturnsNilWhenClean(t *testing.T)
func TestPostWriteHookReturnsDiagnosticsOnError(t *testing.T)
func TestPostWriteHookSkipsWhenLSPNotRunning(t *testing.T)
func TestPostWriteHookTimesOutGracefully(t *testing.T)
func TestPostWriteHookFormatsMultipleErrors(t *testing.T)
func TestRunnerInjectsLSPDiagnosticsAfterWrite(t *testing.T)
func TestRunnerSkipsLSPHookWhenNoServer(t *testing.T)
```

**Exit condition**
- File write → LSP diagnostics → error injection works end-to-end with mock server.
- Graceful degradation: no crash/block when LSP unavailable.
- All 7 tests green.

---

### P-6: LSP as Context Source (CP-44 Registry)

**Status: done** — Task: [Task-359](../../08-Task/todo/Task-359-LSP-Diagnostics-Context-Source.md) + [CA-869](../../../change-audit/CA-869-CP-63-IDE-Grade-LSP-Runtime-Implementation.md)

**Production changes**
- `internal/lsp/context_source.go` (**new**): Implements CP-44 context source interface. `LSPDiagnosticsSource` provides latest diagnostics as a context slot — Agent sees current compiler errors in its context window. Slot key: `lsp.diagnostics`. Rendered section: "## Current Compiler Diagnostics (LSP)" with error list or "No compiler errors detected."
- `internal/lsp/context_source.go`: `LSPNavigationSource` (optional, can be deferred) — provides goto-definition and find-references results as context.
- Wire into CP-44 registry: register `lsp.diagnostics` source when LSP server is available.

**Candidate files**
- `apps/local-runner/internal/lsp/context_source.go`
- `apps/local-runner/internal/lsp/context_source_test.go`
- `apps/local-runner/internal/runner/` (CP-44 registry wiring)

**Test signatures**
```go
func TestLSPDiagnosticsSourceRendersErrors(t *testing.T)
func TestLSPDiagnosticsSourceRendersCleanMessage(t *testing.T)
func TestLSPDiagnosticsSourceSlotKey(t *testing.T)
func TestLSPDiagnosticsSourceSkipsWhenNoServer(t *testing.T)
func TestLSPDiagnosticsSourceRegisteredInCP44Registry(t *testing.T)
```

**Exit condition**
- `lsp.diagnostics` context source renders correctly in Agent prompt.
- Source gracefully returns empty when LSP unavailable.
- All 5 tests green.

---

### P-7: Kotlin Language Server Integration

**Status: done** — Task: [Task-360](../../08-Task/todo/Task-360-Kotlin-Language-Server-Integration.md) + [CA-869](../../../change-audit/CA-869-CP-63-IDE-Grade-LSP-Runtime-Implementation.md)

**Production changes**
- Update `platform_registry.go`: Finalize `android` platform config — `kotlin-language-server` binary, `--stdio` args, `.kt`/`.kts` extensions, initialization options for Gradle/Android project.
- `internal/lsp/kotlin_config.go` (**new**): Kotlin-specific LSP initialization — `KotlinInitializationOptions` struct with Gradle-related settings, classpath hints from `local.properties` / `gradle.properties`.
- `internal/lsp/kotlin_config.go`: `DetectAndroidSDK(workspaceRoot string) (string, error)` — reads `local.properties` for `sdk.dir`, falls back to `ANDROID_HOME` env var.
- Handle known `kotlin-language-server` limitations: diagnostics may be incomplete for Android-specific types (R class, Compose). Document as known gap.

**Candidate files**
- `apps/local-runner/internal/lsp/kotlin_config.go`
- `apps/local-runner/internal/lsp/platform_registry.go` (update)
- `apps/local-runner/internal/lsp/kotlin_config_test.go`

**Test signatures**
```go
func TestKotlinInitOptionsIncludesGradleSettings(t *testing.T)
func TestDetectAndroidSDKFromLocalProperties(t *testing.T)
func TestDetectAndroidSDKFromEnvVar(t *testing.T)
func TestDetectAndroidSDKReturnsErrorWhenMissing(t *testing.T)
func TestKotlinPlatformConfigInRegistry(t *testing.T)
func TestKotlinFileExtensionsCorrect(t *testing.T)
```

**Exit condition**
- `kotlin-language-server` config registered with correct binary and args.
- Android SDK detection works from `local.properties` and env var.
- All 6 tests green.

---

### P-8: Android Gradle Build Fallback

**Status: done** — Task: [Task-361](../../08-Task/todo/Task-361-Android-Gradle-Build-Fallback.md) + [CA-869](../../../change-audit/CA-869-CP-63-IDE-Grade-LSP-Runtime-Implementation.md)

**Production changes**
- `internal/lsp/gradle_fallback.go` (**new**): `GradleFallbackValidator` — for Android projects, after LSP diagnostics pass (or when LSP reports 0 errors but Android-specific errors may still exist), optionally run `./gradlew assembleDebug --dry-run` or `./gradlew compileDebugKotlin` for deeper validation.
- `internal/lsp/gradle_fallback.go`: `ShouldRunGradleFallback(diagnostics []FileDiagnostic, platform string) bool` — returns true when platform is `android` AND diagnostics count is 0 (LSP may miss R class errors).
- `internal/lsp/gradle_fallback.go`: `RunGradleValidation(ctx context.Context, workspaceRoot string) ([]GradleError, error)` — executes Gradle, parses compiler output into `GradleError` structs (file, line, message).
- `internal/lsp/gradle_fallback.go`: `FormatGradleErrorsForAgent(errors []GradleError) string`.

**Candidate files**
- `apps/local-runner/internal/lsp/gradle_fallback.go`
- `apps/local-runner/internal/lsp/gradle_fallback_test.go`

**Test signatures**
```go
func TestShouldRunGradleFallbackTrueForAndroidCleanDiagnostics(t *testing.T)
func TestShouldRunGradleFallbackFalseForNonAndroid(t *testing.T)
func TestShouldRunGradleFallbackFalseWhenDiagnosticsExist(t *testing.T)
func TestRunGradleValidationParsesCompilerOutput(t *testing.T)
func TestRunGradleValidationHandlesMissingGradlew(t *testing.T)
func TestFormatGradleErrorsForAgentOutput(t *testing.T)
func TestGradleFallbackTimesOut(t *testing.T)
```

**Exit condition**
- Gradle fallback only triggers for Android platform with 0 LSP errors.
- Gradle compiler output parsed into structured errors.
- Missing `gradlew` handled gracefully.
- All 7 tests green.

---

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/lsp/protocol.go` (new)
  - `apps/local-runner/internal/lsp/client.go` (new)
  - `apps/local-runner/internal/lsp/server_manager.go` (new)
  - `apps/local-runner/internal/lsp/platform_registry.go` (new)
  - `apps/local-runner/internal/lsp/platform_detect.go` (new)
  - `apps/local-runner/internal/lsp/doc_sync.go` (new)
  - `apps/local-runner/internal/lsp/diagnostics_collector.go` (new)
  - `apps/local-runner/internal/lsp/runner_hook.go` (new)
  - `apps/local-runner/internal/lsp/context_source.go` (new)
  - `apps/local-runner/internal/lsp/kotlin_config.go` (new)
  - `apps/local-runner/internal/lsp/gradle_fallback.go` (new)
  - `apps/local-runner/internal/runner/` (existing — wiring hook + context source)
  - `apps/local-runner/internal/tui/app/project_wizard.go` (refactor: extract `DetectPlatform`)
- modules: `lsp` (new), `runner` (modified), `tui/app` (minor refactor)
- database: none
- external systems: LSP server binaries (gopls, vtsls, pyright, clangd, kotlin-language-server, rust-analyzer)

## 6. Data or Migration Steps

- schema: none
- data backfill: none
- config updates: Optional `~/.flowpilot/settings/lsp-config.json` for custom binary paths and per-language overrides. Not required — defaults to PATH lookup.

## 7. Validation Plan

- tests to add: ~61 tests across 8 slices (see P-1 through P-8 test signatures)
- manual checks:
  - Start TUI on a Go project → verify gopls diagnostics appear after introducing a type error
  - Start TUI on a TS/Next.js project → verify vtsls diagnostics
  - Start TUI on an Android project → verify kotlin-language-server + Gradle fallback
  - Verify graceful degradation when LSP binary not installed
- failure cases:
  - LSP server crash mid-session → auto-restart → continue
  - LSP server not in PATH → warning logged → no diagnostics → build fallback works
  - LSP timeout → skip diagnostics → proceed normally
  - Corrupt JSON-RPC response → log error → continue

## 8. Rollout and Fallback

- rollout order:
  1. P-1 → P-2 → P-3 (core infrastructure, no user-visible change)
  2. P-4 (diagnostics collection, still internal)
  3. P-5 (runner integration — first user-visible: diagnostics injected into conversation)
  4. P-6 (context source — diagnostics appear in Agent context window)
  5. P-7 → P-8 (Kotlin/Android extension)
- fallback path: LSP is entirely optional. If any LSP component fails, Runner operates exactly as before CP-63. No migration needed, no data dependency.
- monitoring: Structured logs for LSP lifecycle events (`lsp.start`, `lsp.crash`, `lsp.restart`, `lsp.diagnostics.count`, `lsp.timeout`).

## 9. Risks

- `R-1` **LSP server binary availability**: Users must install language servers separately. Mitigation: `flowpilot doctor` command checks PATH for required binaries, suggests install commands.
- `R-2` **kotlin-language-server quality**: Community-maintained, may not support all Android features. Mitigation: Gradle fallback (P-8) covers the gap; document known limitations.
- `R-3` **Performance overhead**: Running LSP server per workspace adds memory (~50–200MB per server). Mitigation: Lazy start (only when first file edit detected), auto-shutdown after 5 min idle.
- `R-4` **JSON-RPC edge cases**: Malformed messages, partial reads, encoding issues. Mitigation: Robust framing parser with tests for edge cases, timeout on every read.

## 10. Definition of Done

### Core Infrastructure
- [ ] JSON-RPC client handles Initialize handshake, DidOpen/DidChange/DidClose, and diagnostics notification.
- [ ] ServerManager starts/stops/restarts LSP server processes with health monitoring.
- [ ] Platform registry auto-detects project language and resolves correct LSP binary.

### Diagnostics Pipeline
- [ ] DocumentSyncManager correctly tracks open file versions.
- [ ] DiagnosticsCollector stores, filters, and formats compiler errors.
- [ ] PostWriteDiagnosticsHook integrates into Runner tool execution pipeline.
- [ ] LSP diagnostics available as CP-44 context source.

### Kotlin/Android
- [ ] kotlin-language-server configuration with Android SDK detection.
- [ ] Gradle fallback covers R class and Compose compiler errors.

### Quality gates
- [ ] GitNexus impact analysis run and reported before every symbol edit.
- [ ] HIGH/CRITICAL impact reported before implementation proceeds.
- [ ] All ~61 new test signatures implemented.
- [ ] Existing green tests are not edited (additive tests only).
- [ ] Package tests pass after every slice.
- [ ] Full relevant regression suite passes.
- [ ] GitNexus detect_changes shows only expected symbols/flows before commit.
- [ ] Each implementation slice has a Task document and change-audit entry.
- [ ] Implementation reviewed by reviewer gate with zero blocking findings.
- [ ] Graceful degradation verified: no crash/block when LSP unavailable.

---

## Review Protocol

1. Implementer completes a P-* slice and creates Task + CA documents.
2. Implementer runs all tests in the slice and full regression suite.
3. Reviewer agent reviews the implementation against Task DoD.
4. Any Critical/Important findings must be fixed and mutation-tested before proceeding.
5. Implementer updates CP-63 §4 P-* status to done with Task/CA links.
6. Next slice begins only after current slice is reviewed and accepted.
