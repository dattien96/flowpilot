# Task-195: MCP-Backed Context Source Adapter

## Metadata

- Document ID: `Task-195`
- Title: `MCP-Backed Context Source Adapter`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-08`
- Last Updated: `2026-07-08`
- Parent Documents: [CP-44: Pluggable Context Source Registry](../../07-Coding-Plan/todo/CP-44-Pluggable-Context-Source-Registry.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md)
- Child Documents: `None`
- Related Documents: [Task-194: Per-Flow Context Source Binding](Task-194-Per-Flow-Context-Source-Binding.md), [Task-191: Context Source Interface And Registry](Task-191-Context-Source-Interface-And-Registry.md)
- Replaces: `None`
- Tags: `context-regression-engine, flow-mode, context-source, mcp, driver, extensibility`

## AI Quick View

### Summary

- Hiện thực nguồn ngoài đầu tiên: `mcp.driver` — lấy context từ một MCP driver (đã chạy ở Chat mode) qua một adapter cắm vào registry.
- Chứng minh CP-44 mở rộng được ra nguồn không-phải-ledger mà **không sửa struct** `FlowContextPackage`.
- Nguồn MCP vẫn "deterministic" theo nghĩa lookup tường minh theo id/ref, **không** similarity search; degrade-mềm khi MCP down/timeout.

### Current Ask

- Hoàn tất source ngoài đầu tiên cho CP-44: `mcp.driver` đã có adapter seam, timeout, bounded output, SourceRef, và degrade-mềm semantics qua registry tests.

### Key Decisions

- `T-1` Adapter chỉ gọi MCP đã **declare trong pack** (không lệnh/endpoint tùy ý) — ranh giới bảo mật CP-44 `R-3`/`Q-2`.
- `T-2` `Deterministic()=true`: adapter làm lookup theo driver-ref tường minh, không phải semantic search → không vi phạm guard no-vector.
- `T-3` MCP lỗi/timeout → `Fetch` trả section rỗng + warning (qua `Collect`), **không** fail Plan step.
- `T-4` Output bounded (áp cap byte như `source.excerpt`) và luôn mang `SourceRef` = driver id/uri.

### Constraints

- Depends on Task-194 (flow phải khai báo bật `mcp.driver`).
- Không đọc dữ liệu ngoài phạm vi MCP pack-declared / workspace-safe.
- Timeout cứng; không để Plan step treo theo MCP.
- Không thêm vector/embedding.

### Open Questions

- `Q-1` **(RESOLVED → CP-44 `P-10`)** Gọi MCP **đồng bộ + timeout cứng** tại Plan-time; cache là tối ưu về sau, không làm ở task này.
- `Q-2` **(RESOLVED, 2026-07-08)** Nguồn `jira.ticket` sẽ **dùng lại adapter contract của `mcp.driver`** khi làm MCP Jira (task riêng, sau). Task-195 chỉ cần đảm bảo interface đủ tổng quát.

### Source Refs

- `CP-44 P-5`, `DOD-5`
- current code: đường MCP Chat-mode (driver) hiện có; `context_source_registry.go` (Task-191)

## 1. Goal

Cung cấp một nguồn context ngoài (`mcp.driver`) chạy end-to-end trên registry CP-44, làm bằng chứng sống cho tính mở rộng động và đặt nền adapter cho các nguồn MCP sau (Jira ticket, Crashlytics, ...).

## 2. Parent Links

- coding plan: `CP-44`
- tech design: `SD-17`
- system spec: `SS-13`
- specific upstream ids: `CP-44 P-5`, `CP-44 DOD-5`, `CP-44 DOD-6`

## 3. Trigger

CP-44 chỉ thực sự chứng minh "add context type mà không đổi structure" khi có ít nhất một nguồn không-phải-ledger. Driver MCP đã hoạt động ở Chat mode là ứng viên tự nhiên để đưa vào Flow Mode qua registry.

## 4. Exact Change

- `T-1` Viết `mcpDriverSource` implement `ContextSource` (file `context_source_mcp.go`):
  - `ID()="mcp.driver"`, `Priority()` đặt cạnh source excerpt, `Deterministic()=true`.
  - `Fetch` đọc driver-ref từ `hints`/pack config, gọi đường MCP Chat-mode, nhận nội dung, cắt bounded, gắn `SourceRef`.
- `T-2` Timeout qua `ctx` truyền vào `Fetch`; MCP lỗi/timeout → trả `FlowContextSection{}` + error để `Collect` chuyển thành warning.
- `T-3` Đăng ký `mcp.driver` vào default registry nhưng **không** thêm vào `defaultSourceIDs` — chỉ chạy khi flow khai báo (Task-194).
- `T-4` Cập nhật một flow (hoặc clone rag-harness) khai báo `sources: [..., mcp.driver]` để test/demo.

## 5. Touched Areas

- files:
  - mới: `apps/local-runner/internal/runner/context_source_mcp.go`
  - mới: `apps/local-runner/internal/runner/context_source_mcp_test.go`
  - `apps/local-runner/internal/runner/context_source_registry.go` (đăng ký)
  - pack flow demo khai báo `mcp.driver`
- modules:
  - runner context assembly, MCP integration
- routes: none
- tables: none

## 6. Acceptance Check

- Flow bật `mcp.driver` → package có section `mcp.driver` bounded + SourceRef; xuất hiện trong prompt Coding.
- MCP down/timeout → warning trong package, Plan step vẫn hoàn tất, không treo/crash.
- Không sửa struct `FlowContextPackage` để thêm nguồn này.
- Guard no-vector vẫn xanh.

### 6.1 Test Items

- `TestMcpDriverSourceProducesBoundedSectionWithSourceRef` (fake MCP)
- `TestMcpDriverSourceDegradesOnMcpTimeout`
- `TestMcpDriverSourceNotInDefaultSet`
- `TestFlowContextPackageStillHasNoVectorDependencyWithMcpSource`

### 6.2 Definition of Done

- [x] `DOD-1` `mcp.driver` source chạy end-to-end với fake adapter, bounded + SourceRef.
- [x] `DOD-2` Timeout/lỗi MCP → degrade-mềm, không fail Plan.
- [x] `DOD-3` Thêm nguồn này không sửa struct package (chứng minh CP-44 `DOD-7` sơ khởi).
- [x] `DOD-4` Guard no-vector giữ nguyên.

## 7. Out of Scope

- Nguồn `jira.ticket` thật (cần MCP Jira — task riêng sau).
- Cache kết quả MCP.
- Canonical Head source (thuộc CP-43).

## 8. Completion Notes

- result: `Done` — `mcp.driver` is registered on the default registry, remains opt-in via flow binding, and is covered by fake-adapter tests for bounded content, timeout/error degradation, and no-vector invariants.
- follow-ups: MCP Jira source; production adapter wiring to the real MCP path can evolve behind the same `MCPDriverAdapter` seam; CP-43 cắm Canonical Head cùng cơ chế.
- upstream docs updated: `CP-44` DOD/progress updated to reflect Task-195 shipped.
