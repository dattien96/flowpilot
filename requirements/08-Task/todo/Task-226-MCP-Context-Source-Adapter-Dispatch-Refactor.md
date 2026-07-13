# Task-226: MCP Context-Source Adapter Dispatch Refactor

## Metadata

- Document ID: `Task-226`
- Title: `MCP Context-Source Adapter Dispatch Refactor`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-13`
- Last Updated: `2026-07-13`
- Parent Documents: [CP-05-06: Jira MCP As A Context Artifact Source](../../07-Coding-Plan/todo/CP-05-06-Jira-MCP.md) (`P-5`), [SD-22: Pluggable Context Source Registry](../../06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md), [SD-23: Generic Artifact Framework](../../06-System-Tech-Design/SD-23-Generic-Artifact-Framework.md)
- Child Documents: `None`
- Related Documents: [CP-05-04: Firebase MCP](../../07-Coding-Plan/todo/CP-05-04-Firebase-Mcp.md) (consumer — Firebase source dùng dispatch này), [Task-204: Wire Production MCP Driver Adapter](../done/Task-204-Wire-Production-MCP-Driver-Adapter.md) (Drive adapter hiện tại), [Task-227: Generalize MCP Prompt-Injection And Preflight](./Task-227-Generalize-MCP-Prompt-Injection-And-Preflight.md) (sibling refactor)
- Replaces: `None`
- Tags: `mcp`, `context-source`, `refactor`, `adapter`, `foundation`

## AI Quick View

### Summary

- Hiện `mcp.driver` context source chỉ bind được **một** adapter (`SetMCPDriverAdapter` [context_source_mcp.go:131](../../../apps/local-runner/internal/runner/context_source_mcp.go:131) mutate in place; wire duy nhất ở [interactive_service.go:1637](../../../apps/local-runner/internal/runner/interactive_service.go:1637)). Điều này chặn việc thêm Jira/Firebase làm MCP-backed source song song với Google Drive.
- Task này refactor thành **dispatch nhiều adapter** — theo scheme của `driverRef` (`jira:`, `firebase:`, hoặc Drive file id mặc định) hoặc theo per-source adapter registration — mà **giữ nguyên hành vi Google Drive** (behavior-preserving, regression test).
- Đây là refactor nền tảng, không lộ tính năng mới; Jira (Task-229) và Firebase (Task-231) dựa lên nó.

### Current Ask

- Tách cơ chế single-adapter của `mcp.driver` thành dispatch đa-adapter/đa-scheme, giữ Drive chạy y hệt.

### Key Decisions

- `T-1` Chọn **một trong hai** hình dạng: (a) mỗi MCP là một `ContextSource` riêng với adapter riêng (`jiraIssueSource`, `firebaseCrashlyticsSource` — sạch hơn, ưu tiên); hoặc (b) giữ một `mcpDriverSource` nhưng dispatch theo scheme của `driverRef`. Chốt (a) trừ khi có lý do mạnh cho (b).
- `T-2` Behavior-preserving cho Google Drive: `googleDriveDriverAdapter` + `resolveGoogleDriveAccessTokenForRunner` + `readGoogleDriveDocument` không đổi hành vi; chỉ đổi cách được đăng ký/dispatch.
- `T-3` Không đổi contract `MCPDriverAdapter.Fetch(ctx, driverRef)` public shape trừ khi cần; nếu tách per-source thì mỗi source giữ adapter nội bộ của nó.

### Constraints

- Không phá `ContextSourceRegistry` contract (Register/Resolve/Collect) hay bất biến no-vector/deterministic (CP-44/SD-22 `D-2`).
- Không đổi `FlowContextPackage`/`PackageID`.
- Giữ 5s timeout + 8KB cap của `mcpDriverSource` ([context_source_mcp.go:18](../../../apps/local-runner/internal/runner/context_source_mcp.go:18)) cho mỗi MCP source.

### Open Questions

- `Q-1` (a) vs (b) ở `T-1` — ưu tiên (a). Xác nhận khi đọc code chi tiết.

### Source Refs

- `CP-05-06` `P-5`, `R-2`.
- `SD-22` `D-1` (registry), `D-2` (deterministic-only).
- current code: `context_source_mcp.go`, `context_sources_builtin.go`, `interactive_service.go:1637`.

## 1. Goal

Cho phép nhiều MCP-backed context source (Drive, Jira, Firebase) cùng đăng ký và fetch độc lập, thay vì một adapter Drive duy nhất; giữ hành vi Google Drive không đổi.

## 2. Parent Links

- coding plan: `CP-05-06` (`P-5`)
- tech design: `SD-22` (`D-1/D-2`), `SD-23` (`D-11`)
- system spec: `SS-14` (US-9/AC-16 extensibility)
- specific upstream ids: `CP-05-06 P-5`, `CP-05-06 R-2`

## 3. Trigger

Jira (Task-229) và Firebase (Task-231) cần là MCP-backed source cùng lúc với Drive; cơ chế single-adapter hiện tại chỉ giữ một adapter và sẽ bị ghi đè. Refactor này là blocker chung, phải land trước.

## 4. Exact Change

- `T-1` Giới thiệu dispatch đa-adapter: hoặc per-source adapter registration, hoặc scheme-prefix dispatch trên `driverRef`.
- `T-2` Di chuyển `googleDriveDriverAdapter` sang cơ chế mới; giữ wiring ở `AttachRunner` ([interactive_service.go:1637](../../../apps/local-runner/internal/runner/interactive_service.go:1637)) hoạt động.
- `T-3` Thêm regression test chứng minh Drive fetch không đổi output.
- `T-4` Để lại điểm mở rộng rõ ràng để Task-229/231 cắm adapter Jira/Firebase mà không sửa lại lõi dispatch.

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/context_source_mcp.go`, `context_sources_builtin.go`, `interactive_service.go` (wiring)
- modules: runner context-source registry + MCP adapter layer
- routes: none
- tables: none

## 6. Acceptance Check

- Google Drive `mcp.driver` fetch vẫn cho output y hệt (regression test xanh).
- Có thể đăng ký ≥2 MCP-backed source cùng lúc trong test mà không đè nhau.
- `go test ./apps/local-runner/internal/runner/...` xanh; guard no-vector giữ nguyên.

## 7. Out of Scope

- Không thêm source Jira/Firebase thật (đó là Task-229/231).
- Không đổi prompt-injection (Task-227).
- Không đổi connection/onboarding UI.

## 8. Completion Notes

- result: **done**. Đọc code thật cho thấy `T-1` phương án (a) đã đúng sẵn — mỗi `ContextSource` (`mcpDriverSource`, `featureHistorySource`,...) đã là struct độc lập giữ adapter riêng; registry (`Register`/`Resolve`/`Collect`) vốn đã hỗ trợ N source độc lập, "single adapter" chỉ là đặc điểm của riêng `mcpDriverSource`. Không cần refactor dispatch. Đã: (1) rút `mcpBoundedFetch` helper (timeout+cap+degrade contract) dùng chung cho mọi MCP-backed source tương lai (Jira/Firebase sẽ gọi lại, không copy-paste); (2) refactor `mcpDriverSource.Fetch` dùng helper, behavior-preserving (test cũ pass nguyên); (3) thêm `TestTwoMCPBackedSourcesCoexistIndependently` chứng minh 2 MCP source cùng registry không đè nhau. `go build ./...` sạch; `go test ./internal/runner/...` — 185 pass trên tập MCP/Mcp, 2 fail pre-existing không liên quan (`TestResolveGoogleDriveMcpProviderStatuses_AllNotStarted`, `TestParseGoogleDriveProxyMcpInvocation_AcceptsGoRunFallback` — xác nhận fail y hệt trên code gốc qua `git stash`, không do thay đổi này).
- follow-ups: Task-229/231 (Jira/Firebase context sources) nên gọi `mcpBoundedFetch` thay vì tự viết timeout/cap logic.
- upstream docs updated: CP-05-06 `P-5` (mô tả refactor) vẫn đúng ở mức ý định; chi tiết triển khai nhẹ hơn dự kiến ban đầu (không cần scheme-dispatch), ghi nhận ở đây thay vì sửa lại CP.
