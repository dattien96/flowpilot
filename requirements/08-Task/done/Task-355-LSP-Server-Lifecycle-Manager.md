# Task-355: LSP Server Lifecycle Manager

## Metadata

- Document ID: `Task-355`
- Title: `LSP Server Lifecycle Manager`
- Phase: `task`
- Status: `done`
- Owner: `Claude Sonnet MAX`
- Reviewers: `Claude agent review`
- Created: `2026-09-15`
- Last Updated: `2026-09-15`
- Parent Documents: [CP-63 P-2](../../07-Coding-Plan/done/CP-63-IDE-Grade-LSP-Runtime.md)
- Child Documents: `None`
- Related Documents: [SD-17](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md), [Task-354](../done/Task-354-LSP-JSONRPC-Client-Core.md)
- Replaces: `None`
- Tags: `lsp, language-server, lifecycle, process-management, code-intelligence`
- Feature Keys: `lsp-runtime`

## AI Quick View

### Summary

- Slice P-2 của CP-63: `ServerManager` quản lý OS process của LSP server (spawn, stop, restart, health check) và pipe stdin/stdout vào `Client` từ Task-354.
- Chính sách graceful degradation: crash → auto-restart tối đa 1 lần/session, sau đó disable với warning; timeout → skip.
- `WaitReady(ctx, timeout)` block tới khi Initialize handshake hoàn tất — hook chờ này được P-5 dùng để đảm bảo server sẵn sàng trước khi query diagnostics.
- Không user-visible change; nền tảng cho P-3 → P-8.

### Current Ask

- Implement P-2 theo CP-63 §4: `server_manager.go` + `server_manager_test.go`. 8 test signatures phải xanh.

### Key Decisions

- `T-1` Mỗi workspace chỉ 1 LSP server instance per language (CP-63 P-2 decision), managed bởi `ServerManager`.
- `T-2` `Start(ctx, binary string, args []string, workspaceRoot string)` — pipes stdin/stdout tới `Client` (từ Task-354), stderr capture vào structured log (`lsp.stderr`).
- `T-3` Health check goroutine: ping mỗi 30s (re-initialize hoặc `$/alive` nếu server hỗ trợ), crash → auto-restart tối đa 1 lần/session → nếu crash lần 2, disable cho session với warning `lsp.disabled`.
- `T-4` `Restart()` recreate cả process lẫn `Client` (channel map cũ của client phải được drain/discard an toàn).
- `T-5` `WaitReady(ctx, timeout)` — block tới khi Initialize handshake thành công hoặc timeout; timeout trả error để caller (P-5) skip gracefully.

### Constraints

- Additive tests only — không edit pre-existing tests.
- Graceful degradation bắt buộc: mọi lỗi process đều phải log + cho phép Runner tiếp tục, không block.
- LSP server binary do user/auto-install cài — `ServerManager` không auto-install, chỉ check PATH (P-3 chịu trách nhiệm lookup).
- GitNexus impact analysis trước mỗi symbol edit.

### Open Questions

- None.

### Source Refs

- CP-63 P-2 (§4), test signatures 1–8.
- CP-63 §3.4 graceful degradation table.

## 1. Goal

`ServerManager` có thể spawn một LSP server binary bất kỳ (mock trong unit test, `gopls` thật trong integration), giữ nó sống với health monitoring, auto-restart đúng 1 lần khi crash, và expose `Client()` để toàn bộ LSP pipeline (P-4/P-5) dùng chung.

## 2. Parent Links

- coding plan: `CP-63-IDE-Grade-LSP-Runtime.md` P-2
- tech design: `SD-17-Context-And-Regression-Engine.md`
- system spec: `SS-14-Code-Context-And-Regression-Safety.md`
- specific upstream ids: Task-354 (Client)

## 3. Trigger

Task-354 cung cấp Client nhưng chưa có ai spawn/giữ process LSP server. Không có ServerManager thì không thể duy trì phiên LSP dài hạn trong Runner, và mọi crash server sẽ giết toàn bộ diagnostics pipeline.

## 4. Exact Change

- `T-1` **`internal/lsp/server_manager.go`** (new): `ServerManager` struct — methods `Start(ctx, binary string, args []string, workspaceRoot string) error`, `Stop() error`, `Restart() error`, `IsRunning() bool`, `Client() *Client`. Sử dụng `os/exec.Cmd` với `StdinPipe`/`StdoutPipe`, stderr → `log`.
- `T-2` **`internal/lsp/server_manager.go`**: Health check goroutine — ping 30s/lần, phát hiện process exit, auto-restart 1 lần/session, lần 2 → disable + warning.
- `T-3` **`internal/lsp/server_manager.go`**: `WaitReady(ctx context.Context, timeout time.Duration) error` — chờ Initialize handshake từ `Client`, timeout → error.
- `T-4` **`internal/lsp/server_manager.go`**: Structured log events `lsp.start`, `lsp.crash`, `lsp.restart`, `lsp.disabled` (theo CP-63 §8 monitoring).
- `T-5` **`internal/lsp/server_manager_test.go`** (new): mock binary (script/small helper process) cho unit tests; 8 test signatures dưới đây.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/lsp/server_manager.go` (new)
  - `apps/local-runner/internal/lsp/server_manager_test.go` (new)
- modules: `lsp`
- routes: none
- tables: none

## 6. Acceptance Check

- [ ] AC-1: `ServerManager` start/stop một process thật (mock binary), `IsRunning()` phản ánh đúng state.
- [ ] AC-2: `Stop()` kill process sạch (không zombie), giải phóng pipes.
- [ ] AC-3: `Restart()` recreate process + Client mới.
- [ ] AC-4: Crash → auto-restart đúng 1 lần → crash lần 2 disable với warning.
- [ ] AC-5: `WaitReady` block tới khi Initialize xong; timeout khi server không respond.
- [ ] AC-6: 8 tests green:
  - `TestServerManagerStartSpawnsProcess`
  - `TestServerManagerStopKillsProcess`
  - `TestServerManagerRestartRecreatesClient`
  - `TestServerManagerAutoRestartOnCrash`
  - `TestServerManagerMaxOneRestartPerSession`
  - `TestServerManagerWaitReadyTimesOut`
  - `TestServerManagerWaitReadySucceeds`
  - `TestServerManagerIsRunningReflectsState`

## 7. Out of Scope

- Platform → binary mapping (P-3).
- Document sync (P-4).
- Runner hook (P-5).
- Binary auto-install (R-1 mitigation thuộc `flowpilot doctor`, CP sau).

## 8. Completion Notes

- result: Implemented as specified plus two additive fields discovered during implementation: `Env` (extra process env, also used by tests to drive the helper-process mock) and `InitOptions` (sent as `initializationOptions` by WaitReady, wired in Task-360). Health loop + single crash-restart budget + disable verified with cross-platform helper-process mocks. All 8 test signatures green. Two test-hygiene fixes during implementation (TempDir-cleanup ordering on Windows, deadlock from re-locking in startProcessLocked) — production logic was correct.
- follow-ups: none.
- upstream docs updated: CP-63 P-2 marked done.