# Task-354: LSP JSON-RPC Client Core

## Metadata

- Document ID: `Task-354`
- Title: `LSP JSON-RPC Client Core`
- Phase: `task`
- Status: `done`
- Owner: `Claude Sonnet MAX`
- Reviewers: `Claude agent review`
- Created: `2026-09-15`
- Last Updated: `2026-09-15`
- Parent Documents: [CP-63 P-1](../../07-Coding-Plan/todo/CP-63-IDE-Grade-LSP-Runtime.md)
- Child Documents: `None`
- Related Documents: [SD-17](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md), [CP-55](../../07-Coding-Plan/done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md)
- Replaces: `None`
- Tags: `lsp, json-rpc, language-server, diagnostics, code-intelligence`
- Feature Keys: `lsp-runtime`

## AI Quick View

### Summary

- Slice đầu tiên của CP-63: build LSP client core — JSON-RPC over stdio, giống OMP (Oh-My-Pi), cho phép FlowPilot nhận live compiler diagnostics từ language server.
- `internal/lsp/protocol.go` định nghĩa các LSP message types theo LSP spec; `internal/lsp/client.go` implement client với framing `Content-Length: N\r\n\r\n`.
- Client thread-safe: goroutine đọc stdout, demux responses/notifications, request/response qua channel map, `publishDiagnostics` dispatch tới callback đăng ký.
- Đây là nền móng cho P-2 → P-8; slice này không có user-visible change.

### Current Ask

- Implement P-1 theo CP-63 §4: `protocol.go` + `client.go` + `client_test.go`. 10 test signatures cho sẵn phải xanh.

### Key Decisions

- `T-1` JSON-RPC 2.0 over stdio, không TCP/WebSocket (CP-63 P-1 decision — đơn giản, cross-platform, không port management).
- `T-2` `NewClient(stdin io.Writer, stdout io.Reader)` — Runner chủ động pipe từ `ServerManager` (P-2), client không tự spawn process.
- `T-3` Internal read goroutine demux: message có `id` → response (match channel map), message có `method` → notification (dispatch `handleNotification`).
- `T-4` `textDocument/publishDiagnostics` notification được dispatch tới callback `OnDiagnostics` đăng ký lúc NewClient — collector (P-4) sẽ đăng ký callback này.

### Constraints

- Additive tests only — không edit pre-existing tests.
- Không block Agent khi LSP fail — mọi lỗi đọc/parse message chỉ log, không panic.
- GitNexus impact analysis trước mỗi symbol edit.
- Package mới `apps/local-runner/internal/lsp` — không được import ngược từ `tui/` hoặc tạo cycle dependency.

### Open Questions

- None.

### Source Refs

- CP-63 P-1 (§4), test signatures 1–10.
- LSP spec: `textDocument/didOpen`, `didChange`, `publishDiagnostics`, `Content-Length` framing.

## 1. Goal

Một LSP client hoạt động được: thực hiện Initialize handshake với mock stdio server, gửi `DidOpen`/`DidChange` notification đúng LSP spec, nhận `publishDiagnostics` notification qua callback, và xử lý đúng edge cases (malformed message, concurrent requests) không deadlock.

## 2. Parent Links

- coding plan: `CP-63-IDE-Grade-LSP-Runtime.md` P-1
- tech design: `SD-17-Context-And-Regression-Engine.md`
- system spec: `SS-14-Code-Context-And-Regression-Safety.md`

## 3. Trigger

FlowPilot không có compiler feedback tức thì sau khi Agent sửa file — Agent phải chạy full build (5–120s) để phát hiện lỗi. CP-63 P-1 là slice đầu tiên xây lớp hạ tầng LSP để rút xuống <200ms.

## 4. Exact Change

- `T-1` **`internal/lsp/protocol.go`** (new): LSP protocol message types — `InitializeParams`, `InitializeResult`, `DidOpenTextDocumentParams`, `DidChangeTextDocumentParams`, `PublishDiagnosticsParams`, `Diagnostic`, `Position`, `Range`, `Location`, `TextDocumentIdentifier`, `TextDocumentItem`, `VersionedTextDocumentIdentifier`, `ContentChangeEvent`. JSON tags theo LSP spec (camelCase field names).
- `T-2` **`internal/lsp/client.go`** (new): `Client` struct với `NewClient(stdin io.Writer, stdout io.Reader)`, methods `Initialize`, `Shutdown`, `DidOpen`, `DidChange`, `DidClose`. Internal goroutine đọc stdout, parse framing `Content-Length: N\r\n\r\n{...}`, demux theo ID/method.
- `T-3` **`internal/lsp/client.go`**: `handleNotification(method string, params json.RawMessage)` — dispatch `textDocument/publishDiagnostics` tới callback đăng ký (`OnDiagnostics func(PublishDiagnosticsParams)`).
- `T-4` **`internal/lsp/client.go`**: Request/response matching qua channel map keyed by message ID, thread-safe với mutex; timeout cho mỗi pending request (5s).
- `T-5` **`internal/lsp/client_test.go`** (new): mock stdio server (in-memory pipe pair) cho 10 test signatures dưới đây.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/lsp/protocol.go` (new)
  - `apps/local-runner/internal/lsp/client.go` (new)
  - `apps/local-runner/internal/lsp/client_test.go` (new)
- modules: `lsp` (new)
- routes: none
- tables: none

## 6. Acceptance Check

- [ ] AC-1: `Client` exchange Initialize handshake với mock stdio server (request → response match).
- [ ] AC-2: `DidOpen`/`DidChange` notification serialize đúng LSP spec (`Content-Length` framing).
- [ ] AC-3: Diagnostics callback fire khi nhận `textDocument/publishDiagnostics`.
- [ ] AC-4: Concurrent requests không deadlock, không mất response.
- [ ] AC-5: Malformed response (sai JSON, sai header, partial read) → log error, không crash.
- [ ] AC-6: `Shutdown` gửi `exit` notification đúng thứ tự.
- [ ] AC-7: 10 tests green:
  - `TestClientSendsInitializeRequest`
  - `TestClientReceivesInitializeResponse`
  - `TestClientSendsDidOpenNotification`
  - `TestClientSendsDidChangeNotification`
  - `TestClientReceivesDiagnosticsNotification`
  - `TestClientHandlesMalformedResponse`
  - `TestClientHandlesConcurrentRequests`
  - `TestClientShutdownSendsExitNotification`
  - `TestProtocolMessageFraming`
  - `TestProtocolParseContentLength`

## 7. Out of Scope

- Process lifecycle (P-2 — ServerManager).
- Document version tracking (P-4 — DocSyncManager).
- Platform detection (P-3).
- Mọi integration vào Runner (P-5/P-6).

## 8. Completion Notes

- result: Implemented as specified. `protocol.go` (LSP 3.17 message types) + `client.go` (JSON-RPC over stdio, background demux goroutine, per-request channels, 5s default timeout, malformed-frame resync). All 10 test signatures green (`go test ./internal/lsp/ -run TestClient|TestProtocol`).
- follow-ups: none.
- upstream docs updated: CP-63 P-1 marked done.