# CP-63 Test Steps — IDE-Grade LSP Runtime

## Metadata

- Document ID: `CP-63-TEST-STEPS`
- Title: `CP-63 Verification Steps (Automated + gate-sandbox Manual)`
- Phase: `verification`
- Status: `ready`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-16`
- Last Updated: `2026-09-16`
- Parent Documents: [CP-63: IDE-Grade LSP Runtime](./CP-63-IDE-Grade-LSP-Runtime.md)
- Child Documents: `None`
- Related Documents: [Task-354](../../08-Task/done/Task-354-LSP-JSONRPC-Client-Core.md), [Task-355](../../08-Task/done/Task-355-LSP-Server-Lifecycle-Manager.md), [Task-356](../../08-Task/done/Task-356-LSP-Platform-Registry-And-Auto-Detection.md), [Task-357](../../08-Task/done/Task-357-LSP-Document-Sync-And-Diagnostics-Collection.md), [Task-358](../../08-Task/done/Task-358-LSP-Post-Write-Diagnostics-Hook.md), [Task-359](../../08-Task/done/Task-359-LSP-Diagnostics-Context-Source.md), [Task-360](../../08-Task/done/Task-360-Kotlin-Language-Server-Integration.md), [Task-361](../../08-Task/done/Task-361-Android-Gradle-Build-Fallback.md), [Task-362](../../08-Task/done/Task-362-LSP-Missing-Server-Warning.md), [Task-363](../../08-Task/done/Task-363-flowpilot-doctor.md), [safe-fix-contract](../../../.agents/skills/safe-fix-contract/SKILL.md)
- Replaces: `None`
- Tags: `lsp-runtime, diagnostics, language-server, test-steps, verification, cp-63`
- Feature Keys: `lsp-runtime`

## AI Quick View

### Summary

- Danh mục kiểm thử tự động hóa và thủ công trên `gate-sandbox` cho toàn bộ CP-63 (P-1→P-8 + follow-up Task-362/363).
- Xác thực:
  1. JSON-RPC client core qua stdio (`P-1`).
  2. Server lifecycle start/stop/restart + crash recovery (`P-2`).
  3. Platform registry auto-detect + binary resolve (`P-3`).
  4. Document sync + diagnostics collector (`P-4`).
  5. Post-write diagnostics hook trong gate allow-path (`P-5`).
  6. Context source `lsp.diagnostics` opt-in (`P-6`).
  7. Kotlin init options + Android SDK detection (`P-7`).
  8. Gradle fallback validator + call-site (`P-8`).
  9. Missing-server warning UX + `flowpilot doctor` (Task-362/363).
- Giường thử thủ công (Manual Bed): `gate-sandbox` (`/Users/tiendat/Desktop/BE/gate-sandbox`).

### Current Ask

- Chạy kiểm thử tự động xác nhận toàn bộ suite `lsp`, `cli` (doctor), runner LSP hook, TUI LSP status đều PASS.
- Thực hiện xác minh thủ công trên TUI thật: Go/TS/Android project + graceful degradation.

### Key Decisions

- `V-1` **Provider-agnostic**: hook nằm ở shared post-turn gate path; grep `providerKey|ProviderKey` trong `internal/lsp/` = 0 hit.
- `V-2` **Unix/Windows parity**: fake `gradlew`/helper dùng `.bat`+`echo` trên Windows, `printf`-quoted trên Unix (CA-885).
- `V-3` **Reap ≠ Exited**: `Stop()` SIGKILL by design — assert reap (`ProcessState != nil`), không assert `Exited()` trên Unix.

### Constraints

- Không sửa test cũ (ngoại lệ CA-885: 3 dòng helper/assertion, operator-approved).
- Bed kiểm thử thủ công: `gate-sandbox`.

---

## 1. Goal

Chứng minh Micro Plane (LSP live diagnostics) hoạt động đúng từ unit đến TUI thật: handshake, lifecycle, diagnostics, hook chặn turn lỗi, warning khi thiếu binary, và graceful degradation tuyệt đối.

---

## 2. Automated — run first

Thư mục làm việc: `apps/local-runner`.

```bash
# 1. Full LSP package (P-1 → P-8 + follow-up + CA-885 matrix)
go test ./internal/lsp/... -count=1

# 2. Doctor CLI (Task-363)
go test ./internal/cli/ -run 'TestDoctor|TestFormatDoctor' -count=1 -v

# 3. Runner wiring: hook injection + lsp-status endpoint + context source
go test ./internal/runner/ -run 'TestRunnerInjectsLSP|TestRunnerSkipsLSP|TestHandleLSPStatus|TestLSPDiagnosticsSource|TestPostWriteHook' -count=1 -v

# 4. TUI client + sidebar (Task-362)
go test ./internal/tui/client/ -run 'TestGetLSPStatus' -count=1 -v
go test ./internal/tui/app/ -run 'TestLSPSidebar_|TestLSPStatusMsg_|TestSessionDefaults_QueuesLSPStatusFetch' -count=1 -v
```

| Step | Kiểm tra | Pass khi | Tick |
|---|---|---|---|
| 2.1 | Client core (`P-1`) | `TestClientSendsInitializeRequest`, `TestClientReceivesInitializeResponse`, `TestClientSendsDidOpenNotification`, `TestClientSendsDidChangeNotification`, `TestClientReceivesDiagnosticsNotification`, `TestClientHandlesConcurrentRequests`, `TestClientHandlesMalformedResponse`, `TestProtocolMessageFraming`, `TestProtocolParseContentLength` green | [x] PASS 2026-09-16 |
| 2.2 | Lifecycle (`P-2`) | `TestServerManagerStartSpawnsProcess`, `TestServerManagerStopKillsProcess`, `TestServerManagerRestartRecreatesClient`, `TestServerManagerAutoRestartOnCrash`, `TestServerManagerMaxOneRestartPerSession`, `TestServerManagerWaitReadySucceeds/TimesOut`, `TestServerManagerStopReapsKill`, `TestServerManagerStopReapsExitedServer` green | [x] PASS 2026-09-16 |
| 2.3 | Registry (`P-3`) | `TestDefaultRegistryContainsAllPlatforms`, `TestDetectAndResolveFindsGoplsForGolang/VtslsForNextjs/ClangdForCpp`, `TestDetectAndResolveReturnErrorWhenBinaryMissing`, `TestLanguageIDMapping`, `TestRegistryInstallHintsPresent` green | [x] PASS 2026-09-16 |
| 2.4 | Sync + collector (`P-4`) | `TestDocSyncOpenSendsDidOpen/ChangeIncrementsVersion/CloseSendsDidClose/DoubleOpenIsIdempotent`, `TestDiagnosticsCollectorStoresByURI/GetAllErrorsFiltersWarnings/HasErrorsReturnsFalseWhenClean/WaitForDiagnosticsSucceeds/TimesOut`, `TestFormatDiagnosticsForAgentOutput` green | [x] PASS 2026-09-16 |
| 2.5 | Hook (`P-5`) | `TestPostWriteHookReturnsNilWhenClean/ReturnsDiagnosticsOnError/SkipsWhenLSPNotRunning/TimesOutGracefully/FormatsMultipleErrors`, `TestRunnerInjectsLSPDiagnosticsAfterWrite`, `TestRunnerSkipsLSPHookWhenNoServer`, `TestServerSetRoutesByWorkspaceAndLanguage` green | [x] PASS 2026-09-16 |
| 2.6 | Context source (`P-6`) | `TestLSPDiagnosticsSourceSlotKey/RendersErrors/RendersCleanMessage/SkipsWhenNoServer`, `TestServerSetAllErrorsAggregates` green | [x] PASS 2026-09-16 |
| 2.7 | Kotlin (`P-7`) | `TestKotlinInitOptionsIncludesGradleSettings/ForHookShape/ReachHandshake`, `TestDetectAndroidSDKFromLocalProperties/FromEnvVar/ReturnsErrorWhenMissing`, `TestKotlinPlatformConfigInRegistry` green | [x] PASS 2026-09-16 |
| 2.8 | Gradle (`P-8`) | `TestShouldRunGradleFallbackTrueForAndroidCleanDiagnostics/FalseForNonAndroid/FalseWhenDiagnosticsExist`, `TestRunGradleValidationParsesCompilerOutput/HandlesMissingGradlew`, `TestGradleFallbackRunsWhenAndroidLSPClean/SkippedWhenLSPErrors/SkippedForNonAndroid/TimesOut`, `TestFakeGradlewRoundTripsSpecialLines/EndToEndIgnoresGarbage/WarningOnlyYieldsNoErrors` green | [x] PASS 2026-09-16 |
| 2.9 | Warning + doctor (Task-362/363) | `TestServerStatusInstalled/MissingBinary/UnknownPlatform`, `TestServerSetMissingBinaryWarnsOnce`, `TestHandleLSPStatusInstalled/MissingBinary/MissingPath`, `TestGetLSPStatusParsesResponse/Error`, `TestLSPSidebar_ShowsWarningWhenBinaryMissing/HiddenWhenInstalled/TruncatesLongHint`, `TestDoctorCheck_AllPresent/SomeMissing`, `TestDoctorCommand_RegistersOnRoot`, `TestFormatDoctorShowsMissingAndHints` green | [x] PASS 2026-09-16 |

---

## 3. Manual prep — project gate-sandbox

| # | Việc | Cách kiểm | Tick |
|---|---|---|---|
| P1 | Sandbox tồn tại | Thư mục `/Users/tiendat/Desktop/BE/gate-sandbox` | [ ] |
| P2 | Runner biên dịch | `cd apps/local-runner && go build ./...` thành công | [ ] |
| P3 | Toolchain mẫu | Cài `gopls` (GO), `typescript-language-server` (TS) hoặc quan sát degradation khi thiếu | [ ] |
| P4 | Khởi động TUI | `just chat-dev /Users/tiendat/Desktop/BE/gate-sandbox` | [ ] |

---

## 4. Manual Verification Steps on gate-sandbox

### Kịch bản M-1: Go project — gopls diagnostics live (P-1→P-5)

1. Mở TUI trên 1 Go project có `gopls` trong PATH.
2. Nhờ agent thêm 1 lỗi type (vd gọi hàm không tồn tại) rồi save.
3. **Quan sát**:
   - Agent bị reprompt với dòng dạng `foo.go:42:10 error: undefined: Bar` trước khi chạy `go test`.
   - Agent tự sửa → turn tiếp đi qua (diagnostics clean).
   - Log có `[lsp] lsp.start binary="gopls"`.

### Kịch bản M-2: Graceful degradation khi thiếu binary (P-5 + Task-362)

1. Đổi PATH để không còn language server nào (hoặc project platform lạ).
2. Nhờ agent sửa 1 file code.
3. **Quan sát**:
   - Flow chạy bình thường, không block/crash; log warn missing-binary đúng 1 lần/session.
   - Sidebar TUI hiện warning ≤2 dòng kèm install hint (vd `brew install gopls`).
   - `GET /client/lsp-status?path=<ws>` trả trạng thái missing.
- *HEADLESS 2026-09-16 (partial)*: `flowpilot doctor` live — 7/8 missing kèm hint, exit 1 (bằng chứng degradation signal, xem M-3). `flowpilot serve` không boot được trong sandbox này (silent exit, không listen, log rỗng — cần điều tra riêng, không phải LSP regression: handler suites §2.9 xanh). Endpoint + sidebar cần TUI/serve live → scenario vẫn mở.
- [x] **LIVE 2026-09-16 — warning path trên production code** (scratch `tmp-manual-warn`, đã xóa): workspace go.mod-only (máy không có gopls) → `Status()` trả `platform="golang" binary="gopls" installed=false warn=true` + hint/notice đầy đủ; `CheckFiles` 2 lần đều trả `""` (degrade sạch); log warn missing-binary xuất hiện **đúng 1 lần** (`[lsp] server binary "gopls" ... not found in PATH ... diagnostics disabled for this session`). Sidebar render + endpoint shape đã cover bởi §2.9 (render/handler tests xanh); pixels TUI live vẫn cần phiên TUI thật.

### Kịch bản M-3: `flowpilot doctor` (Task-363)

1. Chạy `flowpilot doctor` trong sandbox.
2. **Quan sát**: bảng per-platform + install hint; exit 1 khi thiếu binary, exit 0 khi đủ.
- [x] **LIVE 2026-09-16** (binary build từ HEAD, chạy headless): bảng 8 platform — `cpp/clangd ok /usr/bin/clangd`, 7 còn lại MISSING kèm hint (`go install ...gopls`, `npm i -g vtsls/pyright`, `rustup ...`); dòng `7 server(s) missing — install them for live compiler diagnostics.`; EXIT=1. Nhánh all-present exit 0 chưa quan sát được (máy thiếu binary).

### Kịch bản M-4: Crash recovery (P-2, R-1 mitigation quan sát)

1. Trên project có LSP đang chạy, kill tiến trình language server từ OS.
2. Nhờ agent sửa tiếp 1 file.
3. **Quan sát**: log `lsp.restart` đúng 1 lần rồi tiếp tục; crash lần 2 trong cùng session → `lsp.disabled` + warning, flow vẫn chạy bằng build fallback.

---

## 5. Log Grep (Bằng chứng Kiểm toán)

```text
[lsp] lsp.start
[lsp] lsp.stop
[lsp] lsp.restart
[lsp] lsp.disabled
file.go:42:10 error:
lsp.diagnostics
```

---

## 6. CP-63 Verification Complete When

- [x] §2 Automated chạy xanh 100% (2026-09-16: lsp full + cli + runner + tui/client).
- [x] M-1: **LIVE 2026-09-16 (grok-4.5, runner :18754, gopls tự cài)** — runner log `[lsp] lsp.start binary="gopls" root="...gate-sandbox"` (gopls do runner quản lý, agent PATH không có); 6+ turn flow viết file đi qua sạch, không false-reprompt; file probe lỗi thật cho diagnostic đúng `manual_lsp_probe.go:5:8: undefined: UndefinedSymbolXYZ` (agent tự chạy `gopls check` xác nhận trùng). *Scope note: hook-injection chưa isolate khỏi agent-tự-chạy-gopls (hook im lặng theo thiết kế) — phần đó vẫn do `TestPostWriteHook*` cover.*
- [x] M-2: **LIVE 2026-09-16 (headless)** — warning path verified trên production code (Status warn=true + CheckFiles `""` + warn-once đúng 1 lần) và doctor missing-path; sidebar pixels + endpoint live cần TUI/serve.
- [x] M-3: **LIVE 2026-09-16** — `flowpilot doctor` bảng đúng + exit 1 khi thiếu (7 missing + hints; cpp/clangd ok).
- [ ] M-4: crash → restart 1 lần → disable + fallback.
- [ ] Toàn bộ test cũ nguyên vẹn (ngoại lệ CA-885 đã ledger).

(End of file - total 147 lines)
