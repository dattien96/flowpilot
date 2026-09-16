# Task-362: LSP Missing-Server Warning (Sidebar + API)

## Metadata

- Document ID: `Task-362`
- Title: `LSP Missing-Server Warning (Sidebar + API)`
- Phase: `task`
- Status: `done`
- Owner: `Claude Sonnet MAX`
- Reviewers: `Claude agent review`
- Created: `2026-09-15`
- Last Updated: `2026-09-15`
- Parent Documents: [CP-63](../../07-Coding-Plan/done/CP-63-IDE-Grade-LSP-Runtime.md) (follow-up to P-5/R-1)
- Child Documents: `None`
- Related Documents: [Task-358](../done/Task-358-LSP-Post-Write-Diagnostics-Hook.md), [CA-869](../../../change-audit/CA-869-CP-63-IDE-Grade-LSP-Runtime-Implementation.md)
- Replaces: `None`
- Tags: `lsp, warning, sidebar, tui, desktop, install-hint`
- Feature Keys: `lsp-runtime`

## AI Quick View

### Summary

- Runner phát hiện project thiếu LSP server binary nhưng trước đây chỉ log (và log mỗi turn — spam). Giờ có warning hiển thị cho user.
- Thứ tự đúng yêu cầu: detect platform → biết project gì → check binary → thiếu thì warning (1 lần trong log, persistent gọn trong sidebar).
- `GET /client/lsp-status?path=` trả `{platform, binary, installed, installHint, warn, notice}` — TUI và desktop dùng chung.
- Sidebar phải TUI hiện tối đa 2 dòng, truncate theo width, nằm trong height budget (steps tự co) — không tràn viewport, không cần scroll.

### Current Ask

- Done. Desktop UI render là follow-up (contract + client method đã sẵn).

### Key Decisions

- `T-1` Status là pure query (không spawn, không warn) — UI poll thoải mái.
- `T-2` Warn-once chỉ áp dụng cho gate log (`ServerSet.warned` per binary); endpoint và sidebar là persistent state (không phải spam).
- `T-3` Desktop tái dùng endpoint qua `getLSPStatus(cwd)` (optional method, mock cũ không vỡ); UI Electron là follow-up riêng.
- `T-4` `InstallHint` sống cùng registry row để data và mapping không lệch.

### Constraints

- Additive tests only — không edit pre-existing tests.
- Sidebar tối đa 2 dòng, truncate theo width.
- Không đụng gate logic, default context sources, modal lifecycle.

### Open Questions

- None.

### Source Refs

- CP-63 §3.4 (graceful degradation), §9 R-1 (doctor/install mitigation — deferred).
- `GET /client/lsp-status`, `lsp.ServerStatus`, `ServerSet.Status`.

## 1. Goal

User mở project thiếu language server thì thấy ngay 1 cảnh báo gọn trong sidebar (kèm câu lệnh cài), thay vì diagnostics im lặng vắng mặt + log spam mỗi turn.

## 2. Parent Links

- coding plan: `CP-63-IDE-Grade-LSP-Runtime.md` (P-5 follow-up, R-1 partial mitigation)
- tech design: `SD-17-Context-And-Regression-Engine.md`
- system spec: `SS-14-Code-Context-And-Regression-Safety.md`
- specific upstream ids: Task-358 (hook), Task-356 (registry)

## 3. Trigger

Review implementation CP-63 phát hiện: máy chưa cài server thì gate log `[lsp] check skipped` mỗi turn (spam), user không biết thiếu gì và cài bằng gì.

## 4. Exact Change

- `T-1` **`internal/lsp/platform_registry.go`**: thêm `InstallHint` cho cả 7 platform.
- `T-2` **`internal/lsp/lsp_status.go`** (new): `ServerStatus` struct + `ServerSet.Status()` pure query.
- `T-3` **`internal/lsp/runner_hook.go`**: `warned` map (warn-once per binary), bỏ log per-turn trong `CheckFiles`, log Start/WaitReady failures (cooldown-gated).
- `T-4` **`internal/runner/lsp_status.go`** (new) + 1 dòng route `GET /client/lsp-status` trong `interactive_handlers.go`.
- `T-5` **`internal/tui/client/lsp_status.go`** (new): `LSPStatus` + `GetLSPStatus`.
- `T-6` **TUI app**: `lspStatus`/`lspStatusPath` fields, `LSPStatusMsg` (stale-drop), `cmdFetchLSPStatus`, trigger ở SessionDefaults firstLoad + ProjectCreated, render `lspSidebarLines` (2 dòng, truncate) sau session header.
- `T-7` **Desktop**: `LSPStatus` type + optional `getLSPStatus(cwd)` trong contract + implementation ở `HttpWsRunnerClient` (UI render là follow-up).

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/lsp/platform_registry.go` (InstallHint)
  - `apps/local-runner/internal/lsp/lsp_status.go` (new)
  - `apps/local-runner/internal/lsp/lsp_status_test.go` (new)
  - `apps/local-runner/internal/lsp/runner_hook.go` (warn-once)
  - `apps/local-runner/internal/runner/lsp_status.go` (new)
  - `apps/local-runner/internal/runner/lsp_status_test.go` (new)
  - `apps/local-runner/internal/runner/interactive_handlers.go` (1 route line)
  - `apps/local-runner/internal/tui/client/lsp_status.go` (new)
  - `apps/local-runner/internal/tui/client/lsp_status_test.go` (new)
  - `apps/local-runner/internal/tui/app/lsp_status.go` (new)
  - `apps/local-runner/internal/tui/app/lsp_status_sidebar_test.go` (new)
  - `apps/local-runner/internal/tui/app/model.go` (fields + msg)
  - `apps/local-runner/internal/tui/app/app.go` (triggers + handler)
  - `apps/local-runner/internal/tui/app/session_panel.go` (render slot)
  - `apps/desktop-flowpilot/src/types/contract.ts` (type + optional method)
  - `apps/desktop-flowpilot/src/client/HttpWsRunnerClient.ts` (method)
- modules: `lsp`, `runner`, `tui/app`, `tui/client`, `desktop`
- routes: `GET /client/lsp-status`
- tables: none

## 6. Acceptance Check

- [x] AC-1: `ServerSet.Status` pure (không spawn, ổn định qua nhiều lần gọi).
- [x] AC-2: Missing binary warn đúng 1 lần trong log dù check nhiều lần.
- [x] AC-3: Endpoint trả đúng shape cho cả 3 case (thiếu path 400 / thiếu binary / đã cài).
- [x] AC-4: Sidebar hiện đúng 2 dòng khi thiếu, ẩn khi đủ/không rõ, truncate đúng width.
- [x] AC-5: Stale response (đổi project) không overwrite state mới.
- [x] AC-6: Pre-existing tests xanh (không edit, trừ 0 file cũ nào về logic — chỉ thêm).
- [x] AC-7: 16 tests green (5 lsp + 3 runner + 2 client + 6 tui/app).

## 7. Out of Scope

- Desktop Electron UI render (contract + client method sẵn, follow-up riêng).
- `flowpilot doctor` command (R-1 full mitigation, CP sau).
- Auto-install server binaries.

## 8. Completion Notes

- result: Implemented as specified. TS typecheck không chạy được (deps desktop chưa cài) — thay đổi TS mirror pattern có sẵn, rủi ro thấp.
- follow-ups: desktop sidebar UI DONE (2026-09-15, CA-871) — `LSPStatusNotice` pinned first in the right stack (CSS ellipsis, max 2 visual rows); `flowpilot doctor` split out to Task-363.
- upstream docs updated: CA-870.
