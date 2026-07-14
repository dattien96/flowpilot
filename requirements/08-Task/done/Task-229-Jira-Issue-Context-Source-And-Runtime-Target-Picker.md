# Task-229: Jira Issue Context Source And Runtime Target Picker

## Metadata

- Document ID: `Task-229`
- Title: `Jira Issue Context Source And Runtime Target Picker`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-13`
- Last Updated: `2026-07-15`
- Parent Documents: [CP-05-06: Jira MCP As A Context Artifact Source](../../07-Coding-Plan/todo/CP-05-06-Jira-MCP.md) (`P-3`, `P-4`, `P-6b`, `P-7`)
- Child Documents: `None`
- Related Documents: [Task-226: MCP Context-Source Adapter Dispatch Refactor](./Task-226-MCP-Context-Source-Adapter-Dispatch-Refactor.md) (blocker), [Task-227: Generalize MCP Prompt-Injection And Preflight](./Task-227-Generalize-MCP-Prompt-Injection-And-Preflight.md) (blocker), [Task-228: Jira Remote-MCP Connection And Provider Config](./Task-228-Jira-Remote-MCP-Connection-And-Provider-Config.md) (blocker), [CP-44: Pluggable Context Source Registry](../../07-Coding-Plan/done/CP-44-Pluggable-Context-Source-Registry.md)
- Replaces: `None`
- Tags: `mcp`, `jira`, `context-source`, `context-artifact`, `runtime-question`, `prompt-injection`, `ui`

## AI Quick View

### Summary

- Đăng ký `jira.issue` và `jira.sprint` như **hai `ContextSource` độc lập** (`Q-1` resolved 2026-07-13 → source riêng, không phải target-mode chung) trong `ContextSourceRegistry` (mirror `mcpDriverSource`), opt-in (không trong `defaultContextSourceIDs`), qua điểm mở rộng của Task-226.
- Thêm runtime-target resolver `resolveJiraTargetForRun` (mirror `resolveMCPDriverTargetForRun` [flow_executor.go:438](../../../apps/local-runner/internal/runner/flow_executor.go:438)) dùng `AskWorkflowQuestion` để hỏi ticket key / JQL / active sprint; loại `jira.*` khỏi collect-path; inject bounded target note qua `appendJiraTargetPrompt`.
- UI: thêm `{ id: "jira.issue", label: "Jira Issue" }` và `{ id: "jira.sprint", label: "Jira Sprint" }` (2 entry độc lập) vào `contextSourceOptions` ([WorkflowsSettings.tsx:64](../../../apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx:64)) + sub-config target mode; wiring một use-case flow (Investigate Bug hoặc Analyze Sprint).

### Current Ask

- Cho `context_artifact.v1` bật Jira làm source, runtime hỏi target Jira, và AI dùng Jira MCP tools trong đúng scope — không dump full ticket vào package.

### Key Decisions

- `T-1` `jira.issue` và `jira.sprint` là **hai** `ContextSource` độc lập, mỗi cái `Deterministic()==true`, opt-in, mỗi cái 1 entry riêng trong `contextSourceOptions` (`Q-1` resolved).
- `T-2` Jira **bị loại khỏi** `FlowContextPackage` collect (mirror `filterString(...)` [flow_executor.go:338](../../../apps/local-runner/internal/runner/flow_executor.go:338)); chỉ inject target note.
- `T-3` Runtime question options: free-text issue key / JQL / "active sprint" (v1, `P-8`); rich picker page về sau.
- `T-4` `appendJiraTargetPrompt` bounded: server name `jira`, tool đọc ưu tiên, "chỉ trong issue/sprint đã chọn", failure codes (dùng contract từ Task-227).
- `T-5` `contextSourceOptions` client phải khớp registry id, nếu drift → fail flow-load (CP-44 Task-194 `T-2`).

### Constraints

- Blocker: Task-226 (dispatch), Task-227 (prompt), Task-228 (connection) phải land trước.
- No-vector/deterministic; `PackageID` không đổi; BUG-236.
- Source chỉ hiện khi Jira integration `status=connected` (CP-44 `P-7`).
- Read-only v1.

### Open Questions

- `Q-1` **(RESOLVED 2026-07-13 → source riêng)** `jira.sprint` là source độc lập với `jira.issue`, không phải target-mode chung (CP-05-06 `Q-3`). Step nào cần sprint tick `jira.sprint`; step nào cần 1 ticket tick `jira.issue`; một step có thể tick cả hai nếu cần.
- `Q-2` Picker list ticket: reuse REST (Task-228 `Q-1`) hay remote-MCP? v1 free-text nên tạm chưa cần.

### Source Refs

- `CP-05-06` `P-3`, `P-4`, `P-6b`, `P-8`.
- `CP-44` `§11.5` (MCP source happy path), Task-194 (validate sources).
- current code: `context_sources_builtin.go` (register), `context_source_mcp.go` (source impl), `flow_executor.go` (`resolveMCPDriverTargetForRun`, `appendGoogleDriveTargetPrompt`, filter), `interactive_service.go` (`AskWorkflowQuestion`), `WorkflowsSettings.tsx` (`contextSourceOptions`).

## 1. Goal

Cho step (Investigate Bug / Plan Task / Analyze Sprint) lấy context Jira deterministic qua `context_artifact.v1`: runtime chọn target Jira, AI dùng Jira MCP tools trong scope đó, không nhồi nội dung vào package.

## 2. Parent Links

- coding plan: `CP-05-06` (`P-3/P-4/P-6b/P-7`)
- tech design: `SD-22` (`D-5`), `SD-23` (`D-9/D-11`)
- system spec: `SS-14` (US-9/AC-16)
- specific upstream ids: `CP-05-06 P-3`, `P-4`, `P-6b`

## 3. Trigger

Sau khi connection (228) + refactor nền (226/227) sẵn sàng, đây là slice biến Jira thành context source dùng được end-to-end trong flow.

## 4. Exact Change

- `T-1` Đăng ký `jiraIssueSource` và `jiraSprintSource` (2 source độc lập) qua điểm mở rộng Task-226; không default-enable.
- `T-2` `resolveJiraTargetForRun` + workflow question (free-text/JQL/active-sprint); degrade khi expire/interrupt.
- `T-3` Loại `jira.*` khỏi collect list; `appendJiraTargetPrompt` bounded note (dùng contract Task-227).
- `T-4` Thêm hint field target Jira trên `FlowContextHints`/`BehaviorInput` (mirror `MCPDriverRef`) hoặc map chung.
- `T-5` UI: `contextSourceOptions` entry + sub-config target mode trong editor `context_artifact.v1`.
- `T-6` ~~Wiring built-in use-case flow~~ **out of scope** (2026-07-15): users wire `context_artifact.v1` manually in workflow editor; no seeded Investigate Bug / Analyze Sprint pack required.

## 5. Touched Areas

- files: `context_sources_builtin.go`, `context_source_mcp.go`, `flow_executor.go`, `mcp_prompt_instructions.go` (Jira contract branch), `WorkflowsSettings.tsx`; có thể `interactive_google_drive_picker.go` mẫu nếu làm picker
- modules: runner context assembly, interactive question, prompt augmentation, desktop artifact authoring
- routes: (tùy chọn) `/client/questions/{id}/jira-picker`
- tables: none

## 6. Acceptance Check

- Tạo `context_artifact.v1` bật `jira.issue`, bind INPUT vào step, run flow → runtime hỏi ticket → prompt handoff có bounded target note; package **không** chứa full ticket body.
- Unknown Jira source id → fail flow-load rõ ràng.
- Skip question → degrade, step chạy bằng source còn lại.
- Test Console `jira_list_bugs` trả issue thật.
- `go test ./apps/local-runner/internal/runner/...` xanh.

## 7. Out of Scope

- Connection/OAuth/provider-config (Task-228).
- Adapter dispatch + prompt generalization (Task-226/227).
- Write actions (create/transition issue).
- Rich Jira picker page (defer trừ khi `Q-2` yêu cầu).

## 8. Completion Notes

- result: **done**.
  - `apps/local-runner/internal/runner/context_source_jira.go` (new): `jiraIssueSource`/`jiraSprintSource` — two independent `ContextSource` structs (`Q-1` resolved → separate sources), each `Deterministic()==true`, opt-in (not in `defaultContextSourceIDs`), using Task-226's `mcpBoundedFetch` helper. Production adapters `jiraRestIssueAdapter`/`jiraRestSprintAdapter` reuse the **existing, already-tested** Jira REST credential path (`executeJiraRequestFn`/`jiraCredential`, CP-05-01/02) via a new `resolveConnectedJiraCredential` helper (mirrors `resolveGoogleDriveAccessTokenForRunner`'s "one active connection per workspace" model) — issue adapter fetches summary/status/assignee/description; sprint adapter resolves the board's active sprint (or an explicit sprint id) via the Jira Agile API and lists its issues. Both registered in `registerBuiltinContextSources` and wired in `AttachRunner` (mirrors `SetMCPDriverAdapter`).
  - `flow_executor.go`: `resolveJiraIssueTargetForRun`/`resolveJiraSprintTargetForRun` (mirror `resolveMCPDriverTargetForRun` exactly — `AskWorkflowQuestion`, degrade on expire/interrupt) + `normalizeJiraIssueTarget`/`normalizeJiraSprintTarget` + `appendJiraIssueTargetPrompt`/`appendJiraSprintTargetPrompt` (bounded target notes, mirror `appendGoogleDriveTargetPrompt`). Wired into `startInlineEntryChain`: both sources filtered out of the collect-path (`filterString`) and their resolved targets injected as prompt notes instead — same division of labor as `mcp.driver` (package never carries Jira content; the AI turn fetches it live via the Jira MCP server, Task-228).
  - `FlowContextHints`/`BehaviorInput` gained `JiraIssueRef`/`JiraSprintRef` (mirror `MCPDriverRef`), threaded through `behaviorContextProduce`.
  - UI: `contextSourceOptions` in `WorkflowsSettings.tsx` gained `jira.issue`/`jira.sprint` entries + a sub-note (mirrors the `mcp.driver` note) explaining the runtime-question behavior and the connected-Jira-integration dependency.
  - Tests: `context_source_jira_test.go` (new, 12 cases — bounded section/SourceRef, empty-ref no-op, adapter-error degrade, registered-not-default, no-vector guard, normalize helpers, prompt-note bounded-scope + no-op). `go build ./...` clean; `go vet` clean; `go test ./internal/runner/... -run 'Jira|jira'` — 31 pass; full suite `go test ./internal/runner/...` — 1415 passed / 16 failed / 18 skipped (same 16 pre-existing/unrelated failures as Task-226/227/228, zero new regressions — count only grew by the ~12 new passing tests). `tsc --noEmit` in `apps/desktop-flowpilot` clean.
  - **Scope note (consistent with Task-228):** the production adapters fetch real Jira data via the REST/API-token path (already fully working, CP-05-01/02) — this is what a direct/test invocation of `jiraIssueSource.Fetch` would return. In the live AI-turn path, these adapters are NOT what the AI actually uses (the source is deliberately filtered out of `startInlineEntryChain`'s collect); the AI instead calls the real Jira MCP tools per the target-note prompt injected by `appendJiraIssueTargetPrompt`/`appendJiraSprintTargetPrompt`, which requires Task-228's OAuth-backed remote-MCP provider config to actually be present — the same documented gap as Task-228 (no live OAuth handshake in this environment).
  - Built-in Investigate Bug / Analyze Sprint pack flows: **won't-do** — manual artifact/workflow wiring is the supported path.
- follow-ups: none.
- upstream docs updated: none required — CP-05-06 `P-3`/`P-4`/`P-6b` describe exactly this shape.

## 9. Addendum (Task-234 revisit): fetch adapters unwired

Task-234 confirmed the reasoning above and acted on it: `jiraRestIssueAdapter`/`jiraRestSprintAdapter` were **unwired** from `AttachRunner` (`interactive_service.go`) — the collect-time filter already meant `Fetch` was never invoked in a live turn, so wiring them was dead weight (CP-05-06 `R-1`: REST is scoped to the pick-list use case, not live content). The adapter files/types are kept intact (not deleted) per explicit instruction, in case a future non-live consumer (e.g. a Test Console direct-collect path) wants them — only `SetJiraIssueAdapter`/`SetJiraSprintAdapter` calls were removed. Separately, Task-234 also gave the remote-MCP provider-config path (the one the live AI turn actually depends on) a working per-turn merge for Claude/Grok plus Codex/Gemini static-config writers — closing the gap this task's own completion notes flagged ("requires Task-228's OAuth-backed remote-MCP provider config to actually be present").
