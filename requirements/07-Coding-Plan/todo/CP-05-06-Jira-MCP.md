# CP-05-06: Jira MCP As A Context Artifact Source

## Metadata

- Document ID: `CP-05-06`
- Title: `Jira MCP As A Context Artifact Source`
- Phase: `coding_plan`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-13`
- Last Updated: `2026-07-13`
- Parent Documents: [SD-11: MCP Connection Flows](../../06-System-Tech-Design/SD-11-MCP-Connection-Flows.md), [SD-23: Generic Artifact Framework](../../06-System-Tech-Design/SD-23-Generic-Artifact-Framework.md), [SD-22: Pluggable Context Source Registry](../../06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md)
- Child Documents: [Task-226: MCP Context-Source Adapter Dispatch Refactor](../../08-Task/todo/Task-226-MCP-Context-Source-Adapter-Dispatch-Refactor.md) (`P-5`), [Task-227: Generalize MCP Prompt-Injection And Preflight](../../08-Task/todo/Task-227-Generalize-MCP-Prompt-Injection-And-Preflight.md) (`P-6`), [Task-228: Jira Remote-MCP Connection And Provider Config](../../08-Task/todo/Task-228-Jira-Remote-MCP-Connection-And-Provider-Config.md) (`P-1`/`P-2`), [Task-229: Jira Issue Context Source And Runtime Target Picker](../../08-Task/todo/Task-229-Jira-Issue-Context-Source-And-Runtime-Target-Picker.md) (`P-3`/`P-4`/`P-6b`)
- Related Documents: [CP-44: Pluggable Context Source Registry](../done/CP-44-Pluggable-Context-Source-Registry.md) (substrate — `mcp.driver` là mẫu source ngoài), [CP-45: Generic Artifact Types And Instances](../done/CP-45-Generic-Artifact-Types-And-Instances.md) (artifact framework — Jira cắm vào `context_artifact.v1`), [CP-05-03: Google Drive MCP Current Implementation Notes](../done/CP-05-03-Driver-Mcp.md) (mẫu provider-CLI-owns-MCP), [CP-05-01: Jira MCP API Token Flow](../done/CP-05-01-Jira-Mcp-Api-Token.md) (legacy token path — nay là fallback, không phải primary), [CP-05-02: MCP Test Console](../done/CP-05-02-Mcp-Test-Console.md) (test surface), [CP-05-04: Firebase MCP](./CP-05-04-Firebase-Mcp.md) (sibling input source), [CP-05-05: Telegram MCP](./CP-05-05-Tele-Mcp.md) (sibling output artifact)
- Replaces: `None`
- Tags: `mcp`, `jira`, `atlassian`, `context-source`, `context-artifact`, `artifact-framework`, `remote-mcp`, `flow-mode`

## AI Quick View

### Summary

- CP-05-06 gắn **Jira** vào artifact framework hiện có như một **context source cho INPUT step**, đối xứng chính xác với `mcp.driver` (Google Drive) đã ship ở CP-44/CP-45 — **không** thêm artifact type mới, chỉ thêm hai `ContextSource` mới độc lập (`jira.issue`, `jira.sprint`) vào `ContextSourceRegistry` rồi cho `context_artifact.v1` tick chọn.
- Transport là **Atlassian official remote MCP** (`https://mcp.atlassian.com/v1/mcp/authv2`, OAuth trình duyệt) đúng theo SD-11 §3.2. Theo nguyên tắc CP-05-03 §11, **AI provider CLI là MCP client**; runner chỉ config provider, preflight, chèn prompt-instruction, rồi launch CLI để nó tự dùng Jira MCP tools.
- Giữ đúng mô hình 2 lớp của `mcp.driver`: **Connection tĩnh (per-project)** = Atlassian site + OAuth (Surface ①); **Runtime target động (per-run)** = user chọn ticket / sprint / JQL qua một câu hỏi workflow-driven khi flow chạy (Surface ④). Runner **không** nhồi nội dung Jira vào `FlowContextPackage`; nó chỉ inject một *target note* để AI tự fetch bằng Jira MCP tools trong đúng scope.
- Use case đích: các step như **Investigate Bug**, **Plan Task**, **Analyze Team Sprint** — mỗi step bind một `context_artifact.v1` instance có bật `jira.issue`/`jira.sprint`, rồi runtime hỏi target Jira cụ thể.
- Điều kiện tiên quyết kỹ thuật: `mcp.driver` hiện chỉ bind **1 adapter** ([context_source_mcp.go:131](../../../apps/local-runner/internal/runner/context_source_mcp.go:131)); phải refactor thành **dispatch theo scheme** để Drive + Jira (+ Firebase) cùng sống. Và `mcp_prompt_instructions.go` đang **hardcode `google_drive`** — phải tổng quát hóa.

### Current Ask

- Thiết kế + lên kế hoạch triển khai Jira như một MCP-backed context source cắm vào `context_artifact.v1`, dùng Atlassian remote MCP + OAuth, với runtime-target picker và prompt-injection theo mẫu `mcp.driver`, mà không phá contract `FlowContextPackage`/artifact framework.

### Key Decisions

- `P-1` **Transport = Atlassian remote MCP + OAuth** (SD-11 §3.2), **không** phải Go MCP client. Runner sở hữu connection + preflight + prompt-instruction; provider CLI (Claude/Codex/Gemini/Grok) là MCP client thực sự, đúng CP-05-03 §11. Legacy API-token/REST path của CP-05-01 hạ xuống **fallback/diagnostic**, không phải luồng chính.
- `P-2` **Jira là context source, không phải artifact type mới.** Đăng ký `jira.issue` và `jira.sprint` như **hai `ContextSource` độc lập** (`Q-3` resolved) trong `ContextSourceRegistry` ([context_sources_builtin.go:141](../../../apps/local-runner/internal/runner/context_sources_builtin.go:141)), opt-in (không nằm trong `defaultContextSourceIDs`), mirror `mcpDriverSource`. `context_artifact.v1` tick chọn qua `config_json.sources` như mọi source khác.
- `P-3` **Tách Connection (tĩnh) vs Runtime target (động)** giống `mcp.driver`: site/OAuth cấu hình 1 lần ở Surface ①; ticket/sprint/JQL chọn per-run ở Surface ④ qua `AskWorkflowQuestion` ([interactive_service.go:3827](../../../apps/local-runner/internal/runner/interactive_service.go:3827)).
- `P-4` **Jira bị loại khỏi collect-path** của `FlowContextPackage` (giống `mcp.driver` bị `filterString` ở [flow_executor.go:338](../../../apps/local-runner/internal/runner/flow_executor.go:338)); thay vào đó inject một *target note* bounded qua `appendJiraTargetPrompt` (mirror `appendGoogleDriveTargetPrompt` [flow_executor.go:511](../../../apps/local-runner/internal/runner/flow_executor.go:511)). **Không** append full nội dung ticket vào package.
- `P-5` **Refactor single-adapter → scheme dispatch.** `SetMCPDriverAdapter`/`mcpDriverSource` hiện chỉ giữ 1 adapter; đổi sang dispatch theo scheme (`jira:` vs Drive file id vs `firebase:`) hoặc mỗi source một adapter riêng, để Drive + Jira + Firebase cùng đăng ký được.
- `P-6` **Tổng quát hóa prompt-injection + preflight.** `mcp_prompt_instructions.go` đang hardcode `google_drive` (`requiresGoogleDriveMcp`, `buildGoogleDriveMcpInstructions`); tách thành contract per-MCP (tool names, failure codes, read/write policy) + `PreflightJiraMcp`.
- `P-7` **Read-only v1.** Chỉ tools đọc (get issue, search JQL, list sprint issues). Write (create/transition issue) là scope sau, gate bằng approval như CP-05-03 §11.15 / CP-29.
- `P-8` **Runtime picker v1 = free-text/JQL + select active sprint**; rich picker page (mirror `interactive_google_drive_picker.go`) là tối ưu về sau (`Q-3`).

### Constraints

- Kế thừa bất biến CP-41/CP-44: deterministic, **no vector/embedding/similarity search**; Jira source truy xuất bằng lookup tường minh theo id/JQL, mang `SourceRef`, degrade-mềm (Jira down → warning, không fail run) như Task-168 `T-4`.
- Không phá contract `FlowContextPackage`/`PackageID`; Jira thêm section qua `Sections` projection hoặc chỉ qua prompt-note (P-4), không sửa struct.
- **Secret boundary (SD-11 §6, CP-05-01 §5):** OAuth token/refresh token chỉ ở runner keyring ([secret_store.go](../../../apps/local-runner/internal/runner/secret_store.go)); **không** vào Supabase `config_encrypted`, không log, không query string.
- Nguồn ngoài chỉ tới MCP **đã connect** (CP-44 `P-7`); options ở UI giới hạn theo integration `status=connected`.
- Giữ contract BUG-236: binding ở `step_definitions`, không ghi metadata artifact xuống `workflow_steps`.
- `client-core`/desktop `contextSourceOptions` là descriptor **đồng bộ tay** với Go registry ([WorkflowsSettings.tsx:64](../../../apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx:64)); id drift → fail flow-load (CP-44 Task-194 `T-2`).

### Open Questions

- `Q-1` Runtime picker cho Jira: v1 dùng free-text issue key/JQL + list active sprint là đủ, hay cần rich picker page (list bugs qua MCP) ngay? Đề xuất: free-text trước (`P-8`), picker sau.
- `Q-2` Runner có cần một Go client nói remote-MCP transport **chỉ để list ticket cho picker** không (vì provider CLI mới là MCP client)? Hoặc dùng lại REST verify path của CP-05-01 cho phần list-để-pick? Đề xuất: reuse REST cho pick, remote-MCP cho AI step.
- `Q-3` **(RESOLVED 2026-07-13 → source riêng)** `jira.sprint` là một `ContextSource` **độc lập** với `jira.issue`, mỗi cái 1 checkbox riêng trong `contextSourceOptions` (mirror pattern hiện có — mỗi source 1 entry độc lập, không cần logic chọn "loại target" 2 tầng lúc runtime). Step "Analyze Sprint" tick sẵn `jira.sprint`; step "Investigate Bug" tick `jira.issue`. Đơn giản hơn phương án target-mode chung.
- `Q-4` CP-05-01 (token) vs SD-11 (OAuth remote): CP-05-06 chọn OAuth remote. Có cần giữ song song token path làm fallback headless không, hay retire hẳn CP-05-01? Cần owner chốt trước Task.
- `Q-5` Confluence dùng chung Atlassian credential (CP-05-02 §3) — có gộp vào cùng integration/source `atlassian.*` không, hay tách? v1 chỉ Jira.

### Source Refs

- `SD-11` §2 (Add MCP sequence), §3.2 (Jira = Atlassian remote MCP `/v1/mcp/authv2`), §6 (secret boundary).
- `SD-23` `D-5` (`context_artifact.v1` compose lên `ContextSourceRegistry`), `D-9` (deterministic/no-vector mọi producer), `D-11` (prompt-assembly seam).
- `SD-22` `D-1` (registry pattern), `D-2` (deterministic-only), `D-5` (external boundary + degrade).
- `CP-44` `P-5` (MCP-backed source adapter), `P-7` (chỉ MCP đã connect), `§11.5/§11.6` (MCP happy-path + degrade).
- `CP-05-03` §11 (provider-CLI-owns-MCP), §11.10 (prompt augmentation contract), §11.11 (preflight).
- current code: `context_source_registry.go`, `context_sources_builtin.go`, `context_source_mcp.go`, `flow_executor.go` (`resolveMCPDriverTargetForRun`, `appendGoogleDriveTargetPrompt`), `interactive_service.go` (`AskWorkflowQuestion`), `mcp_prompt_instructions.go`, `runner.go` (`mcpBackendSpecs`, `jiraCredential`, `executeJiraMcpPrompt`), `WorkflowsSettings.tsx` (`contextSourceOptions`), `settingsHelpers.ts` (`providerFields.jira`).

## 1. Goal

Cho các flow step cần dữ liệu Jira (Investigate Bug, Plan Task, Analyze Team Sprint, ...) khả năng lấy context Jira **có kiểm soát, deterministic, auditable** thông qua Atlassian remote MCP, bằng cách gắn Jira vào artifact framework hiện có như một context source của `context_artifact.v1` — tái dùng nguyên mô hình `mcp.driver` (connection tĩnh + runtime target động + prompt-note + provider-CLI-uses-MCP), không thêm artifact type mới, không phá `FlowContextPackage`.

## 2. Input Documents

- `requirements/06-System-Tech-Design/SD-11-MCP-Connection-Flows.md`
- `requirements/06-System-Tech-Design/SD-23-Generic-Artifact-Framework.md`
- `requirements/06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md`
- `requirements/07-Coding-Plan/done/CP-44-Pluggable-Context-Source-Registry.md`
- `requirements/07-Coding-Plan/done/CP-45-Generic-Artifact-Types-And-Instances.md`
- `requirements/07-Coding-Plan/done/CP-05-03-Driver-Mcp.md`
- `requirements/07-Coding-Plan/done/CP-05-01-Jira-Mcp-Api-Token.md`
- `apps/local-runner/internal/runner/context_source_mcp.go`
- `apps/local-runner/internal/runner/mcp_prompt_instructions.go`
- `apps/local-runner/internal/runner/flow_executor.go`
- `apps/local-runner/internal/runner/runner.go` (`mcpBackendSpecs`, Jira REST helpers)
- `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx`
- `apps/desktop-flowpilot/src/components/settings/settingsHelpers.ts`

## 3. Implementation Strategy

- overall approach:
  - Clone toàn bộ pipeline `mcp.driver` cho Jira: connection layer (integration row + keyring + OAuth) → context source registration (`jira.issue`) → runtime-target resolver + workflow question → prompt-note injection → provider-config injection để CLI dùng Atlassian remote MCP.
  - Runner **không** tự parse Jira; nó config remote MCP cho provider account, preflight (OAuth ok + provider config ok), inject `## Required MCP Usage` + target note, rồi launch CLI (đúng CP-05-03 §11.12).
  - Tách rõ 2 lớp: Connection (Surface ①, tĩnh) và Runtime target (Surface ④, động).
- sequencing logic:
  - `P-1` connection/OAuth → `P-5` adapter-dispatch refactor (điều kiện tiên quyết) → `P-2` provider-config injection remote-MCP → `P-6` prompt/preflight generalization → `P-3` register source → `P-4` UI source + sub-config → runtime target + question → use-case wiring → tests.
  - `P-5` (scheme dispatch) phải land trước hoặc cùng `P-3`, nếu không Jira đè lên adapter Drive.
- dependencies:
  - Yêu cầu CP-44/CP-45 (đã done) cho registry + artifact framework.
  - Yêu cầu Node.js trên máy runner cho local proxy path của Atlassian (SD-11 §3.2 "the local machine still needs Node.js for the proxy path").
  - Độc lập với CP-05-04 (Firebase) và CP-05-05 (Telegram) nhưng chia chung refactor `P-5` (adapter dispatch) và `P-6` (prompt generalization) với Firebase.

## 4. Work Breakdown

- `P-1` **Connection layer (Atlassian remote MCP + OAuth).**
  - Tái dùng `integrations` type `jira` + `project_mcp_links` + keyring `jira:<integrationId>` (`jiraCredential`, [runner.go:2377](../../../apps/local-runner/internal/runner/runner.go:2377)).
  - Onboarding UI trong `McpSettings.tsx` (mode `jira`): link Atlassian guide + trigger OAuth trình duyệt; status `pending → awaiting_oauth → connected` (SD-11 §2). Duplicate-guard theo normalized site URL (CP-05-01 §3.2).
  - `mcpBackendSpec` jira đã có transport `remote` + `BinaryPath: https://mcp.atlassian.com/v1/mcp/authv2` ([runner.go:2302](../../../apps/local-runner/internal/runner/runner.go:2302)) — bổ sung code thực sự nói remote-MCP setup (hiện chưa có).

- `P-2` **Provider-config injection cho remote MCP.**
  - Thêm `jira_mcp_provider_config.go` mirror `google_drive_mcp_provider_config.go`: `Ensure*JiraMcpConfig`/`check*`/`detectStale*` cho codex/gemini/claude/grok, viết entry `mcpServers`/`mcp_servers` trỏ Atlassian remote endpoint + OAuth header/token env.
  - Claude `--strict-mcp-config` path: thêm Jira vào `flowpilotClaudeExtraMCPServers` (mirror [google_drive_mcp_provider_config.go:984](../../../apps/local-runner/internal/runner/google_drive_mcp_provider_config.go:984)).

- `P-5` **Adapter/source dispatch refactor (điều kiện tiên quyết).**
  - Đổi `mcp.driver` single-adapter ([context_source_mcp.go:131](../../../apps/local-runner/internal/runner/context_source_mcp.go:131)) sang dispatch theo scheme, hoặc tách thành các source độc lập với adapter riêng. Giữ Drive chạy nguyên (regression test).

- `P-6` **Prompt-injection + preflight generalization.**
  - Refactor `mcp_prompt_instructions.go` từ hardcode `google_drive` thành per-MCP contract (server name, tool allowlist, failure codes). Thêm `PreflightJiraMcp` (OAuth ok, provider config ok, không stale) theo CP-05-03 §11.11.

- `P-3` **Register `jira.issue` context source.**
  - `jiraIssueSource` implement `ContextSource` (mirror `mcpDriverSource` [context_source_mcp.go:41](../../../apps/local-runner/internal/runner/context_source_mcp.go:41)); `Deterministic()==true`; đăng ký trong `registerBuiltinContextSources` ([context_sources_builtin.go:141](../../../apps/local-runner/internal/runner/context_sources_builtin.go:141)); **không** thêm vào `defaultContextSourceIDs`. Tùy chọn thêm `jira.sprint` (`Q-3`).
  - Thêm hint field trên `FlowContextHints`/`BehaviorInput` cho Jira target (mirror `MCPDriverRef`), hoặc dùng chung một `MCPTargets map`.

- `P-4` **UI: source + sub-config.**
  - Thêm `{ id: "jira.issue", label: "Jira" }` (và `jira.sprint`) vào `contextSourceOptions` ([WorkflowsSettings.tsx:64](../../../apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx:64)) → tự hiện trong editor `context_artifact.v1` + step picker.
  - Sub-config per-source (như note đặc biệt của `mcp.driver`): **Target mode** = `ask-at-runtime` | `fixed` (JQL/issue key gõ sẵn) | `from-prompt`; chọn connected Jira integration nếu project có nhiều.

- `P-6b` **Runtime target resolver + workflow question.**
  - `resolveJiraTargetForRun` mirror `resolveMCPDriverTargetForRun` ([flow_executor.go:438](../../../apps/local-runner/internal/runner/flow_executor.go:438)): nếu `jira.issue` enabled + mode=`ask` → `AskWorkflowQuestion` với options (free-text issue key / JQL / "active sprint"); mode=`fixed` → dùng config; degrade khi expire/interrupt.
  - `filterString` loại `jira.*` khỏi collect list; `appendJiraTargetPrompt(prompt, target)` inject note bounded: server name `jira`, tool đọc ưu tiên, "chỉ trong issue/sprint đã chọn, không query rộng hơn", failure codes.
  - (Tùy chọn `P-8`) picker page `/client/questions/{id}/jira-picker` mirror `interactive_google_drive_picker.go`.

- `P-7` **Use-case wiring.**
  - Seed/định nghĩa step behavior mẫu cho Investigate Bug / Plan Task / Analyze Sprint (built-in flow hoặc pack), mỗi step bind `context_artifact.v1` instance có `jira.issue`/`jira.sprint`.

- `P-9` **Test Console (CP-05-02) hookup.** Thêm Jira read actions (`jira_list_bugs`, `jira_get_ticket_content`) chạy qua đường mới để verify connected instance usable.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/context_source_mcp.go` (dispatch refactor + `jiraIssueSource`)
  - `apps/local-runner/internal/runner/context_sources_builtin.go` (register)
  - `apps/local-runner/internal/runner/flow_executor.go` (`resolveJiraTargetForRun`, `appendJiraTargetPrompt`, filter)
  - `apps/local-runner/internal/runner/mcp_prompt_instructions.go` (generalize + Jira contract + preflight)
  - mới: `apps/local-runner/internal/runner/jira_mcp_provider_config.go` (+ test)
  - `apps/local-runner/internal/runner/runner.go` (Jira backend spec remote-MCP setup; reuse keyring cred)
  - `apps/local-runner/internal/cli/root.go` (endpoint/subcommand nếu cần local proxy)
  - `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx` (`contextSourceOptions` + sub-config)
  - `apps/desktop-flowpilot/src/components/settings/McpSettings.tsx` (Jira OAuth onboarding)
  - `apps/desktop-flowpilot/src/components/settings/settingsHelpers.ts` (Jira field tweaks nếu chuyển OAuth)
- modules:
  - runner context assembly (Plan/entry step), MCP provider-config, prompt augmentation, interactive question
  - desktop settings (MCP servers + artifact instance authoring)
- database:
  - không thêm bảng; dùng `integrations`/`project_mcp_links` sẵn có (type `jira` đã trong CHECK constraint).
- external systems:
  - Atlassian remote MCP `https://mcp.atlassian.com/v1/mcp/authv2`; Jira REST (chỉ cho pick/list nếu `Q-2` chọn reuse).

## 6. Data or Migration Steps

- schema:
  - Không migration DB (type `jira` đã hợp lệ).
- data backfill:
  - Không cần.
- config updates:
  - Provider MCP config (Claude/Codex/Gemini/Grok) được viết per-account-home lúc `Ensure*JiraMcpConfig` (không commit vào repo, theo CP-05-03 §11.4).
  - `context_artifact.v1` instance mới (user tạo) mang `config_json.sources` chứa `jira.issue` + sub-config target mode.

## 7. Validation Plan

- tests to add:
  - `TestJiraIssueSourceRegisteredDeterministic` — source đăng ký, `Deterministic()==true`, không trong default set.
  - `TestAdapterDispatchKeepsDriveAndJiraSeparate` — refactor `P-5` không phá Drive; scheme dispatch đúng.
  - `TestResolveJiraTargetForRunPromptsWhenAsk` / `...UsesFixedConfig` / `...DegradesOnExpire` — mirror các test `resolveMCPDriverTargetForRun`.
  - `TestAppendJiraTargetPromptBoundedScope` — note chỉ giới hạn issue/sprint đã chọn, không broad search; có failure codes.
  - `TestJiraExcludedFromContextPackageCollect` — Jira không nhồi nội dung vào `FlowContextPackage`.
  - `TestEnsureJiraMcpProviderConfig*` — viết/không đè config, stale detection (mirror Google Drive provider-config tests).
  - `TestInjectRequiredMcpInstructionsJira` — prompt có server name `jira`, tool allowlist, failure codes; không inject khi source tắt.
  - `TestValidateFlowArtifactBindingsUnknownJiraSourceFailsFast` — id lạ fail flow-load (kế thừa CP-44).
- manual checks:
  - Connect Jira qua OAuth → status `connected`; tạo `context_artifact` instance bật `jira.issue`; bind vào step Investigate Bug; run flow → runtime hỏi ticket → AI fetch qua Jira MCP đúng scope; prompt handoff chứa target note, **không** dump full ticket vào package.
  - Test Console: `jira_list_bugs` trả issue thật.
- failure cases:
  - Jira chưa connect → source không hiện / preflight chặn step với lỗi rõ, trỏ Surface ①.
  - OAuth hết hạn → `MCP_AUTH_REQUIRED`, không false-connected.
  - User skip runtime question → degrade (không target note), step vẫn chạy bằng source còn lại.
  - Provider config stale → preflight báo re-configure.

## 8. Rollout and Fallback

- rollout order:
  - Ship `P-5` refactor (behavior-preserving cho Drive) → connection/OAuth `P-1` → provider-config `P-2` → source + UI `P-3/P-4` sau flag internal → runtime question + prompt-note → use-case flows.
- fallback path:
  - Jira source lỗi/down → warning + package dựng từ source còn lại (degrade-mềm CP-44 `F-3`).
  - Nếu remote-MCP transport chưa ổn định → dùng REST diagnostic path (CP-05-01) chỉ cho Test Console/pick, không cho AI step (theo `Q-2`).
- monitoring:
  - Log: Jira source enabled/priority, thời gian resolve target, target đã chọn (id, không secret), preflight fail reason, provider-config status.

## 9. Risks

- `R-1` **Remote-MCP transport chưa có code.** Không có Go path nào nói remote-MCP hôm nay (agent-verified). Mitigation: `P-2` provider-config injection là phần chính; runner không cần tự nói remote-MCP nếu provider CLI làm client; chỉ `Q-2` (pick list) có thể cần REST.
- `R-2` **Single-adapter đè nhau.** Không refactor `P-5` → Jira ghi đè Drive adapter. Mitigation: `P-5` là blocker, có regression test Drive.
- `R-3` **Prompt hardcode `google_drive`.** Không generalize `P-6` → Jira instruction không được inject. Mitigation: refactor + test per-MCP.
- `R-4` **Latency/treo tại runtime.** OAuth/Atlassian chậm. Mitigation: preflight sớm + timeout; target note không block collect path.
- `R-5` **Secret leak.** Token vào Supabase/log. Mitigation: keyring-only (SD-11 §6), redact log, `config_encrypted` không chứa token.
- `R-6` **Determinism erosion.** Jira "search" dễ thành fuzzy. Mitigation: target note giới hạn issue/sprint tường minh; `Deterministic()==true`; giữ dòng `No vector retrieval used`.
- `R-7` **Divergence CP-05-01.** Token path vs OAuth path gây nhầm. Mitigation: `Q-4` chốt vai trò CP-05-01 = fallback; ghi rõ trong doc.

## 10. Definition of Done

- [ ] `DOD-1` `jira.issue` và `jira.sprint` (hai source độc lập) đăng ký trong `ContextSourceRegistry`, opt-in, `Deterministic()==true`, có test; xuất hiện trong `contextSourceOptions` UI khớp registry.
- [ ] `DOD-2` Connect Jira qua Atlassian remote MCP + OAuth end-to-end; status `connected`; token chỉ ở keyring, không ở Supabase/log.
- [ ] `DOD-3` Adapter/source dispatch refactor (`P-5`) xong; Drive vẫn chạy nguyên (regression xanh).
- [ ] `DOD-4` Provider-config injection cho ≥1 provider (Claude hoặc Codex) trỏ Atlassian remote MCP; stale detection có test.
- [ ] `DOD-5` `context_artifact.v1` instance bật `jira.issue` bind vào step → runtime hỏi target → prompt handoff có target note bounded; **không** dump full ticket vào `FlowContextPackage`.
- [ ] `DOD-6` Prompt-injection + preflight tổng quát hóa (không còn hardcode `google_drive`); Jira preflight chặn step khi chưa connect/OAuth stale với lỗi rõ.
- [ ] `DOD-7` Guard bất biến: no-vector/deterministic, `PackageID` không đổi, `workflow_steps` không nhận metadata (BUG-236), source ngoài chỉ MCP đã connect (CP-44 `P-7`).
- [ ] `DOD-8` Ít nhất một use-case flow (Investigate Bug hoặc Analyze Sprint) chạy live end-to-end; Test Console `jira_list_bugs` trả dữ liệu thật.
- [ ] `DOD-9` Read-only v1 enforced; write tools disabled (theo `P-7`).
