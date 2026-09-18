# Task-357: LSP Document Sync and Diagnostics Collection

## Metadata

- Document ID: `Task-357`
- Title: `LSP Document Sync and Diagnostics Collection`
- Phase: `task`
- Status: `done`
- Owner: `Claude Sonnet MAX`
- Reviewers: `Claude agent review`
- Created: `2026-09-15`
- Last Updated: `2026-09-15`
- Parent Documents: [CP-63 P-4](../../07-Coding-Plan/done/CP-63-IDE-Grade-LSP-Runtime.md)
- Child Documents: `None`
- Related Documents: [SD-17](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md), [Task-354](../done/Task-354-LSP-JSONRPC-Client-Core.md), [Task-355](../done/Task-355-LSP-Server-Lifecycle-Manager.md), [Task-356](../done/Task-356-LSP-Platform-Registry-And-Auto-Detection.md)
- Replaces: `None`
- Tags: `lsp, document-sync, diagnostics, versioning, code-intelligence`
- Feature Keys: `lsp-runtime`

## AI Quick View

### Summary

- Slice P-4 của CP-63: `DocumentSyncManager` track open documents + version per URI, delegate tới Client (Task-354) với versioning đúng LSP spec.
- `DiagnosticsCollector` nhận `publishDiagnostics` qua callback, lưu latest diagnostics per URI, filter theo severity, và `WaitForDiagnostics` chờ kết quả (hook P-5 dùng).
- `FormatDiagnosticsForAgent` render báo lỗi `file.go:42:10 error: undefined: Foo` — đầu vào cho auto-reprompt của Agent.
- Vẫn internal — chưa wire vào Runner.

### Current Ask

- Implement P-4 theo CP-63 §4: `doc_sync.go` + `diagnostics_collector.go` + 2 test files. 10 test signatures phải xanh.

### Key Decisions

- `T-1` `DocumentSyncManager` giữ `map[uri]version`; `OpenDocument` gửi `DidOpen` (version 1), `ChangeDocument` tăng version + gửi `DidChange`, `CloseDocument` gửi `DidClose` và xóa khỏi map.
- `T-2` `OpenDocument` double-open là idempotent (không tăng version, không gửi DidOpen lần 2 — LSP spec: một document chỉ open một lần).
- `T-3` `DiagnosticsCollector` đăng ký callback `OnDiagnostics` trên Client; `GetAllErrors()` filter `Severity == Error` (bỏ warning/info/hint).
- `T-4` `WaitForDiagnostics(ctx, timeout)` dùng per-URI version gate: chờ diagnostics với version >= version đã gửi, hoặc timeout — tránh nhận diagnostics cũ của turn trước.
- `T-5` `FormatDiagnosticsForAgent` output: `file.go:42:10 error: undefined: Foo` — mỗi error 1 dòng, có range (line:col).

### Constraints

- Additive tests only — không edit pre-existing tests.
- Graceful degradation: không có server → collector trả empty, không block.
- GitNexus impact analysis trước mỗi symbol edit.
- Không import từ `runner` package (tránh cycle) — `lsp` package đứng độc lập ở slice này.

### Open Questions

- None.

### Source Refs

- CP-63 P-4 (§4), test signatures 1–10.
- LSP spec: `textDocument/didOpen`, `didChange` (full sync, `textDocument.contentChanges`), `publishDiagnostics`.

## 1. Goal

Sau slice này, runner-side có đủ khả năng: mở/track file đang edit với version chính xác, nhận và lọc compiler diagnostics realtime, và render chúng thành text cho Agent — tất cả qua API nhỏ gọn `DocSync` + `Collector`.

## 2. Parent Links

- coding plan: `CP-63-IDE-Grade-LSP-Runtime.md` P-4
- tech design: `SD-17-Context-And-Regression-Engine.md`
- system spec: `SS-14-Code-Context-And-Regression-Safety.md`
- specific upstream ids: Task-354 (Client), Task-356 (PlatformLSPConfig.FileExtensions)

## 3. Trigger

Client + ServerManager + Platform registry đã có (Task-354/355/356) nhưng chưa có lớp quản lý document và diagnostics — đây là lớp biến raw LSP traffic thành dữ liệu hữu dụng cho hook và context source.

## 4. Exact Change

- `T-1` **`internal/lsp/doc_sync.go`** (new): `DocumentSyncManager` — `OpenDocument(uri, languageID string, content string)`, `ChangeDocument(uri, newContent string)`, `CloseDocument(uri string)`; track `map[string]int` version per uri; delegate tới `Client.DidOpen/DidChange/DidClose`.
- `T-2` **`internal/lsp/diagnostics_collector.go`** (new): `DiagnosticsCollector` — callback từ `Client.OnDiagnostics`, lưu latest `[]Diagnostic` per uri; methods `GetDiagnostics(uri string) []Diagnostic`, `GetAllErrors() []FileDiagnostic`, `HasErrors() bool`, `WaitForDiagnostics(ctx context.Context, timeout time.Duration) error`.
- `T-3` **`internal/lsp/diagnostics_collector.go`**: `FormatDiagnosticsForAgent(diagnostics []FileDiagnostic) string` — format `file.go:42:10 error: undefined: Foo`, nhiều error → nhiều dòng.
- `T-4` **`internal/lsp/doc_sync_test.go`** + **`internal/lsp/diagnostics_collector_test.go`** (new): mock stdio server (reuse pattern Task-354) cho 10 test signatures dưới đây.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/lsp/doc_sync.go` (new)
  - `apps/local-runner/internal/lsp/diagnostics_collector.go` (new)
  - `apps/local-runner/internal/lsp/doc_sync_test.go` (new)
  - `apps/local-runner/internal/lsp/diagnostics_collector_test.go` (new)
- modules: `lsp`
- routes: none
- tables: none

## 6. Acceptance Check

- [ ] AC-1: `OpenDocument` gửi `DidOpen` đúng languageID + content, version khởi tạo = 1.
- [ ] AC-2: `ChangeDocument` tăng version + gửi `DidChange` với full content (full sync).
- [ ] AC-3: `CloseDocument` gửi `DidClose`, xóa khỏi tracking map.
- [ ] AC-4: Double-open idempotent — không gửi DidOpen lần 2.
- [ ] AC-5: `DiagnosticsCollector` lưu đúng theo URI; `GetAllErrors` filter bỏ warnings.
- [ ] AC-6: `HasErrors()` false khi clean; `WaitForDiagnostics` succeed khi có diagnostics, timeout khi không.
- [ ] AC-7: `FormatDiagnosticsForAgent` đúng format `file:line:col error: message`.
- [ ] AC-8: 10 tests green:
  - `TestDocSyncOpenSendsDidOpen`
  - `TestDocSyncChangeIncrementsVersion`
  - `TestDocSyncCloseSendsDidClose`
  - `TestDocSyncDoubleOpenIsIdempotent`
  - `TestDiagnosticsCollectorStoresByURI`
  - `TestDiagnosticsCollectorGetAllErrorsFiltersWarnings`
  - `TestDiagnosticsCollectorHasErrorsReturnsFalseWhenClean`
  - `TestDiagnosticsCollectorWaitForDiagnosticsTimesOut`
  - `TestDiagnosticsCollectorWaitForDiagnosticsSucceeds`
  - `TestFormatDiagnosticsForAgentOutput`

## 7. Out of Scope

- Runner hook (Task-358).
- Context source (Task-359).
- Kotlin/Gradle extension (Task-360/361).
- Goto-definition/find-references (CP-63 P-6 optional `LSPNavigationSource` — defer).

## 8. Completion Notes

- result: Implemented as specified. `ChangeDocument` auto-opens unknown documents (guessing languageId from extension) so the hook needs a single call; `WaitForDiagnostics` uses generation-polling (empty publishes count as arrival, matching clean-file server behavior); `GetAllErrors` sorts deterministically; positions render 1-based. All 10 test signatures green.
- follow-ups: none.
- upstream docs updated: CP-63 P-4 marked done.