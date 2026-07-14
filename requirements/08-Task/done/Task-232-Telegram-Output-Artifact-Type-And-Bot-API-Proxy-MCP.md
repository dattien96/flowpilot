# Task-232: Telegram Output Artifact Type And Bot-API Proxy MCP

## Metadata

- Document ID: `Task-232`
- Title: `Telegram Output Artifact Type And Bot-API Proxy MCP`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-13`
- Last Updated: `2026-07-15`
- Parent Documents: [CP-05-05: Telegram MCP As An Output Notification Artifact](../../07-Coding-Plan/todo/CP-05-05-Tele-Mcp.md) (`P-1`, `P-2`), [SD-11: MCP Connection Flows](../../06-System-Tech-Design/SD-11-MCP-Connection-Flows.md) (§3.3, amended 2026-07-13), [SD-23: Generic Artifact Framework](../../06-System-Tech-Design/SD-23-Generic-Artifact-Framework.md)
- Child Documents: `None`
- Related Documents: [CP-05-03: Google Drive MCP Current Implementation Notes](../../07-Coding-Plan/done/CP-05-03-Driver-Mcp.md) (mẫu FlowPilot-owned proxy MCP `google_drive_proxy_mcp.go`), [Task-197: Artifact Type Catalog And Schema](../done/Task-197-Artifact-Type-Catalog-And-Schema.md) (mẫu seed type), [Task-233: Telegram Output Write-Contract, Verify Gate, And Approval](./Task-233-Telegram-Output-Write-Contract-Verify-Gate-And-Approval.md)
- Replaces: `None`
- Tags: `mcp`, `telegram`, `artifact-type`, `output-artifact`, `bot-api-proxy`, `notification`

## AI Quick View

### Summary

- Thêm artifact type **`telegram.v1`** (category `notify`) vào catalog: hằng `ArtifactTypeTelegram` ([artifact_type_registry.go:15](../../../apps/local-runner/internal/runner/artifact_type_registry.go:15)) + migration seed `artifact_types` (mirror `20260709090000_add_artifact_types_catalog.sql`), `config_schema = { integrationId, chatId, messageTemplate }`, RLS select-only.
- Dựng **FlowPilot-owned Telegram Bot-API proxy MCP** (mirror `google_drive_proxy_mcp.go`) expose đúng **một** tool `send_message` (wrap `https://api.telegram.org/bot<TOKEN>/sendMessage`). Khảo sát 2026-07-13 chốt: community Telegram MCP hoặc là MTProto user-account (over-scoped, ban risk) hoặc <25★ single-maintainer → self-host an toàn hơn, tool surface tối thiểu, không lộ user account.
- **Tool-response bắt buộc trả `message_id`** thật từ Telegram Bot API (`Q-4` của CP-05-05, resolved 2026-07-13) — đây là bằng chứng cứng "đã gửi thành công", để Task-233 verify gate không chỉ dựa vào "AI có gọi tool" mà biết chắc Telegram server đã confirm nhận tin.
- Provider-config injection cho ≥1 provider CLI + Claude `--strict-mcp-config` extra server; bot token env-inject từ keyring.

### Current Ask

- Định nghĩa `telegram.v1` type + dựng Telegram Bot-API proxy MCP (`send_message` only) + provider-config để CLI gọi được.

### Key Decisions

- `T-1` Type system-owned `telegram.v1`, category `notify`; user tạo instance (`is_builtin=false`) với `{ integrationId, chatId, messageTemplate }`.
- `T-2` Server = **FlowPilot-owned Bot-API proxy** (`Q-2` của CP-05-05 → RESOLVED bởi khảo sát 2026-07-13). Expose duy nhất `send_message`; **không** import community package qua `npx`.
- `T-2b` **Response contract của `send_message` phải trả `message_id`** (từ `result.message_id` của Telegram Bot API `sendMessage` response) trong tool-call result, không chỉ trả `ok:true`. Đây là điều kiện tiên quyết cho verify gate của Task-233 (`Q-4` resolved).
- `T-3` Bot token chỉ ở keyring `telegram:<integrationId>`, env-inject cho proxy; **không** vào Supabase/prompt/log.
- `T-4` Proxy mang sẵn approval bridge seam (mirror `google_drive_proxy_approval.go`) để Task-233 gate hành động gửi.

### Constraints

- SD-11 §3.3 đã amend hợp thức hóa MCP path (2026-07-13); native Go là fallback (`Q-3`).
- Type system-owned (SD-23 `D-2`); seed idempotent (Task-198 `DOD-5`).
- Secret boundary SD-11 §6.
- Tool surface tối thiểu: chỉ `send_message` (không read/delete/forward).

### Open Questions

- `Q-1` Proxy stdio (như Drive) hay HTTP? Đề xuất stdio để đồng nhất provider-config injection.
- `Q-2` `messageTemplate` schema: field cố định (What/status/link) hay free template string? (CP-05-05 `Q-5`).

### Source Refs

- `CP-05-05` `P-1`, `P-2`, `Q-2`.
- Survey 2026-07-13: không có official Telegram MCP; self-host Bot-API proxy khuyến nghị.
- `CP-05-03` §11 (proxy MCP + provider-config), `google_drive_proxy_mcp.go` (mẫu server + approval bridge).
- current code: `artifact_type_registry.go` (`ArtifactType*` consts), `supabase/migrations/20260709090000_add_artifact_types_catalog.sql` (mẫu seed), `settingsHelpers.ts` (`providerFields.telegram`: botToken, channelId).

## 1. Goal

Có type `telegram.v1` trong catalog + một Telegram MCP server tối thiểu (send-only) mà provider CLI gọi được, credential an toàn, làm nền cho output write-contract của Task-233.

## 2. Parent Links

- coding plan: `CP-05-05` (`P-1`, `P-2`)
- tech design: `SD-11` (§3.3 amended), `SD-23` (`D-1/D-2/D-8`)
- system spec: `SS-14` (US-10/AC-17)
- specific upstream ids: `CP-05-05 P-1/P-2`

## 3. Trigger

Telegram output write-contract + gate (Task-233) cần type + MCP server tồn tại trước. Khảo sát đã chốt self-host Bot-API proxy.

## 4. Exact Change

- `T-1` `ArtifactTypeTelegram = "telegram.v1"` const + migration seed row (idempotent) + `config_schema`.
- `T-2` Telegram Bot-API proxy MCP server (Go, mirror `google_drive_proxy_mcp.go`): `tools/list` = `[send_message]`, `tools/call` gọi Bot API sendMessage, **trả về `message_id` từ response thật của Telegram** trong tool result; approval bridge seam.
- `T-3` Keyring cred `telegram:<integrationId>`; env-inject bot token + chat id.
- `T-4` `telegram_mcp_provider_config.go` (+ test): `Ensure*TelegramMcpConfig`/`check*`/`detectStale*` cho ≥1 provider + Claude extra server.
- `T-5` CLI subcommand launcher (mirror `google-drive-mcp`) nếu dùng stdio proxy.

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/artifact_type_registry.go` (const), mới `telegram_proxy_mcp.go` + `telegram_mcp_provider_config.go` (+ tests), `apps/local-runner/internal/cli/root.go` (subcommand), `runner.go` (backend spec + keyring cred), supabase migration `*_add_telegram_artifact_type.sql`, `settingsHelpers.ts`/`McpSettings.tsx` (connect form)
- modules: artifact type catalog, runner MCP proxy + provider-config, desktop MCP settings
- routes: MCP proxy launch/approval
- tables: `artifact_types` (+1 seed row), `integrations` (type `telegram`, đã hợp lệ)

## 6. Acceptance Check

- `telegram.v1` hiện trong catalog tab Artifacts (read-only); tạo instance user với config hợp lệ.
- Telegram proxy MCP expose đúng `send_message`; gọi tool → tin xuất hiện trong test channel; tool-call result chứa `message_id` thật.
- Provider config cho ≥1 CLI có MCP server `telegram`; stale detection có test.
- Bot token chỉ ở keyring (test); không trong Supabase/prompt/log.

## 7. Out of Scope

- Output prompt write-contract + verify gate + approval enforcement (Task-233).
- UI config editor branch `isTelegramType` + compat OUTPUT-only (Task-233).
- Community MCP adoption; MTProto/user-account features.

## 8. Completion Notes

- result: **done**.
  - `supabase/migrations/20260713100000_add_telegram_artifact_type.sql` (new): seeds `telegram.v1` into `artifact_types` (category `notify`, `config_schema` requires `integrationId`+`chatId`, optional `messageTemplate`); `ArtifactTypeTelegram` const added to `artifact_type_registry.go`.
  - `apps/local-runner/internal/runner/telegram_proxy_mcp.go` (new): `telegramProxyMcpServer` — a minimal stdio JSON-RPC MCP server exposing **exactly one tool**, `send_message`, wrapping the real Telegram Bot API `sendMessage` endpoint. Reuses the JSON-RPC plumbing (`mcpRequest`/`mcpResponse`/`mcpError`/`proxyMcpTool`/`textToolResult`) already defined for the Google Drive proxy (same package) rather than redefining it. **Returns the real `message_id`** from Telegram's response embedded in the tool result text (`"...message_id: 42"`) — this is the load-bearing detail Task-233's verify gate needs (CP-05-05 `Q-4` resolved: tool-call-happened alone isn't enough evidence).
  - `apps/local-runner/internal/runner/telegram_mcp_provider_config.go` (new): `telegramCredential` + keyring save/load/delete (mirrors `jiraCredential`/`firebaseCredential`); `validateTelegramCredentialFields`; `resolveConnectedTelegramCredential`; `detectTelegramBackend`; `EnsureClaudeTelegramMcpConfig`/`checkClaudeTelegramMcpConfig` (reuses the existing typed `claudeConfig`/`claudeMcpServer` structs — stdio launcher, but pointed at **this same FlowPilot binary's own `telegram-mcp` subcommand**, not `npx`, since the proxy IS FlowPilot code); `PreflightTelegramMcp` (deliberately **not** registered into Task-227's `mcpInstructionSpecs` — that registry is for `requiredMcps`-style READ context sources; Telegram is an OUTPUT artifact whose prompt injection goes through `composeFlowNodeAgentPrompt`'s artifact-binding dispatch instead, which is Task-233's job. `PreflightTelegramMcp` stands alone for Task-233 to call directly).
  - CLI: `internal/cli/root.go` gained `telegram-mcp` subcommand (mirrors `google-drive-mcp`) — **no token/chat-id flags**; the bot token/channel id are resolved from the runner keyring at launch inside `RunTelegramProxyMcpServer` (a `*Runner` method, not a free function), never passed as process arguments or written into provider config.
  - `runner.go`: added the first-ever `mcpBackendSpec` for `telegram`; wired a real `case "telegram":` in `TriggerIntegrationConnection` (previously grouped with figma/firebase in a shared no-op placeholder).
  - **Bug found and fixed while wiring this in:** `ListMcpBackends`'s dispatch called the generic `spec.detect()`, which gates "installed" on `lookPathFn(spec.Launcher)` succeeding — correct for Google Drive's real `npx` dependency, but wrong for Jira (already special-cased before this task), and it turned out **also wrong for Firebase** (Task-230's tests passed only because this dev sandbox happens to have `npx` on PATH — environment-dependent, not deterministic) and definitely wrong for Telegram (`Launcher: "flowpilot"` is never a real PATH-resolvable command in test/CI). Fixed by special-casing `jira`/`firebase`/`telegram` in `ListMcpBackends` to call their own `detect*Backend` methods (keyring-state-based, not launcher-probe-based), matching how connection state actually works for all three. This is a real correctness fix that also makes Task-230's Firebase tests deterministic instead of PATH-dependent.
  - **Pre-existing test updated (not a regression):** `TestTriggerIntegrationConnectionAcceptsValidRequest` used `ProviderType: "telegram"` with no credentials, asserting the old shared no-op "pending" placeholder behavior — now correctly rejected since Telegram validates real fields. Changed the test to use `ProviderType: "figma"` (the one remaining true no-op placeholder provider), preserving its original intent.
  - Desktop UI: `providerFields.telegram` unchanged (`botToken`/`channelId` already matched the new Go JSON tags with no collision); `McpSettings.tsx`'s `createIntegration()` `needsConnect` and the "show Test button" condition both extended to include `telegram`; `stripSecretFields`'s `secretConfigFields` map extended with `telegram: ["botToken"]` (SD-11 §6 — same fix pattern as Firebase in Task-230).
  - Tests: `telegram_proxy_mcp_test.go` (7 — tools/list shape, sendMessage success/message_id, sendMessage API-error, callTool text-contains-message_id, unknown-tool rejection, empty-text rejection, initialize/tools-list JSON-RPC dispatch — using an `httptest.Server` in place of the real Telegram API via a package-level `telegramBotAPIBase` var, matching the `executeJiraRequestFn` seam pattern); `telegram_mcp_provider_config_test.go` (9 — field validation, connect reject/accept, provider-config write/idempotent/not-connected, preflight ready/not-connected, credential round-trip). `go build ./...` clean; `go vet` clean; `go test ./internal/runner/... -run 'Telegram|telegram|Firebase|firebase|Jira|jira'` — 71 pass; full suite `go test ./internal/runner/...` — 1454 passed / 16 failed / 18 skipped, confirmed by full failure-name diff against Task-231's baseline (identical 16 names — the earlier 17/18 readings were subtest-counting variance in the same flaky pre-existing codex-resume group, not new failures). `tsc --noEmit` in `apps/desktop-flowpilot` clean; `settingsHelpers.test.ts` — 11/11 pass.
  - **Not verified live (matches Task-228/230's documented limitation):** an actual message delivered to a real Telegram chat via a real bot token. What's real and tested: the Bot API request/response shape (against a fake HTTP server proving the exact wire contract), `message_id` extraction, JSON-RPC tool dispatch, keyring storage, provider-config injection, and the CLI subcommand wiring — everything up to an actual live token.
- follow-ups: (1) Task-233 wires the actual OUTPUT prompt injection (`appendTelegramOutputPrompt`) + verify gate (`r-artifact-telegram-sent`) + approval + `isTelegramType` UI editor + OUTPUT-only compat filter — none of that is built yet, by design (this task's scope was type+proxy+connection only); (2) extend provider-config injection to Codex/Gemini/Grok if needed (Claude only, matching CP-05-05's "≥1 provider" bar); (3) `InstallMcpBackend`/`VerifyMcpBackend`'s jira-only special case may need the same firebase/telegram branches `ListMcpBackends` got, if those code paths are ever exercised for firebase/telegram (not currently tested — `TriggerIntegrationConnection` is the connect path actually used).
- upstream docs updated: none required — CP-05-05 `P-1`/`P-2` describe exactly this shape.
