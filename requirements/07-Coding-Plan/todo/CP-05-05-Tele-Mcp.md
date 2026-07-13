# CP-05-05: Telegram MCP As An Output Notification Artifact

## Metadata

- Document ID: `CP-05-05`
- Title: `Telegram MCP As An Output Notification Artifact`
- Phase: `coding_plan`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-13`
- Last Updated: `2026-07-13`
- Parent Documents: [SD-11: MCP Connection Flows](../../06-System-Tech-Design/SD-11-MCP-Connection-Flows.md), [SD-23: Generic Artifact Framework](../../06-System-Tech-Design/SD-23-Generic-Artifact-Framework.md)
- Child Documents: [Task-232: Telegram Output Artifact Type And Bot-API Proxy MCP](../../08-Task/todo/Task-232-Telegram-Output-Artifact-Type-And-Bot-API-Proxy-MCP.md) (`P-1`/`P-2`), [Task-233: Telegram Output Write-Contract, Verify Gate, And Approval](../../08-Task/todo/Task-233-Telegram-Output-Write-Contract-Verify-Gate-And-Approval.md) (`P-3`–`P-7`); prereq: [Task-227](../../08-Task/todo/Task-227-Generalize-MCP-Prompt-Injection-And-Preflight.md) (shared prompt refactor)
- Related Documents: [CP-45: Generic Artifact Types And Instances](../done/CP-45-Generic-Artifact-Types-And-Instances.md) (artifact framework — Telegram là artifact type OUTPUT thứ ba, cạnh `context_artifact.v1`/`file_artifact.v1`), [CP-44: Pluggable Context Source Registry](../done/CP-44-Pluggable-Context-Source-Registry.md), [CP-05-03: Google Drive MCP Current Implementation Notes](../done/CP-05-03-Driver-Mcp.md) (mẫu provider-CLI-owns-MCP + prompt-injection), [CP-05-06: Jira MCP](./CP-05-06-Jira-MCP.md) (sibling — input source), [CP-05-04: Firebase MCP](./CP-05-04-Firebase-Mcp.md) (sibling — input source), [Task-223: File Artifact Output Contract](../../08-Task/done/Task-223-File-Artifact-Output-Contract-And-Review-Input-Chain.md) (mẫu OUTPUT write-contract + gate)
- Replaces: `None`
- Tags: `mcp`, `telegram`, `artifact-type`, `output-artifact`, `notification`, `artifact-framework`, `flow-mode`, `side-effect`

## AI Quick View

### Summary

- CP-05-05 thêm **Telegram** vào artifact framework như một **artifact type OUTPUT mới** (`telegram.v1`) — khác Jira/Firebase (là INPUT context source). Một step cuối flow bind `telegram.v1` làm **output** để **bắn final notification** lên Telegram.
- Cơ chế bám đúng OUTPUT write-contract của `file_artifact.v1` (Task-223): seam thật là `composeFlowNodeAgentPrompt` ([artifact_type_registry.go:515](../../../apps/local-runner/internal/runner/artifact_type_registry.go:515)) chèn một "write contract" vào prompt producer, sau đó một **flow-gate verify**. Khác `file_artifact` ở chỗ **không có file để `os.Stat`** — Telegram là side-effect gửi tin, nên gate verify bằng **`message_id` thật trong tool-response** (`Q-4` resolved 2026-07-13), không chỉ "AI có gọi tool" (`detectMcpToolUsed` [provider_driven_mcp.go:191](../../../apps/local-runner/internal/runner/provider_driven_mcp.go:191) là không đủ tin cậy đứng riêng).
- **Quyết định (owner, 2026-07-13):** AI **tự gọi Telegram MCP tool** (`send_message`) trong turn của nó — **không** dùng native Go adapter. **SD-11 §3.3 đã amend** (2026-07-13) để hợp thức hóa (`Q-1` done).
- Vì AI là MCP client (CP-05-03 §11), cần một **Telegram MCP server** để provider CLI gọi được. Khảo sát (`Q-2` resolved 2026-07-13) chốt: **FlowPilot-owned Bot-API proxy** (mirror `google_drive_proxy_mcp.go`), expose đúng `send_message` và trả `message_id` — community MCP hoặc MTProto user-account (over-scoped) hoặc quá non (<25★).
- Gửi Telegram là **hành động outward-facing không thể hoàn tác** → phải đi qua **approval gate** (mirror `approval_policy.go`/`google_drive_proxy_approval.go`), mặc định cần user xác nhận trước khi gửi thật.

### Current Ask

- Thiết kế + lên kế hoạch triển khai `telegram.v1` như một OUTPUT artifact type, với prompt write-contract ("gửi final noti qua Telegram MCP tool"), verify gate bằng tool-call detection, và approval gate cho hành động gửi.

### Key Decisions

- `P-1` **Telegram là artifact type OUTPUT mới `telegram.v1`**, không phải context source. Thêm hằng `ArtifactTypeTelegram = "telegram.v1"` ([artifact_type_registry.go:15](../../../apps/local-runner/internal/runner/artifact_type_registry.go:15)) + seed row trong bảng `artifact_types` (category `"notify"`/`"action"`) qua migration mirror `20260709090000_add_artifact_types_catalog.sql`.
- `P-2` **Cắm vào seam THẬT, không dùng `ArtifactResolver` interface đang chết.** `ArtifactTypeRegistry`/`ArtifactResolver` tồn tại nhưng **không** được gọi ở production (agent-verified: zero production callers). Prompt-injection Telegram phải thêm `appendTelegramOutputPrompt` cạnh `appendRequiredOutputArtifactPrompt` ([artifact_type_registry.go:407](../../../apps/local-runner/internal/runner/artifact_type_registry.go:407)) trong `composeFlowNodeAgentPrompt`, dispatch theo `Direction=="output" && ArtifactTypeID==ArtifactTypeTelegram`.
- `P-3` **AI gọi Telegram MCP tool trong turn** (owner-chosen), mirror `InjectRequiredMcpInstructions` ([mcp_prompt_instructions.go:12](../../../apps/local-runner/internal/runner/mcp_prompt_instructions.go:12)): prompt chỉ thị AI gọi `send_message` tới chat/channel đã cấu hình với final content. **Lệch SD-11 §3.3 — cần amend (`Q-1`).**
- `P-4` **Verify gate = `message_id` trong tool-response** (`Q-4` resolved 2026-07-13), không phải file existence và không chỉ "tool-call detection" đơn thuần. Proxy `send_message` ([Task-232](../../08-Task/todo/Task-232-Telegram-Output-Artifact-Type-And-Bot-API-Proxy-MCP.md)) **phải trả `message_id`** thật từ Telegram Bot API trong response của tool call — đây là bằng chứng cứng rằng Telegram server đã nhận và gửi, phân biệt được "AI gọi tool nhưng Telegram trả lỗi" hoặc "AI chỉ nói đã gửi mà không gọi tool" khỏi "đã gửi thành công thật". Thêm rule pair trong `flowgate/rules.go` ([DefaultRules](../../../apps/local-runner/internal/flowgate/rules.go:122)) (vd `r-artifact-telegram-sent`, trigger `required_telegram_notification_missing`) + case trong `evaluate.go`; verify bằng scan tool-call **response** tìm `message_id` hợp lệ (không chỉ tìm tên tool được gọi, kiểu `detectMcpToolUsed`) thay `MissingRequiredFileArtifactOutputs`. Wire vào `runChildArtifactOutputGate` ([gate_hook.go:223](../../../apps/local-runner/internal/runner/gate_hook.go:223)) allowlist.
- `P-5` **Approval-gated irreversible send.** Gửi tin outward-facing → mặc định qua approval (mirror `google_drive_proxy_approval.go`), user confirm trước khi gửi thật; hỗ trợ mode "yolo/auto" tùy config như Drive write.
- `P-6` **Config theo instance:** `telegram.v1` `config_json` = `{ integrationId, chatId/channel, messageTemplate }`; validate theo `configSchema` (SD-23 `D-3`). Chọn connected Telegram integration (Surface ①).
- `P-7` **Type-compat: `telegram.v1` chỉ bind OUTPUT.** Nới `compatibleArtifactInstancesFor` ([WorkflowsSettings.tsx:285](../../../apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx:285)) để `telegram.v1` chỉ hiện ở slot OUTPUT, không hiện ở INPUT/context slot.

### Constraints

- Giữ artifact framework contract: type system-owned (code chỉ hardcode type), instance user-authored (`is_builtin=false` RLS), binding ở `step_definitions` (BUG-236). SD-23 `D-2/D-4`.
- **Secret boundary (SD-11 §6):** bot token chỉ ở runner keyring ([secret_store.go](../../../apps/local-runner/internal/runner/secret_store.go)); **không** vào Supabase `config_encrypted`, không log, không đưa vào prompt (chỉ chat id/channel + nội dung).
- **Security (harness action policy):** gửi message là hành động cần permission; mặc định approval-gated, không tự động gửi trừ khi user bật auto mode.
- Nguồn ngoài chỉ tới MCP đã connect (CP-44 `P-7`).
- Không phá `context_artifact`/`file_artifact` path hiện có; Telegram là type độc lập.

### Open Questions

- `Q-1` **(DONE 2026-07-13 — SD-11 §3.3 đã amend)** SD-11 §3.3 đã được cập nhật: Telegram = MCP-backed OUTPUT artifact (AI gọi Telegram MCP tool), native Go = fallback; giữ lại nguyên văn thiết kế cũ + ghi chú dated. Không còn treo upstream.
- `Q-2` **(RESOLVED 2026-07-13 → FlowPilot-owned Bot-API proxy)** Khảo sát: không có official Telegram MCP; community hoặc là MTProto user-account (over-scoped, ban risk) hoặc <25★ single-maintainer. Chốt: self-host proxy tối thiểu expose đúng `send_message` (wrap `sendMessage` Bot API), không `npx` package chưa vet, tool surface tối thiểu, không lộ user account. Xem [Task-232](../../08-Task/todo/Task-232-Telegram-Output-Artifact-Type-And-Bot-API-Proxy-MCP.md).
- `Q-3` **Cân nhắc lại native Go?** Native Go adapter (SD-11 gốc) quan sát/approval dễ hơn và không cần dựng MCP server; owner đã chọn MCP nhưng nếu `Q-2` cho thấy server chưa đủ tin cậy, có nên fallback native Go? Ghi lại để tái xét.
- `Q-4` **(RESOLVED 2026-07-13 → cần `message_id`)** Tool-call detection đơn thuần (chỉ biết "AI có gọi tool") không đủ tin cậy — không phân biệt được lỗi Telegram API (token/chat_id sai, rate limit) hay AI chỉ quote guidance mà không gọi tool thật (bài học false-positive `detectMcpFailureCode`, CP-05-03 §12.3) khỏi gửi thành công thật. Proxy `send_message` phải trả `message_id` từ Telegram Bot API response; gate verify sự tồn tại của `message_id` hợp lệ trong tool-response, không chỉ tên tool được gọi.
- `Q-5` Nội dung noti: template cố định (What/status/link) hay để AI tự soạn từ output các step trước? Ảnh hưởng `messageTemplate` schema.

### Source Refs

- `SD-11` §3.3 (Telegram — **sẽ amend**), §6 (secret boundary).
- `SD-23` `D-1` (three-layer), `D-8` (OUTPUT write-contract semantics), `D-11` (prompt-assembly seam; ArtifactResolver seam).
- `CP-45` Task-223 (file OUTPUT write-contract + `r-artifact-output` gate), `§11.4` (OUTPUT consume).
- `CP-05-03` §11.10 (prompt augmentation), §11.15/CP-29 (write approval model).
- current code: `artifact_type_registry.go` (`composeFlowNodeAgentPrompt`, `appendRequiredOutputArtifactPrompt`, `ArtifactType*` consts), `flowgate/rules.go` + `evaluate.go` (gate rules), `gate_hook.go` (`runChildArtifactOutputGate`), `mcp_prompt_instructions.go` (`InjectRequiredMcpInstructions`), `provider_driven_mcp.go` (`detectMcpToolUsed`), `approval_policy.go`, `fileArtifactConfig.ts`, `WorkflowsSettings.tsx` (`compatibleArtifactInstancesFor`), `settingsHelpers.ts` (`providerFields.telegram`).

## 1. Goal

Cho step cuối của một flow khả năng **bắn final notification lên Telegram** như một OUTPUT artifact có type rõ ràng (`telegram.v1`), bằng cách chỉ thị AI gọi Telegram MCP tool trong turn của nó (write-contract trong prompt) rồi verify hành động đã xảy ra qua flow-gate — tái dùng đúng mô hình OUTPUT của `file_artifact.v1` (Task-223) nhưng thay "file existence" bằng "tool-call detection", và thêm approval cho hành động gửi không thể hoàn tác.

## 2. Input Documents

- `requirements/06-System-Tech-Design/SD-11-MCP-Connection-Flows.md`
- `requirements/06-System-Tech-Design/SD-23-Generic-Artifact-Framework.md`
- `requirements/07-Coding-Plan/done/CP-45-Generic-Artifact-Types-And-Instances.md`
- `requirements/08-Task/done/Task-223-File-Artifact-Output-Contract-And-Review-Input-Chain.md`
- `requirements/07-Coding-Plan/done/CP-05-03-Driver-Mcp.md`
- `apps/local-runner/internal/runner/artifact_type_registry.go`
- `apps/local-runner/internal/flowgate/rules.go`, `evaluate.go`
- `apps/local-runner/internal/runner/gate_hook.go`
- `apps/local-runner/internal/runner/mcp_prompt_instructions.go`, `provider_driven_mcp.go`
- `apps/desktop-flowpilot/src/components/settings/fileArtifactConfig.ts`, `WorkflowsSettings.tsx`

## 3. Implementation Strategy

- overall approach:
  - Định nghĩa type `telegram.v1` (DB + Go const). Prompt-assembly: khi step có OUTPUT binding `telegram.v1`, chèn write-contract "trước khi kết thúc, gửi final notification qua Telegram MCP tool `send_message` tới chat X". Sau turn, gate verify tool-call đã xảy ra; nếu thiếu → reprompt (mirror `runChildArtifactOutputGate`).
  - Dựng/chọn Telegram MCP server + provider-config injection để CLI gọi tool được. Wrap send bằng approval gate.
- sequencing logic:
  - `P-1` type + migration → `Q-1` amend SD-11 → `Q-2` chọn MCP server → `P-2` provider-config → `P-3` prompt write-contract seam → `P-4` verify gate → `P-5` approval → `P-6/P-7` UI config + compat → use-case → tests.
- dependencies:
  - CP-45 (done) cho artifact framework + OUTPUT gate pattern.
  - Independent với Jira/Firebase (khác shape) nhưng dùng chung `integrations` `telegram` + keyring + prompt-injection helpers.

## 4. Work Breakdown

- `P-1` **Type `telegram.v1`.**
  - `ArtifactTypeTelegram` const ([artifact_type_registry.go:15](../../../apps/local-runner/internal/runner/artifact_type_registry.go:15)); migration seed `artifact_types` (category `notify`, `config_schema` = `{ integrationId, chatId, messageTemplate }`, `consumer_hints '{"promptSection":"notify"}'`), RLS select-only.
- `P-2` **Telegram MCP server + provider-config injection.**
  - Chọn server (`Q-2`); `telegram_mcp_provider_config.go` mirror `google_drive_mcp_provider_config.go` cho codex/gemini/claude/grok + Claude `--strict-mcp-config` extra server; env inject bot token từ keyring.
- `P-3` **Prompt write-contract seam.**
  - `appendTelegramOutputPrompt` trong `composeFlowNodeAgentPrompt` ([artifact_type_registry.go:515](../../../apps/local-runner/internal/runner/artifact_type_registry.go:515)); dispatch `Direction=="output" && ArtifactTypeID==ArtifactTypeTelegram`; nội dung theo `InjectRequiredMcpInstructions` style: server name `telegram`, tool `send_message`, chat id, template, failure codes ("nếu MCP unavailable → dừng, báo `MCP_UNAVAILABLE`").
- `P-4` **Verify gate.**
  - Rule pair `r-artifact-telegram-sent` trong `DefaultRules()` ([rules.go:122](../../../apps/local-runner/internal/flowgate/rules.go:122)); case `required_telegram_notification_missing` trong `evaluate.go` verify bằng tool-call scan (mirror `detectMcpToolUsed`), điền `TurnResult` field mới; wire vào `runChildArtifactOutputGate` allowlist ([gate_hook.go:259](../../../apps/local-runner/internal/runner/gate_hook.go:259)); reprompt tối đa `maxFlowGateReprompts`.
- `P-5` **Approval gate.**
  - Send đi qua approval bridge (mirror `google_drive_proxy_approval.go`); mặc định pending-approval; user confirm → gửi thật; auto/yolo mode tùy config.
- `P-6` **UI config.**
  - `telegramArtifactConfig.ts` mirror `fileArtifactConfig.ts`; branch `isTelegramType` trong `ArtifactsTabContent` (editor: chọn connected Telegram integration + chat id + template); `onCreateNew` seed config; save normalizer.
- `P-7` **Type-compat OUTPUT-only.**
  - Nới `compatibleArtifactInstancesFor` ([WorkflowsSettings.tsx:285](../../../apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx:285)) để `telegram.v1` chỉ hiện ở slot OUTPUT.
- `P-8` **Use-case wiring:** step cuối built-in/pack flow bind `telegram.v1` OUTPUT gửi final summary.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/artifact_type_registry.go` (`ArtifactTypeTelegram`, `appendTelegramOutputPrompt`, seam dispatch)
  - `apps/local-runner/internal/flowgate/rules.go`, `evaluate.go` (verify gate)
  - `apps/local-runner/internal/runner/gate_hook.go` (allowlist + `TurnResult` field)
  - `apps/local-runner/internal/runner/mcp_prompt_instructions.go` (Telegram MCP-usage block)
  - `apps/local-runner/internal/runner/provider_driven_mcp.go` (tool-call detection cho telegram)
  - mới: `apps/local-runner/internal/runner/telegram_mcp_provider_config.go` (+ test), approval wiring
  - `apps/local-runner/internal/runner/runner.go` (Telegram backend spec + keyring cred + verify)
  - mới: `apps/desktop-flowpilot/src/components/settings/telegramArtifactConfig.ts`
  - `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx` (`isTelegramType` branch + compat OUTPUT-only)
  - `apps/desktop-flowpilot/src/components/settings/settingsHelpers.ts` (Telegram fields)
  - supabase migration: `*_add_telegram_artifact_type.sql`
- modules: artifact framework (type/instance/binding), runner prompt-assembly + gate, MCP provider-config, approval, desktop settings.
- database: `artifact_types` (+1 seed row `telegram.v1`); dùng `integrations` `telegram` sẵn có. Không sửa `artifact_instances`/`step_artifact_bindings` schema.
- external systems: Telegram Bot API qua MCP server (`Q-2`).

## 6. Data or Migration Steps

- schema:
  - Thêm seed row `telegram.v1` vào `artifact_types` (idempotent upsert, mirror CP-45 Task-198 `DOD-5`). Không bảng mới.
- data backfill:
  - Không (user tạo instance thủ công qua Artifacts tab).
- config updates:
  - Provider MCP config per-account-home; `telegram.v1` instance `config_json` = integrationId + chatId + template.

## 7. Validation Plan

- tests to add:
  - `TestTelegramArtifactTypeSeeded` — type có trong catalog, read-only.
  - `TestAppendTelegramOutputPromptContract` — OUTPUT binding chèn write-contract (server `telegram`, tool `send_message`, chat id, template, failure codes); INPUT/không-binding không chèn.
  - `TestTelegramCompatOutputOnly` — `telegram.v1` chỉ bind OUTPUT (UI + runner).
  - `TestTelegramGateDetectsMissingSend` / `...PassesWhenToolCalled` — verify bằng tool-call detection; reprompt khi thiếu; không false-positive khi chỉ quote guidance (mirror CP-05-03 §12.3).
  - `TestTelegramSendRequiresApproval` — mặc định pending-approval; auto mode gửi thẳng.
  - `TestEnsureTelegramMcpProviderConfig*` — write/stale.
  - `TestTelegramBotTokenNotInPromptOrSupabase` — secret boundary.
- manual checks:
  - Connect Telegram (bot token + channel) → `connected`; tạo `telegram.v1` instance; bind OUTPUT vào step cuối; run flow → AI gọi `send_message` → approval prompt → confirm → tin xuất hiện trong channel; gate xác nhận đã gửi.
- failure cases:
  - Chưa connect / MCP unavailable → step báo `MCP_UNAVAILABLE`, gate không pass giả.
  - User từ chối approval → không gửi; step báo rõ.
  - AI chỉ mô tả mà không gọi tool → gate reprompt.

## 8. Rollout and Fallback

- rollout order:
  - Amend SD-11 (`Q-1`) → type + migration → MCP server chọn (`Q-2`) → provider-config → prompt seam → gate → approval → UI → use-case (flag internal).
- fallback path:
  - Nếu Telegram MCP server không đủ tin cậy (`Q-3`) → tái xét native Go adapter; giữ interface OUTPUT ổn định để đổi backing không phá binding.
  - MCP down → step fail rõ (final noti là OUTPUT required); optional binding thì degrade-mềm (warning).
- monitoring:
  - Log: telegram binding, send attempt (chat id, không token/không full body nhạy cảm), approval decision, gate pass/fail.

## 9. Risks

- `R-1` ⚠️ **Divergence SD-11 §3.3 chưa amend.** Downstream redefine upstream. Mitigation: `Q-1` amend SD-11 (và SD-23 nếu cần) trước Task; không land khi upstream còn nói ngược.
- `R-2` **Telegram-as-MCP chưa tồn tại.** Cần dựng/chọn server (`Q-2`). Mitigation: khảo sát community MCP + PoC; giữ tùy chọn FlowPilot proxy hoặc native Go fallback (`Q-3`).
- `R-3` **Gửi không thể hoàn tác.** Gửi nhầm/spam channel. Mitigation: approval-gated mặc định (`P-5`); auto mode phải opt-in tường minh.
- `R-4` **Verify tool-call không tin cậy nếu chỉ check tên tool.** False positive/negative. Mitigation: `Q-4` resolved — bắt buộc `message_id` thật trong tool-response; tránh bẫy quote-guidance (CP-05-03 §12.3).
- `R-5` **Secret leak.** Bot token vào prompt/log/Supabase. Mitigation: keyring-only, chỉ chat id + content vào prompt.
- `R-6` **Nhầm seam.** Cắm vào `ArtifactResolver` interface đang chết thay vì seam thật. Mitigation: `P-2` chỉ rõ seam thật là `composeFlowNodeAgentPrompt`; có test đường production.

## 10. Definition of Done

- [x] `DOD-1` SD-11 §3.3 **đã amend** (2026-07-13) hợp thức hóa AI-gọi-MCP path; native Go giữ làm fallback (`Q-1` done).
- [x] `DOD-2` `telegram.v1` artifact type seeded trong `artifact_types`, read-only, hiện trong catalog tab Artifacts; instance user tạo được với config. — Task-232 migration; config thực tế = `{ chatId, messageTemplate }` (bỏ `integrationId` — dead field không ai đọc, sửa lại cho khớp model "1 connection/workspace" giống Jira/Firebase, xem Task-233 completion notes).
- [x] `DOD-3` Telegram MCP server chốt (`Q-2` → tự viết FlowPilot-owned Bot-API proxy) + provider-config injection cho Claude; bot token chỉ ở keyring. — Task-232, test thật với `httptest` (không phải live Telegram).
- [x] `DOD-4` OUTPUT binding `telegram.v1` chèn write-contract vào prompt qua seam thật (`composeFlowNodeAgentPrompt`), không dùng `ArtifactResolver` chết; chỉ bind OUTPUT (compat). — Task-233, có test xác nhận đúng seam production.
- [x] `DOD-5` Verify gate `r-artifact-telegram-sent` bằng `message_id` thật trong tool-response (không chỉ tên tool được gọi); thiếu/lỗi API → reprompt; không false-positive khi quote guidance. — Task-233, `telegram_rules_test.go`.
- [x] `DOD-6` Approval-gated send: mặc định user confirm trước khi gửi thật; auto mode opt-in. — **Đã build approval-queue thật** (`telegram_proxy_approval.go`), mirror chính xác cơ chế `google_drive_proxy_approval.go` đã ship: lần gọi `send_message` đầu tiên cho một (run, step, process, chatId, text) cụ thể luôn trả về **pending** (không gửi gì); user approve/reject qua HTTP API thật (`/telegram-proxy-approvals`, `/telegram-proxy-approvals/{id}/decision`) **và UI thật** (panel "Pending Telegram Approvals" trong MCP Servers settings, nút Approve/Reject); AI retry đúng lệnh cũ mới thực sự gửi (approved) hoặc nhận rejection sạch (rejected); gọi lại lần 3 chỉ replay kết quả đã ghi, không gửi trùng. Test thật: `telegram_proxy_approval_test.go` (pending→approve→send, pending→reject→no-send, no-double-send, khác text→khác approval). **Giới hạn còn lại (đã ghi nhận, không phải thiếu sót riêng của Telegram):** việc thread `workflowRunId`/`workflowStepRunId`/`processKey` vào **live Claude turn** (qua `--strict-mcp-config` ephemeral merge) là khoảng trống **kiến trúc đã tồn tại sẵn ở chính Google Drive** — `flowpilotClaudeExtraMCPServers` không nhận các id này ở call site hiện tại. Sửa nó đụng vào closure dùng chung bởi cả Claude lẫn Grok (`extraMCPServers`), rủi ro regression 2 provider đã ship với confidence còn lại trong phiên này — cố ý không đụng. Không có scope env vars → rơi về cờ `autoApprove` tĩnh (không regression so với bản trước, chỉ là fallback path).
- [ ] `DOD-7` **Chưa xong.** End-to-end: bind `telegram.v1` OUTPUT vào step cuối → run flow → AI gửi noti thật lên channel (qua approval) → gate xác nhận. — chưa có bot token thật để chạy live.
- [x] `DOD-8` Guard: type system-owned, binding ở `step_definitions` (BUG-236), secret boundary (SD-11 §6), source ngoài chỉ MCP đã connect. — kế thừa framework sẵn có, không đổi; secret boundary có test riêng (`stripSecretFields`).
