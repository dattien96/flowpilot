# Task-227: Generalize MCP Prompt-Injection And Preflight

## Metadata

- Document ID: `Task-227`
- Title: `Generalize MCP Prompt-Injection And Preflight`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-13`
- Last Updated: `2026-07-13`
- Parent Documents: [CP-05-06: Jira MCP As A Context Artifact Source](../../07-Coding-Plan/todo/CP-05-06-Jira-MCP.md) (`P-6`), [SD-11: MCP Connection Flows](../../06-System-Tech-Design/SD-11-MCP-Connection-Flows.md), [CP-05-03: Google Drive MCP Current Implementation Notes](../../07-Coding-Plan/done/CP-05-03-Driver-Mcp.md) (§11.10, §11.11)
- Child Documents: `None`
- Related Documents: [Task-226: MCP Context-Source Adapter Dispatch Refactor](./Task-226-MCP-Context-Source-Adapter-Dispatch-Refactor.md) (sibling foundation), [CP-05-04: Firebase MCP](../../07-Coding-Plan/todo/CP-05-04-Firebase-Mcp.md), [CP-05-05: Telegram MCP](../../07-Coding-Plan/todo/CP-05-05-Tele-Mcp.md)
- Replaces: `None`
- Tags: `mcp`, `prompt-injection`, `preflight`, `refactor`, `foundation`

## AI Quick View

### Summary

- `mcp_prompt_instructions.go` hiện **hardcode `google_drive`** — `requiresGoogleDriveMcp()` chỉ match `"google_drive"` ([mcp_prompt_instructions.go:31](../../../apps/local-runner/internal/runner/mcp_prompt_instructions.go:31)), `buildGoogleDriveMcpInstructions` chỉ dựng block Drive, `PreflightGoogleDriveMcp` chỉ check Drive.
- Task này tổng quát hóa thành **contract per-MCP**: mỗi MCP key (`google_drive`, `jira`, `firebase`, `telegram`) khai server name, tool allowlist đọc/ghi, failure codes, và preflight riêng — chọn qua một registry/switch nhỏ, giữ Google Drive **behavior-preserving**.
- Nền cho prompt-injection của Jira (Task-229), Firebase (Task-231) và Telegram output (Task-233).

### Current Ask

- Refactor prompt-injection + preflight từ Drive-hardcode sang per-MCP contract, giữ output Google Drive không đổi.

### Key Decisions

- `T-1` Thêm một `mcpInstructionSpec` per MCP key: `{ serverName, readTools[], writeTools[], failureCodes[], preflight fn }`; `InjectRequiredMcpInstructions` chọn spec theo `requiredMcps` thay vì hardcode Drive.
- `T-2` Giữ nguyên nội dung block Google Drive hiện tại (byte-preserving nếu có golden/prompt test) để không phá CP-05-03 Phase A/B đã pass.
- `T-3` Preflight tổng quát: `PreflightMcp(key, providerKey, accountHome)` dispatch tới per-MCP checker; Drive dùng lại logic hiện có.

### Constraints

- Không phá Phase A/B của CP-05-03 (provider config + preflight + failure-code detection) — regression test giữ.
- Không đổi cơ chế provider-config injection (`google_drive_mcp_provider_config.go`) — chỉ đổi prompt/preflight layer.
- Giữ nguyên tắc: chỉ inject khi `requiredMcps` chứa key tương ứng.

### Open Questions

- `Q-1` Jira/Firebase/Telegram tool names + failure codes cần chốt khi từng CP land; Task này chỉ dựng khung + Drive.

### Source Refs

- `CP-05-06` `P-6`.
- `CP-05-03` §11.10 (prompt augmentation contract), §11.11 (preflight), §11.14 (Phase B failure-code detection).
- current code: `mcp_prompt_instructions.go`, `provider_driven_mcp.go` (`detectMcpFailureCode`/`detectMcpToolUsed`).

## 1. Goal

Biến prompt-injection + preflight MCP thành cơ chế per-MCP mở rộng được, giữ Google Drive không đổi, để Jira/Firebase/Telegram cắm contract riêng mà không sửa lõi.

## 2. Parent Links

- coding plan: `CP-05-06` (`P-6`)
- tech design: `SD-11` (§3), `SD-23` (`D-11`)
- system spec: `SS-14` (AC-16)
- specific upstream ids: `CP-05-06 P-6`, `CP-05-03 §11.10/§11.11`

## 3. Trigger

Prompt/preflight hardcode `google_drive` chặn mọi MCP mới; CP-05-06/04/05 đều cần block prompt riêng. Đây là refactor nền, land cùng/sau Task-226.

## 4. Exact Change

- `T-1` Thêm `mcpInstructionSpec` registry + generic `InjectRequiredMcpInstructions` dispatch theo key.
- `T-2` Chuyển logic Drive hiện tại vào spec `google_drive` mà không đổi output.
- `T-3` Generic `PreflightMcp(key, ...)`; Drive dùng lại checker cũ.
- `T-4` Test: Drive block/preflight không đổi; một fake MCP key mới inject đúng block riêng.

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/mcp_prompt_instructions.go` (+ test), có thể chạm `provider_driven_mcp.go` cho failure-code generalization
- modules: runner prompt-assembly + preflight
- routes: none
- tables: none

## 6. Acceptance Check

- Google Drive prompt block + preflight cho output byte-identical trước/sau (regression test).
- Đăng ký một MCP key giả → inject đúng block riêng, không lẫn Drive.
- `go test ./apps/local-runner/internal/runner/...` xanh.

## 7. Out of Scope

- Không viết contract Jira/Firebase/Telegram thật (thuộc từng CP task).
- Không đổi adapter dispatch (Task-226).
- Không đổi provider-config file writers.

## 8. Completion Notes

- result: **done**. Thêm `mcpInstructionSpec` (`key`, `buildInstructions`, `preflight`) + registry `mcpInstructionSpecs` + `registerMcpInstructionSpec` + `resolveRequiredMcpSpec` trong `mcp_prompt_instructions.go`. `google_drive` đăng ký làm compile-time built-in duy nhất hiện có, giữ nguyên `buildGoogleDriveMcpInstructions`/`PreflightGoogleDriveMcp` không đổi API công khai. `InjectRequiredMcpInstructions`, `preparePromptForRequiredMcps`, `applyRequiredMcpFailureStatus` chuyển từ check cứng `requiresGoogleDriveMcp` sang `resolveRequiredMcpSpec` (dispatch theo key có đăng ký) — behavior-preserving cho Drive (test cũ pass nguyên byte-for-byte qua assertion có sẵn). Thêm `PreflightMcp(key, providerKey, accountHomePath)` public dispatcher cho Task-228/230 gọi khi đăng ký Jira/Firebase spec. Thêm `TestRegisterMcpInstructionSpecInjectsOwnBlockNotGoogleDrive` chứng minh 1 MCP key mới inject đúng block riêng, không lẫn Drive, và key chưa đăng ký thì prompt không đổi. `go build ./...` sạch; `go test ./internal/runner/...` toàn bộ nhóm Mcp/MCP — 185 pass (cùng 2 fail pre-existing không liên quan, xác nhận ở Task-226) + 21 pass riêng cho prompt-instruction/preflight tests.
- follow-ups: Task-228/230/233 gọi `registerMcpInstructionSpec` cho `jira`/`firebase`/`telegram`, dùng `PreflightMcp` thay vì viết preflight riêng lẻ.
- upstream docs updated: none cần thiết — CP-05-06 `P-6` mô tả đúng ý định, chi tiết triển khai khớp.
