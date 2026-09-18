# Task-359: LSP Diagnostics as Context Source (CP-44 Registry)

## Metadata

- Document ID: `Task-359`
- Title: `LSP Diagnostics as Context Source (CP-44 Registry)`
- Phase: `task`
- Status: `done`
- Owner: `Claude Sonnet MAX`
- Reviewers: `Claude agent review`
- Created: `2026-09-15`
- Last Updated: `2026-09-15`
- Parent Documents: [CP-63 P-6](../../07-Coding-Plan/todo/CP-63-IDE-Grade-LSP-Runtime.md)
- Child Documents: `None`
- Related Documents: [SD-17](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md), [CP-44](../../07-Coding-Plan/done/CP-44-Pluggable-Context-Source-Registry.md), [Task-358](../todo/Task-358-LSP-Post-Write-Diagnostics-Hook.md)
- Replaces: `None`
- Tags: `lsp, context-source, cp-44, diagnostics, prompt-packing, code-intelligence`
- Feature Keys: `lsp-runtime`

## AI Quick View

### Summary

- Slice P-6 của CP-63: LSP diagnostics trở thành context source mới cắm vào CP-44 registry — Agent thấy "## Current Compiler Diagnostics (LSP)" trong context window.
- `LSPDiagnosticsSource` implement `ContextSource` interface (`context_source_registry.go:33`): `ID()` = `lsp.diagnostics`, `Deterministic()` = true, `Fetch` render error list từ `DiagnosticsCollector`.
- Không hardcode vào runner loop — đăng ký qua `registerBuiltinContextSources` pattern; nếu LSP không available → render "No compiler errors detected." hoặc empty section (graceful).
- `LSPNavigationSource` (goto-definition/find-references) là optional, defer — không nằm trong slice này.

### Current Ask

- Implement P-6 theo CP-63 §4: `context_source.go` + wire vào CP-44 registry + `context_source_test.go`. 5 test signatures phải xanh.

### Key Decisions

- `T-1` `LSPDiagnosticsSource` implement `ContextSource`: `ID() = "lsp.diagnostics"`, `Priority()` đặt sau `source.excerpt` (thứ tự pack: errors hiển thị cuối context package), `Deterministic() = true` (dữ liệu từ collector — deterministic theo state hiện tại, không similarity search).
- `T-2` Rendered section: `## Current Compiler Diagnostics (LSP)` + error list (reuse `FormatDiagnosticsForAgent` từ Task-357) hoặc `No compiler errors detected.` khi collector rỗng.
- `T-3` Source đọc state từ `DiagnosticsCollector` singleton (share với Task-358 hook) — không tự spawn server, không tự query; nếu không có collector/server → `Fetch` trả section với body `No compiler errors detected.` (không error — degrade như warning-level).
- `T-4` Registry wiring: thêm `mustRegisterContextSource(r, &lspDiagnosticsSource{...})` trong `registerBuiltinContextSources` (`context_sources_builtin.go:166`) — nhưng KHÔNG thêm `lsp.diagnostics` vào `defaultContextSourceIDs` mặc định (chỉ khi flow/server opt-in qua CP-44 config — tránh thay đổi output prompt của mọi flow hiện hữu).
- `T-5` FlowContextSection fields: `SourceType = "lsp.diagnostics"`, `SourceRef` = `lsp://<platform>/<uri>` per error (traceability contract CP-41 T-3), `Body` = rendered section.

### Constraints

- Additive tests only — không edit pre-existing tests (đặc biệt `context_package_sections_test.go` assert 3 built-ins + custom; không được phá).
- Không thay đổi output của default flows — `lsp.diagnostics` chỉ active khi được enable (registry lookup là fail-fast, nhưng default set không chứa id này).
- Graceful degradation: không server → empty body, không warning spam.
- GitNexus impact analysis trước mỗi symbol edit (`registerBuiltinContextSources`, `mustRegisterContextSource`).

### Open Questions

- `LSPNavigationSource` (goto-definition/find-references) — deferred (CP-63 P-6 optional).

### Source Refs

- CP-63 P-6 (§4), test signatures 1–5.
- `apps/local-runner/internal/runner/context_source_registry.go` (interface + Register), `context_sources_builtin.go:166` (registerBuiltinContextSources).
- CP-44 registry contract.

## 1. Goal

Agent trong mọi flow có LSP enabled sẽ luôn thấy compiler diagnostics hiện tại trong context package — không cần chờ test command, biết ngay file nào đang lỗi khi bắt đầu turn.

## 2. Parent Links

- coding plan: `CP-63-IDE-Grade-LSP-Runtime.md` P-6
- tech design: `SD-17-Context-And-Regression-Engine.md`
- system spec: `SS-14-Code-Context-And-Regression-Safety.md`
- specific upstream ids: CP-44 (registry), Task-357 (collector API), Task-358 (shared collector state)

## 3. Trigger

Hook (Task-358) đã inject diagnostics sau write, nhưng khi turn mới bắt đầu (đặc biệt flow coding node), Agent không thấy trạng thái lỗi hiện tại trừ khi nó chạy tool đọc. Context source làm cho errors luôn hiển thị trong prompt — theo đúng kiến trúc CP-44 pluggable.

## 4. Exact Change

- `T-1` **`internal/lsp/context_source.go`** (new): `LSPDiagnosticsSource` struct — implement `ContextSource` (ID, Priority, Deterministic, Fetch).
- `T-2` **`internal/lsp/context_source.go`**: `Fetch(ctx, hints)` — đọc `DiagnosticsCollector.GetAllErrors()`, render section `## Current Compiler Diagnostics (LSP)`; collector rỗng → `No compiler errors detected.`
- `T-3` **`internal/runner/context_sources_builtin.go`** (wiring): đăng ký `lspDiagnosticsSource` trong `registerBuiltinContextSources` khi LSP manager available (lazy: registry giữ source; source nội bộ check server availability khi Fetch).
- `T-4` **`internal/lsp/context_source_test.go`** (new) + runner-side registry test (file mới): 5 test signatures dưới đây.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/lsp/context_source.go` (new)
  - `apps/local-runner/internal/lsp/context_source_test.go` (new)
  - `apps/local-runner/internal/runner/context_sources_builtin.go` (wiring)
  - `apps/local-runner/internal/runner/` (registry test mới)
- modules: `lsp` (new), `runner` (modified)
- routes: none
- tables: none

## 6. Acceptance Check

- [ ] AC-1: `LSPDiagnosticsSource` render danh sách errors khi collector có error.
- [ ] AC-2: Render `No compiler errors detected.` khi collector clean.
- [ ] AC-3: `ID()` = `lsp.diagnostics`, slot key đúng.
- [ ] AC-4: Không server → empty/clean body, không panic, không error.
- [ ] AC-5: Source đăng ký được trong CP-44 registry (`DefaultContextSourceRegistry().Resolve("lsp.diagnostics")` không lỗi) và KHÔNG nằm trong `defaultContextSourceIDs` mặc định.
- [ ] AC-6: Pre-existing context source tests xanh (không edit).
- [ ] AC-7: 5 tests green:
  - `TestLSPDiagnosticsSourceRendersErrors`
  - `TestLSPDiagnosticsSourceRendersCleanMessage`
  - `TestLSPDiagnosticsSourceSlotKey`
  - `TestLSPDiagnosticsSourceSkipsWhenNoServer`
  - `TestLSPDiagnosticsSourceRegisteredInCP44Registry`

## 7. Out of Scope

- `LSPNavigationSource` (goto-definition/find-references — deferred).
- Kotlin/Gradle (Task-360/361).
- Thêm `lsp.diagnostics` vào defaultContextSourceIDs mặc định (giữ opt-in).

## 8. Completion Notes

- result: Implemented with one structural deviation forced by the import graph: the `ContextSource` implementation lives in `runner/context_source_lsp.go` (thin adapter) while `lsp/context_source.go` holds the logic (`Source`, render, `ServerSet.GetAllErrors` aggregation) — `lsp` cannot import `runner` (cycle). Registered opt-in only (priority 5, NOT in `defaultContextSourceIDs`); all pre-existing context-source tests green. All 5 test signatures green (+1 aggregation test).
- follow-ups: `LSPNavigationSource` (goto-definition/find-references) remains deferred per CP-63.
- upstream docs updated: CP-63 P-6 marked done.