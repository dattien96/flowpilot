# Task-233: Telegram Output Write-Contract, Verify Gate, And Approval

## Metadata

- Document ID: `Task-233`
- Title: `Telegram Output Write-Contract, Verify Gate, And Approval`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-13`
- Last Updated: `2026-07-15`
- Parent Documents: [CP-05-05: Telegram MCP As An Output Notification Artifact](../../07-Coding-Plan/done/CP-05-05-Tele-Mcp.md) (`P-3`, `P-4`, `P-5`, `P-6`, `P-7`)
- Child Documents: `None`
- Related Documents: [Task-232: Telegram Output Artifact Type And Bot-API Proxy MCP](./Task-232-Telegram-Output-Artifact-Type-And-Bot-API-Proxy-MCP.md) (blocker), [Task-223: File Artifact Output Contract](../done/Task-223-File-Artifact-Output-Contract-And-Review-Input-Chain.md) (mẫu OUTPUT write-contract + gate), [Task-227: Generalize MCP Prompt-Injection And Preflight](./Task-227-Generalize-MCP-Prompt-Injection-And-Preflight.md)
- Replaces: `None`
- Tags: `mcp`, `telegram`, `output-artifact`, `write-contract`, `flow-gate`, `approval`, `ui`

## AI Quick View

### Summary

- Cắm Telegram OUTPUT vào **seam THẬT** `composeFlowNodeAgentPrompt` ([artifact_type_registry.go:515](../../../apps/local-runner/internal/runner/artifact_type_registry.go:515)) — thêm `appendTelegramOutputPrompt` cạnh `appendRequiredOutputArtifactPrompt`, dispatch `Direction=="output" && ArtifactTypeID==ArtifactTypeTelegram`. **KHÔNG** dùng `ArtifactResolver` interface (đã verify: zero production caller).
- Write-contract prompt (mirror Task-223 + `InjectRequiredMcpInstructions`): chỉ thị AI gọi tool `send_message` của MCP server `telegram` với final content tới `chatId`, kèm failure codes.
- Verify gate mới `r-artifact-telegram-sent` trong `flowgate/rules.go` ([DefaultRules](../../../apps/local-runner/internal/flowgate/rules.go:122)) + case trong `evaluate.go`, verify bằng **tool-call detection** (mirror `detectMcpToolUsed` [provider_driven_mcp.go:191](../../../apps/local-runner/internal/runner/provider_driven_mcp.go:191)), **không** `os.Stat`; wire vào `runChildArtifactOutputGate` allowlist ([gate_hook.go:259](../../../apps/local-runner/internal/runner/gate_hook.go:259)).
- Send là hành động outward-facing không hoàn tác → **auto-approve-gated** mặc định (`telegramCredential.AutoApprove`, keyring); user bật **Allow Telegram sends** trong Settings trước khi gửi. UI: `isTelegramType` config editor + compat OUTPUT-only.

### Current Ask

- Cho step cuối bind `telegram.v1` OUTPUT gửi final noti: prompt write-contract + verify gate + approval + UI config.

### Key Decisions

- `T-1` Seam = `composeFlowNodeAgentPrompt` (hardcoded type-id dispatch), không phải `ArtifactResolver` chết.
- `T-2` Verify = **`message_id` thật trong tool-call response** (`Q-1` resolved 2026-07-13), không phải file existence và không chỉ "tên tool được gọi". Gate đọc response của `send_message` (do Task-232 đảm bảo trả `message_id`), reprompt tối đa `maxFlowGateReprompts` khi thiếu/lỗi; tránh false-positive khi AI chỉ quote guidance mà không thật sự gọi tool (bài học CP-05-03 §12.3).
- `T-3` Auto-approve-gated mặc định (`autoApprove=false`); user bật integration auto-approve trong Settings trước khi `send_message` thực sự gửi (harness action policy: gửi message cần permission).
- `T-4` `telegram.v1` chỉ bind OUTPUT — nới `compatibleArtifactInstancesFor` ([WorkflowsSettings.tsx:285](../../../apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx:285)).

### Constraints

- Blocker: Task-232 (type + proxy MCP).
- Không phá `context_artifact`/`file_artifact` path; Telegram type độc lập.
- Binding ở `step_definitions` (BUG-236); secret boundary SD-11 §6.
- SD-11 §3.3 đã amend (2026-07-13).

### Open Questions

- `Q-1` **(RESOLVED 2026-07-13 → cần `message_id`)** Tool-call detection đơn thuần không đủ tin cậy (không phân biệt lỗi Telegram API hay AI chỉ quote guidance). Proxy (Task-232) trả `message_id` thật; gate verify sự tồn tại `message_id` hợp lệ trong tool-response (CP-05-05 `Q-4`).
- `Q-2` `messageTemplate`: field cố định hay AI tự soạn từ output step trước (CP-05-05 `Q-5`)?

### Source Refs

- `CP-05-05` `P-3/P-4/P-5/P-6/P-7`, `Q-4`, `Q-5`.
- `SD-23` `D-8` (OUTPUT write-contract), `D-11` (prompt-assembly seam).
- Task-223 (file OUTPUT write-contract + `r-artifact-output` gate mẫu).
- current code: `artifact_type_registry.go` (`composeFlowNodeAgentPrompt`, `appendRequiredOutputArtifactPrompt`), `flowgate/rules.go` + `evaluate.go`, `gate_hook.go` (`runChildArtifactOutputGate`), `provider_driven_mcp.go` (`detectMcpToolUsed`), `telegram_proxy_mcp.go` (`autoApprove` gate), `fileArtifactConfig.ts`, `WorkflowsSettings.tsx`.

## 1. Goal

Cho step cuối flow bind `telegram.v1` OUTPUT: AI gửi final notification qua Telegram MCP tool (write-contract), có approval, và flow-gate xác nhận đã gửi.

## 2. Parent Links

- coding plan: `CP-05-05` (`P-3/P-4/P-5/P-6/P-7`)
- tech design: `SD-23` (`D-8/D-11`), `SD-11` (§3.3 amended)
- system spec: `SS-14` (US-10/AC-17)
- specific upstream ids: `CP-05-05 P-3`, `P-4`, `P-5`

## 3. Trigger

Sau khi type + proxy MCP sẵn (Task-232), đây là slice biến `telegram.v1` thành OUTPUT dùng được: prompt contract + verify + approval + UI.

## 4. Exact Change

- `T-1` `appendTelegramOutputPrompt` trong `composeFlowNodeAgentPrompt`; write-contract (server `telegram`, tool `send_message`, chatId, template, failure codes) qua contract Task-227.
- `T-2` Rule pair `r-artifact-telegram-sent` (trigger `required_telegram_notification_missing`) trong `DefaultRules()` + case `evaluate.go` verify **`message_id` hợp lệ trong tool-call response** của `send_message` (không chỉ tên tool được gọi); `TurnResult` field mới mang `message_id`/lỗi API nếu có; wire `runChildArtifactOutputGate` allowlist; reprompt khi thiếu `message_id` hoặc Telegram API trả lỗi.
- `T-3` Auto-approve gate cho send (`telegramProxyMcpServer.autoApprove`, keyring); default refuse (`MCP_TOOL_APPROVAL_REQUIRED`); user bật trong MCP Settings.
- `T-4` UI: `telegramArtifactConfig.ts` + branch `isTelegramType` trong `ArtifactsTabContent` (chọn integration + chatId + template); `onCreateNew` seed; save normalizer; `compatibleArtifactInstancesFor` OUTPUT-only.
- `T-5` ~~Wiring built-in/pack flow~~ **out of scope** (2026-07-15): user tự bind `telegram.v1` OUTPUT trong workflow editor.

## 5. Touched Areas

- files: `artifact_type_registry.go` (`appendTelegramOutputPrompt`, dispatch), `flowgate/rules.go` + `evaluate.go`, `gate_hook.go` (allowlist + `TurnResult` field), `mcp_prompt_instructions.go` (telegram block), `provider_driven_mcp.go` (detection), `telegram_proxy_mcp.go` (`autoApprove`), mới `telegramArtifactConfig.ts`, `WorkflowsSettings.tsx` (`isTelegramType` + compat)
- modules: runner prompt-assembly + gate + auto-approve, desktop artifact authoring
- routes: none (auto-approve via keyring + integration test/toggle)
- tables: none (dùng `artifact_types`/`artifact_instances`/`step_artifact_bindings` sẵn có)

## 6. Acceptance Check

- Bind `telegram.v1` OUTPUT vào step cuối, run flow với auto-approve ON → AI gọi `send_message` → tin xuất hiện trong channel; gate đọc `message_id` từ tool-response, xác nhận đã gửi thật.
- AI chỉ mô tả mà không gọi tool → gate reprompt (không có tool-call nào để lấy `message_id`); AI quote guidance → không false-positive; Telegram API trả lỗi (chat_id/token sai) → gate phát hiện thiếu `message_id` hợp lệ → reprompt, không pass giả.
- Auto-approve OFF → `send_message` trả `MCP_TOOL_APPROVAL_REQUIRED`, không gửi.
- `telegram.v1` chỉ bind được OUTPUT (UI + runner).
- `go test ./apps/local-runner/internal/...` xanh.

## 7. Out of Scope

- Type seed + proxy MCP + provider-config (Task-232).
- Read/context Telegram (không có; chỉ output).
- Native Go fallback implementation (chỉ giữ làm option `Q-3` của CP-05-05).

## 8. Completion Notes

- result: **done** — v1 send gate = integration **auto-approve** toggle only (keyring); per-send approval queue removed 2026-07-15 (CA-318, user live-verified).
  - **Prompt seam (`T-1`, seam confirmed real, not the dead `ArtifactResolver`):** `apps/local-runner/internal/runner/artifact_type_registry.go` gained `requiredTelegramOutputTargets(node)` (extracts required `telegram.v1` OUTPUT bindings' `chatId`/`messageTemplate` from `config_json`, mirrors `requiredFileArtifactOutputPaths`) and `appendTelegramOutputPrompt` (the write-contract text: "you MUST call `send_message` on the `telegram` MCP server... the response includes a `message_id`; do not claim success without it"), wired into `composeFlowNodeAgentPrompt` alongside the existing file-artifact injections — confirmed via `TestComposeFlowNodeAgentPromptIncludesTelegramWriteContract` that this is the real production seam, not the unused `ArtifactTypeRegistry`/`ArtifactResolver` interface.
  - **Verify gate (`T-2`, `message_id`-based per CP-05-05 `Q-4`):** `apps/local-runner/internal/flowgate/rules.go` gained `TurnResult.RequiredTelegramSends []string` + rule `r-artifact-telegram-sent` (trigger `required_telegram_send_missing`). `evaluate.go` gained `MissingTelegramSends(finalMessage, required)` — scans `FinalMessage` for `message_id` occurrences via regex, requiring **at least as many occurrences as required targets** (documented v1 simplification: text-scanning can't reliably attribute a specific match to a specific chat id when 2+ Telegram targets are bound in one turn — a future iteration would need structured tool-call/response logs instead of `FinalMessage` text). `gate_hook.go` gained `requiredTelegramSendsForRun` + wired both the `TurnResult` field and the new rule ID into `runChildArtifactOutputGate`'s allowlist, exactly mirroring the file-artifact-output gate's shape.
  - **Approval (`T-3`):** `telegramProxyMcpServer.autoApprove` (keyring-gated, default `false`) — `send_message` refuses with `MCP_TOOL_APPROVAL_REQUIRED` until user enables **Allow Telegram sends** in MCP Settings. `telegramCredential.AutoApprove` + `IntegrationConnectionRequest.TelegramAutoApprove` thread from connection flow through spawned proxy via `RunTelegramProxyMcpServer`. A per-send approval queue (`telegram_proxy_approval.go`) was built then **removed** (CA-318): product decision keeps only the config toggle.
  - **UI (`T-4`):** `WorkflowsSettings.tsx` gained `isTelegramType` editor (chat/channel id + optional message template, mirrors the file/context type editors), seeded via `onCreateNew`, and a catalog description note for `telegram.v1`. `compatibleArtifactInstancesFor` gained a `direction` parameter so `telegram.v1` instances **only** appear in OUTPUT pickers, never INPUT (there's nothing to read back from a sent message) — wired at the one call site that already knew its direction.
  - **Migration fix:** found `telegram.v1`'s `config_schema` (from Task-232) required an unused `integrationId` field that neither the UI nor the Go consumer (`requiredTelegramOutputTargets`, which only reads `chatId`/`messageTemplate`) ever populates — the actual runtime model is "one connected Telegram integration per workspace" (same as Jira/Firebase), so a per-instance `integrationId` was dead weight inherited from an earlier draft of the schema. Removed it from the migration so the schema matches what's actually implemented.
  - Tests: `telegram_output_prompt_test.go` (5 — required-targets extraction, missing-chatId skip, prompt bounded content, no-op without binding, `composeFlowNodeAgentPrompt` wiring); `flowgate/telegram_rules_test.go` (6 — no-targets no-op, message_id-present satisfies, missing-evidence flags violation, per-target message_id-count requirement, `Evaluate` integration satisfied/violated); updated `flowgate_test.go`'s `TestDefaultRules` (15→16 rules, inserted `r-artifact-telegram-sent` in its correct position) since it asserts an exact rule count/order; added `TestTelegramProxyCallToolRequiresAutoApprove` + updated 3 existing proxy tests to pass `autoApprove: true` now that the default posture refuses sends. `go build ./...` clean; `go vet` clean; `go test ./internal/flowgate/...` — 132 pass (whole package, confirming the `TestDefaultRules` update didn't break anything else depending on rule count/order); `go test ./internal/runner/... -run 'Telegram|telegram'` — 23 pass; full suite `go test ./internal/runner/...` — 1466 passed / 16 failed / 18 skipped, same 16 pre-existing/unrelated failure names as every prior task in this series (zero new regressions across all of Task-226→233). `tsc --noEmit` in `apps/desktop-flowpilot` clean.
  - **Live send (user-verified 2026-07-15):** with auto-approve enabled in Settings, real `send_message` delivers to Telegram channel.
- follow-ups: (1) `MissingTelegramSends`'s per-target attribution is approximate (count-based, not identity-based) — revisit if multiple simultaneous Telegram targets in one turn becomes a real use case; (2) built-in/pack flow with final `telegram.v1` OUTPUT step is **won't-do** — manual workflow wiring is the supported path.
- upstream docs updated: CP-05-05 `DOD-6`/`P-8`/`DOD-7` (CA-318); per-send queue references removed.
