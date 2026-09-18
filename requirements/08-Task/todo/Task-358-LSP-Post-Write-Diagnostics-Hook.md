# Task-358: LSP Post-Write Diagnostics Hook in Runner

## Metadata

- Document ID: `Task-358`
- Title: `LSP Post-Write Diagnostics Hook in Runner`
- Phase: `task`
- Status: `done`
- Owner: `Claude Sonnet MAX`
- Reviewers: `Claude agent review`
- Created: `2026-09-15`
- Last Updated: `2026-09-15`
- Parent Documents: [CP-63 P-5](../../07-Coding-Plan/todo/CP-63-IDE-Grade-LSP-Runtime.md)
- Child Documents: `None`
- Related Documents: [SD-17](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md), [CP-55](../../07-Coding-Plan/done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md), [Task-357](../todo/Task-357-LSP-Document-Sync-And-Diagnostics-Collection.md), [Task-355](../todo/Task-355-LSP-Server-Lifecycle-Manager.md)
- Replaces: `None`
- Tags: `lsp, runner, diagnostics-hook, file-write, auto-reprompt, code-intelligence`
- Feature Keys: `lsp-runtime`

## AI Quick View

### Summary

- Slice P-5 của CP-63: user-visible change đầu tiên — sau mỗi file write của Agent, Runner query LSP diagnostics; có compiler error → đưa message vào conversation trước turn kế tiếp (auto-reprompt), clean → proceed.
- `PostWriteDiagnosticsHook` trong `internal/lsp/runner_hook.go`: `ChangeDocument` → `WaitForDiagnostics(5s)` → `HasErrors()` → formatted string hoặc nil.
- Điểm hỏi LSP đã chốt sau discovery (2026-09-15): **cuối turn, dùng danh sách file đã gom sẵn** (`pendingGateChangedFiles` → `finalizeInput.ChangedFiles`), không hỏi lúc event vừa tới. Lý do: `emitLocked` chạy trong lúc giữ lock nên không được block; và Runner không chen tin nhắn vào giữa turn đang chạy được — diagnostics đằng nào cũng chỉ tới tay Agent ở turn sau.
- Điểm đưa tin nhắn cho Agent đã chốt: **dùng sẵn bộ máy reprompt của gate** — set 3 field `pendingGateRepromptPrompt/StepID/Gen` trên run, bộ máy settle tự mở turn mới mang prompt đó (`scheduleSettleAfterGateBlock` → `scheduleRootGateRepromptOrPark` cho run gốc / `startTurnClearingIntent` cho run con → `startTurn` với `TurnInput{StepID, Prompt}`). Không viết cơ chế mở turn mới.
- Graceful degradation: `ServerManager.IsRunning() == false` → skip hook hoàn toàn; timeout 5s → skip; mọi lỗi không block Runner.

### Current Ask

- Implement P-5 theo CP-63 §4: `runner_hook.go` + điểm nối vào Runner (đã chốt sau discovery — xem Key Decisions). 7 test signatures phải xanh. **Chạy GitNexus impact analysis trước khi chạm bất kỳ symbol runner nào.**

### Key Decisions

- `T-1` Hook sequence cố định: (1) `DocSync.ChangeDocument(uri, newContent)`, (2) `Collector.WaitForDiagnostics(5s)`, (3) `HasErrors()` → trả formatted diagnostic string, (4) clean → nil.
- `T-2` URI tính từ workspaceRoot + relative path (filepath → `file://` URI), chỉ track file trong workspace (bỏ qua ngoài workspace).
- `T-3` Điểm hỏi LSP: **cuối turn, trong post-turn gate path** (`runFlowGateAtEpoch` / `runChildArtifactOutputGateAtEpoch` trong `gate_hook.go`), dùng `fin.ChangedFiles` đã gom sẵn — KHÔNG hỏi trong `emitLocked` (`interactive_service.go:5356`, chạy trong lúc giữ lock `s.mu`, cấm block). File thay đổi tới gate qua chuỗi: event mapper (`claude/codex/grok/opencode_event_mapper.go` → `EventFileChanged`) → `emitLocked` append vào `rs.events` (`interactive_service.go:5386`) → `markPendingFlowGateSettleLocked` gom thành `pendingGateChangedFiles` (`interactive_service.go:5019-5028`) → `finalizeInput.ChangedFiles` (`interactive_service.go:4730-4735`).
- `T-4` Điểm đưa tin nhắn cho Agent: **set 3 field có sẵn** `rs.pendingGateRepromptPrompt` (nội dung báo lỗi) + `pendingGateRepromptStepID` + `pendingGateRepromptGen++` (khai báo `interactive_service.go:556-560`), rồi return block để bộ máy settle tự mở turn mới — run gốc qua `scheduleRootGateRepromptOrPark` (`hub_stall.go:275`), run con qua `startTurnClearingIntent` (`interactive_resume.go:1692` → `startTurn` với `TurnInput{StepID, Prompt}` dòng 1713). Mẫu tham chiếu: `gate_hook.go:1315-1319` (nhánh auto-fix của gate). Không viết cơ chế mở turn mới, không sửa logic đánh giá của gate.
- `T-5` Hook chỉ chạy cho file có extension thuộc `PlatformLSPConfig.FileExtensions` (từ Task-356), bỏ qua doc/audit file (dùng `flowgate.IsDocOrAuditFile` như gate).
- `T-6` Multi-server (kết quả Q3): `ServerManager` giữ map key **`workspace + language`** (không chỉ workspace — 1 workspace đa ngôn ngữ chạy nhiều server), chọn server theo: file → workspace (khớp prefix dài nhất) → ngôn ngữ (đuôi file) → client; `GetOrStart` lazy-start ở lần ghi đầu tiên, shutdown sau 5 min idle (CP-63 §9 R-3). Crash 1 server không ảnh hưởng server khác (process độc lập, xem policy restart P-2 ở Task-355).

### Constraints

- Graceful degradation bắt buộc (CP-63 §3.4): binary thiếu/crash/timeout/unsupported platform → log warning, Runner chạy y hệt trước CP-63.
- Additive tests only — không edit pre-existing tests.
- Không block Agent turn khi LSP chậm — timeout 5s là trần cứng; tuyệt đối không query đồng bộ trong `emitLocked` (đang giữ `s.mu`).
- Không chen tin nhắn vào giữa turn đang chạy (mô hình 1 turn tại 1 thời điểm) — diagnostics chỉ đi đường reprompt cuối turn.
- Không sửa logic đánh giá của gate (`gate_hook.go`) — LSP chỉ set 3 field reprompt có sẵn rồi return block.
- Flow mode: required (auto-reprompt); Normal chat: best-effort (log warning, không block) — theo CP-63 §1.1.
- GitNexus impact analysis trước mỗi symbol edit (`runFlowGateAtEpoch`, `runChildArtifactOutputGateAtEpoch`, `pendingGateRepromptPrompt`).

### Open Questions

- None — discovery hoàn tất 2026-09-15 (điểm hỏi cuối turn + điểm đưa tin nhắn qua reprompt có sẵn + multi-server key `workspace + language` đã chốt ở trên).

### Source Refs

- CP-63 P-5 (§4), test signatures 1–7; §3.2 target behavior; §3.4 degradation table.
- Event intake: `apps/local-runner/internal/runner/{claude,codex,grok,opencode}_event_mapper.go` → `EventFileChanged`; `interactive_service.go:5356` (`emitLocked`), `:5386` (append `rs.events`), `:5019-5028` (`markPendingFlowGateSettleLocked` gom `pendingGateChangedFiles`), `:4730-4735` (`finalizeInput.ChangedFiles`), `:556-560` (3 field reprompt).
- Reprompt delivery: `dispatch_settle_wire.go:186` (`scheduleSettleAfterGateBlock`), `hub_stall.go:275` (`scheduleRootGateRepromptOrPark`), `interactive_resume.go:1692` (`startTurnClearingIntent` → `startTurn` dòng 1713); mẫu set field: `gate_hook.go:1315-1319`.

## 1. Goal

Sau slice này, khi Agent sửa file trên workspace có LSP server hoạt động, compiler errors được phát hiện trong <5s và tự động đưa vào conversation để Agent sửa trước khi chạy test — cắt giảm vòng lặp build 5–120s.

## 2. Parent Links

- coding plan: `CP-63-IDE-Grade-LSP-Runtime.md` P-5
- tech design: `SD-17-Context-And-Regression-Engine.md`
- system spec: `SS-14-Code-Context-And-Regression-Safety.md`
- specific upstream ids: Task-357 (DocSync/Collector API), Task-355 (IsRunning)

## 3. Trigger

Toàn bộ LSP infra (Task-354→357) đã xong nhưng chưa kết nối vào Runner — Agent vẫn phải dựa vào full build để biết lỗi. P-5 là slice đầu tiên mang lại giá trị user-visible.

## 4. Exact Change

- `T-1` **`internal/lsp/runner_hook.go`** (new): `PostWriteDiagnosticsHook` struct — fields: `Manager *ServerManager`, `DocSync *DocumentSyncManager`, `Collector *DiagnosticsCollector`, `WorkspaceRoot string`. Method `AfterFileWrite(relPath string, newContent string) (string, error)` — sequence theo T-1; trả `""` khi clean; `diagnosticString` khi có error; error nếu timeout/skip.
- `T-2` **`internal/lsp/runner_hook.go`**: `ShouldRun(relPath string) bool` — check `Manager.IsRunning()` + file extension thuộc `PlatformLSPConfig.FileExtensions` (từ Task-356).
- `T-3` **`internal/runner/gate_hook.go`** (điểm nối, trong `runFlowGateAtEpoch` / `runChildArtifactOutputGateAtEpoch`): sau khi có `fin.ChangedFiles`, gọi hook cho từng file code; nếu trả diagnostic string → set 3 field reprompt (`pendingGateRepromptPrompt/StepID/Gen++`) rồi return block — KHÔNG sửa logic `Evaluate`/`Enforce` của gate.
- `T-4` **`internal/lsp/runner_hook_test.go`** (new) + runner-side test (file mới): 7 test signatures dưới đây.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/lsp/runner_hook.go` (new)
  - `apps/local-runner/internal/lsp/runner_hook_test.go` (new)
  - `apps/local-runner/internal/runner/gate_hook.go` (điểm nối trong `runFlowGateAtEpoch` / `runChildArtifactOutputGateAtEpoch` — chỉ thêm gọi hook + set reprompt fields, không sửa gate logic)
  - `apps/local-runner/internal/runner/` (test mới cho injection)
- modules: `lsp` (new), `runner` (modified: `gate_hook.go` + test mới)
- routes: none
- tables: none

## 6. Acceptance Check

- [ ] AC-1: File write → LSP diagnostics → error injection chạy end-to-end với mock server.
- [ ] AC-2: Hook trả nil khi clean, trả formatted diagnostics khi có error (multi-error format).
- [ ] AC-3: `ServerManager.IsRunning() == false` → hook skip, không lỗi.
- [ ] AC-4: Timeout 5s → skip gracefully, không block turn.
- [ ] AC-5: Runner không crash khi LSP unavailable (degradation verified).
- [ ] AC-6: Pre-existing runner tests xanh (không edit).
- [ ] AC-7: 7 tests green:
  - `TestPostWriteHookReturnsNilWhenClean`
  - `TestPostWriteHookReturnsDiagnosticsOnError`
  - `TestPostWriteHookSkipsWhenLSPNotRunning`
  - `TestPostWriteHookTimesOutGracefully`
  - `TestPostWriteHookFormatsMultipleErrors`
  - `TestRunnerInjectsLSPDiagnosticsAfterWrite`
  - `TestRunnerSkipsLSPHookWhenNoServer`

## 7. Out of Scope

- Context source registry wiring (Task-359).
- Kotlin/Gradle (Task-360/361).
- Navigation (goto-definition/find-references).
- Sửa logic đánh giá của gate (`Evaluate`/`Enforce` trong `gate_hook.go`) — LSP chỉ mượn 3 field reprompt có sẵn, gate logic giữ nguyên.

## 8. Completion Notes

- result: Implemented per the discovery update (see Key Decisions T-3–T-6): gate-time query via `fin.ChangedFiles` in both gate allow-paths (4-line insertions, nil-checker = zero behavior change), delivery via existing reprompt fields, multi-server `ServerSet` keyed by workspace+language with lazy-start/idle-reap/cooldown. GitNexus impact on both gate functions: LOW. Gate regression batch re-run: only pre-existing HEAD failures remain (verified via stash); one load-flake (`TestProviderAdaptersReceiveEquivalentFrozenContractPayload`) passed on re-run and in isolation. All 7 test signatures green (+3 ServerSet routing tests). One self-caught brace bug during gate insertion fixed before test (verified via build).
- follow-ups: none for this slice (Gradle-fallback call-site decision deferred to post-P-8 work per Task-361).
- upstream docs updated: CP-63 P-5 marked done.