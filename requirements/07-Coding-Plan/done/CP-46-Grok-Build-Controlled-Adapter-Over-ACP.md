# CP-46: Grok Build Controlled Adapter Over ACP Transport

## Metadata

- Document ID: `CP-46`
- Title: `Grok Build Controlled Adapter Over ACP Transport`
- Phase: `coding_plan`
- Status: `done` — product path closed 2026-07-11 (Task-212 live smoke). [Task-210](../../08-Task/done/Task-210-Grok-Account-Model-Detect-Connect-Switch-Quota.md) multi-account connect/switch/resume closed 2026-07-15.
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-09`
- Last Updated: `2026-07-11`
- Parent Documents: [SS-05: Workflow AI Provider](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md), [SS-12: Multiple Agents](../../05-System-Specs/SS-12-Multiple-Agents.md), [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md), [SD-16: Agent Spawn And Tool Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- Child Documents: [Task-206: Grok ACP Transport And Process/Dispatcher](../../08-Task/done/Task-206-Grok-ACP-Transport-And-Process-Dispatcher.md), [Task-207: Grok Controlled Adapter MVP (Chat/Stream/Resume)](../../08-Task/done/Task-207-Grok-Controlled-Adapter-MVP.md), [Task-208: Grok Permission Channel And YOLO Posture](../../08-Task/done/Task-208-Grok-Permission-Channel-And-Yolo-Posture.md), [Task-209: Grok MCP, Ask-User, And Spawn-Agent Parity](../../08-Task/done/Task-209-Grok-MCP-Ask-User-Spawn-Agent-Parity.md), [Task-210: Grok Account Model — Detect, Connect, Switch, Quota](../../08-Task/done/Task-210-Grok-Account-Model-Detect-Connect-Switch-Quota.md), [Task-211: Grok Desktop UI Surface](../../08-Task/done/Task-211-Grok-Desktop-UI-Surface.md), [Task-212: Grok Parity Hardening And Live DOD](../../08-Task/done/Task-212-Grok-Parity-Hardening-And-Live-DOD.md), [Task-213: Auto-Detect And Sync Provider Models](../../08-Task/done/Task-213-Auto-Detect-And-Sync-Provider-Models.md) (spin-off — provider-agnostic model-catalog auto-sync; Grok surfaced the gap), [Task-214: Grok Skill-Catalog And Selection Parity](../../08-Task/done/Task-214-Grok-Skill-Catalog-And-Selection-Parity.md) (spin-off — Grok's native `.grok/skills` support surfaced the gap), [Task-218: Grok YOLO Enforced Via Config Rewrite And Always-Approve Flag](../../08-Task/done/Task-218-Grok-Yolo-Enforced-Via-Config-Rewrite-And-Always-Approve-Flag.md), [Task-221: Grok MCP Tool Gating Via PreToolUse Hook](../../08-Task/done/Task-221-Grok-MCP-Tool-Gating-Via-PreToolUse-Hook.md) (**cancelled / won't-do PreToolUse** — MCP Drive gate met via Task-208/218; closes [BUG-273](../../09-BugFix/done/BUG-273-Grok-Does-Not-Gate-MCP-Tool-Calls-Under-Yolo-Off-In-Chat.md))
- Related Documents: [03 - Solution And System Design](../../10-Refactor/New-System/03-Solution-And-System-Design.md), [04 - Detailed Coding Plan](../../10-Refactor/New-System/04-Detailed-Coding-Plan.md), [07 — Claude Provider Adapter Plan](../../10-Refactor/New-System/07-Claude-Adapter-Plan.md), [04-07 — Phase 7: Providers Capability Packaging](../../10-Refactor/New-System/04-07-Phase7-Providers-Capability-Packaging.md), [CP-40: Gemini Controlled Adapter Over ACP Transport](../todo/CP-40-Gemini-Adapter-Plan.md)
- Replaces: `None`
- Tags: `grok, grok-build, xai, ai-providers, adapter, acp, json-rpc, local-runner, desktop-chat`

## AI Quick View

### Summary

- Add **Grok Build** (xAI's official coding-agent CLI, `grok`, verified at `0.2.93`, model `grok-4.5`/`grok-build`) as a first-class controlled-mode provider alongside Codex, Claude, and Gemini.
- Grok is **not** driven by one-shot `grok -p` (that mode is closer to `agy` and only emits flat text/`json`/`streaming-json`). It is driven by **`grok agent stdio`**, an **Agent Client Protocol (ACP), JSON-RPC 2.0** transport. This is the tier that gives structured tool events, per-turn MCP, permission round-trips, sub-agents, and real server-issued session ids.
- **Empirically verified end-to-end** during CP authoring (`grok 0.2.93`, live `grok.com` login): `initialize` → `session/new{cwd,mcpServers}` → `session/prompt` → streamed `session/update` notifications (`agent_message_chunk`, `agent_thought_chunk`, `tool_call`, `tool_call_update`, `plan`) → server→client `session/request_permission` request (with `options[]`) answered by the client → `tool_call_update{status:completed}` → `turn_completed`. A gated `write` tool was approved via the round-trip and the file was actually written. This is the exact controlled-mode shape Codex/Claude already implement, so Grok can reach real parity (unlike Gemini/CP-40, which is blocked by `UNSUPPORTED_CLIENT` for individual accounts).
- The engineering task is **not** prompt execution. It is making Grok behave like Codex and Claude inside the controlled runner: normal chat + streaming + final message, tool approval / YOLO, MCP (incl. FlowPilot `ask_user`/`spawn_agent` and Google Drive), sub-agents and multi-agent orchestration, skills + context injection, flow-gate finalization, summaries, cross-provider handoff, and multi-account isolation via `GROK_HOME` with detect / add / switch / quota display in the desktop settings and account UI.
- Architecture: Grok's transport is structurally the **Codex app-server dispatcher** (persistent shared process, async multiplexed JSON-RPC, server→client requests, teardown-on-account-switch) expressed in **ACP** — which the repo already speaks via `gemini_acp_transport.go`. The plan is to **generalize the existing ACP transport** and **copy the Codex dispatcher/process model**, not to invent a new runtime. Grok is done when the same desktop and runner workflows users rely on for Codex/Claude also pass for Grok, with any unsupported capability blocked honestly rather than silently bypassed.

### Current Ask

- **Closed.** Grok is a first-class controlled provider for chat/parity smoke. Task-210 multi-account connect/switch/resume closed 2026-07-15.


### Key Decisions

- `P-0` **PLUGIN-ONLY — ZERO BASE REGRESSION (highest-priority rule).** Grok is added as a *new plugged-in provider*. It must be **additive-only**: shared/base files may only gain a new `grok` branch/case/field; **no existing Codex, Claude, or Gemini code path, signature, or behavior may change.** In particular: (a) do **not** refactor `gemini_acp_transport.go` — build a standalone Grok ACP module and leave Gemini's transport byte-identical; (b) every shared file that gets a `grok` branch must ship with a regression test proving the Codex/Claude/Gemini branches still behave identically; (c) reuse (never mutate) the provider-neutral shared helpers (`claudeMCP`/`claudeMCPToolDefs`/`handleClaude*`, `agent_orchestrator`, `finishTurn`, `YoloPosture`); (d) if a shared function is hard-switched on provider (`defaultModelForProvider`, `summarizerModelFor`, `supportsHandoffSource`, `resolvePromptExecutionAdapter`, `LocateSessionFile`, `providerKeyFromModel`, etc.), add the `grok` case at the end without reordering existing cases. This rule overrides convenience: if full parity would require changing a Codex/Claude path, stop and flag it instead.
- `P-1` Drive Grok through **`grok agent stdio` (ACP JSON-RPC 2.0)**, never one-shot `grok -p` for the interactive turn loop (the summarizer one-shot exception is documented in `P-13`). The controlled capabilities (tool events, per-turn MCP, permission round-trip, sub-agents, real session ids) only exist on the ACP transport. (Verified.)
- `P-2` The adapter must implement `ProviderRuntimeAdapter` (`Key`, `Capabilities`, `SendTurn`) and emit only normalized `ProviderEvent`; no Grok-only desktop flow. (`provider_registry.go`, `provider_event.go`.)
- `P-3` **Copy the Codex process/dispatcher model** (`codex_appserver_process.go` + `codex_appserver.go`): one shared persistent `grok agent stdio` process bound to a `scopeKey`, an async dispatcher multiplexing many sessions over one stdio, and teardown+respawn on account switch. Do **not** use Claude's spawn-per-turn model.
- `P-4` **Build a standalone Grok ACP module; do NOT refactor `gemini_acp_transport.go`.** The Gemini ACP helpers (session/new param builders, `agent_message_chunk` text extraction, `flowpilot` MCP server entry builder, response `sessionId` reader) are a *reference* to copy the shapes from, not a file to extract/mutate. Grok gets its own `grok_acp.go` with its x.ai extensions. Unifying the two ACP implementations is an explicit **non-goal** of this CP (deferred; only revisit once Grok is proven and with its own regression-gated task) — per `P-0`, touching Gemini's live transport is disallowed here.
- `P-5` **Permission gating = Codex inbound-request pattern, not Claude HTTP-MCP.** Grok delivers `session/request_permission` (a server→client JSON-RPC *request* carrying `options[]`); route it to `bridge.RequestApproval` and answer with `{outcome:{outcome:"selected", optionId:<chosen>}}` using the exact option ids Grok offers (`allow-once`, `allow-edits-session`, `reject-once`). Do **not** stand up `claudeMCPServer`/`--permission-prompt-tool` for approvals.
- `P-6` **Set `clientCapabilities.fs.writeTextFile=false` and `readTextFile=false`** so Grok performs file I/O with its own tools and routes each write/edit through `session/request_permission` (proven path). Derive `EventFileChanged` from `tool_call_update` diffs (they carry `path`+`newText`). This avoids implementing an ACP client-side filesystem server in v1.
- `P-7` **Reuse the runner-hosted FlowPilot MCP server** (`ask_user`, `spawn_agent`, `submit_review_outcome`) by passing it through ACP `session/new{mcpServers:[...]}` as an HTTP MCP entry — the same mechanism `geminiACPFlowPilotMCPServers` already builds. ACP has a real `mcpServers` field, so `--strict-mcp-config`-style shadowing is a non-issue.
- `P-8` **Disable Grok's ambient MCP auto-discovery** by launching the process with `GROK_CLAUDE_MCPS_ENABLED=false` and `GROK_CURSOR_MCPS_ENABLED=false` (and not relying on project `.mcp.json`). This is both a correctness requirement (deterministic tool set per turn, like Claude's `--strict-mcp-config`) **and a security requirement** — see Constraint on credential leakage.
- `P-9` **YOLO** uses the runner `YoloPosture` SSOT. Add a `GrokPermissionMode` field; map YOLO=true → `bypassPermissions` (the only mode Grok honors via flag/session per its own docs) with `RunnerAutoApprove=true`, YOLO=false → default + live `session/request_permission` gating. `ask_user` must never be auto-approved even when YOLO=true.
- `P-10` **Multi-account = multiple `GROK_HOME` dirs**, mirroring Codex's `CODEX_HOME` model exactly. Slot 0 = `~/.grok`; slots 1..N = managed `~/.grokHomeN`. Add `grok` branches to every hard-coded `codex/claude/gemini` provider switch (≈15 sites catalogued in §5). No Supabase schema change (all cloud tables treat `provider_key` as opaque text).
- `P-11` **Capability parity must be proven, not assumed.** A `ProviderCapabilities` flag stays false until a unit test or live check proves the runner can observe and control it. Grok's live ACP evidence already covers streaming, approval events, tool events, sub-agents, MCP, and real session ids.
- `P-12` Grok is treated as **additive**. Existing Codex/Claude/Gemini and shared runner behavior must not change except where a neutral extension point or Grok registration is required; every touched shared file keeps regression coverage for the other providers.
- `P-13` **Summarizer one-shot exception (only exception to `P-1`).** Summary generation uses the shared one-shot `resolvePromptExecutionAdapter` path like the other providers (no session, no tools, no gating), so Grok's summarizer branch MAY use one-shot `grok -p --output-format json`. This is the *only* place Grok uses `-p`; the interactive turn loop stays on ACP. `summarizerModelFor` gets a cheap Grok model case.
- `P-14` **`ModelContextWindow` + token usage must reach the UI.** Map Grok's `turn_completed._meta` token counts (verified live: `totalTokens`/`inputTokens`/`outputTokens`/`cachedReadTokens`/`reasoningTokens`) and the model's `totalContextTokens` (verified live in `initialize`, `500000` for grok-4.5) into `EventTokenUsageUpdated` + `TokenUsageSnapshot.ModelContextWindow`, or show absence explicitly.
- `P-15` **MCP readiness must gate the prompt.** Grok initializes MCP asynchronously (verified: `_x.ai/mcp/init_progress` notification). Withhold `session/prompt` until FlowPilot's MCP tools are connected (mirror Claude's `waitReady`/`claudeMCPReadyDefaultTimeout`), else the first turn drops `ask_user`/`spawn_agent` (BUG-114 class). Resumed turns must re-attach the same MCP + permission channel (BUG-087 class).

### Constraints

- **ABSOLUTE (per `P-0`): this is a plugged-in new provider. Do NOT change any base/shared behavior that affects Claude, Codex, or Gemini.** Shared-file edits are additive-only (`grok` branch appended, existing cases untouched, no signature changes to existing callers). Do NOT refactor `gemini_acp_transport.go`. Any shared function switched on provider must get its `grok` case appended without reordering. If achieving parity appears to require modifying a Codex/Claude/Gemini path, stop and flag it rather than proceeding.
- The following shared functions are hard-switched on provider and MUST gain an appended `grok` case (they were confirmed by audit and are easy to miss): `interactive_service.go` `defaultModelForProvider` (else a cross-provider child spawned onto Grok with no explicit model gets `""` → request failure), `summarizer.go` `summarizerModelFor` + the `supportsHandoffSource` summary gate, `runner.go` `resolvePromptExecutionAdapter` (summarizer one-shot), `handoff_context.go` `supportsHandoffSource` (Grok-as-source), `session_file_locator.go` `LocateSessionFile` (cross-account/Drive restore — Grok sessions are SQLite, so this needs a real branch or an explicit typed-unsupported path).
- Preserve the runner ownership model: workflow state, approvals, prompt assembly, finalizer, and orchestration stay in the Go runner. The desktop client must need only capability rendering + enum/label/icon/color additions.
- **Security (hard):** During CP authoring, launching `grok agent stdio` inside the flowpilot workspace caused Grok to auto-discover and **echo plaintext credentials** (a Google Drive OAuth refresh token + client secret) from an ambient MCP config via the `_x.ai/mcp/servers_updated` notification. The Grok process MUST always be launched with ambient compat scanning disabled (`GROK_CLAUDE_MCPS_ENABLED=false`, `GROK_CURSOR_MCPS_ENABLED=false`) and MUST NOT inherit host secrets it does not need. Treat any credential surfaced by the transport as sensitive and never log it.
- Do not mark a `ProviderCapabilities` flag true until proven by test or live check.
- Do not add a Grok-only bypass for approvals, agent spawning, summaries, flow gates, or prompt/context injection.
- Keep Grok account/home handling aligned with existing provider-account discovery; multiple accounts = multiple `GROK_HOME` dirs with deterministic account IDs (`deterministicProviderAccountID`).
- Any touched shared/base file must preserve existing behavior and add/keep regression checks for the Codex/Claude/Gemini paths it also serves.
- Grok Build is early (`0.2.93`) and its `x.ai/*` ACP extensions "may expand across releases"; pin the observed wire shapes behind a validation gate and a tested-baseline version so churn is detected (`compat.go`).

### Open Questions

- `Q-1` Which `ProviderCapabilities` can Grok truthfully claim on day one? Live evidence supports Streaming, Resume, ApprovalEvents, FileEvents (derived from `tool_call_update` diffs), Mcp, Interrupt. Vision is unconfirmed (`initialize` reported `promptCapabilities.image=false`) — keep `Vision=false` until proven.
- `Q-2` What is Grok's exact resume request — `session/load{sessionId,cwd,mcpServers}` (ACP standard) or an `x.ai/*` variant? Confirm the response shape and whether resume is portable across `GROK_HOME` values (account-bound vs relocatable).
- `Q-3` **ANSWERED (2026-07-11):** YOLO=false via live `session/request_permission` gates **write/exec native tools** (Task-208) **and Google Drive MCP tools** (desktop live: ApprovalCard for `google-drive__authGetStatus`; deny blocks; YOLO=true auto-runs — BUG-273 closed). Read-class native tools may still fast-path (Grok safe-read model) — accepted. **PreToolUse / `--allow/--deny` not required** for the verified MCP Drive path; Task-221 cancelled as won't-do. Reopen a hard-gate task only if MCP permission emission regresses.
- `Q-4` Should FlowPilot expose `ask_user`/`spawn_agent` via ACP `session/new{mcpServers}` (runner-hosted HTTP MCP, reuse `claudeMCP`) or via Grok's native `spawn_subagent`/`ask_user_question` tools (seen in the `initialize` tool list)? Native tools avoid an MCP hop but need bridge shims and prompt-composition parity (BUG-128).
- `Q-5` Does Grok expose account **usage/quota** anywhere machine-readable (the only quota signal observed was a turn-time `402 personal-team-blocked:spending-limit`), or is the account card limited to email/plan + terminal usage-limit classification? **Corrected (2026-07-10):** yes — `GET https://cli-chat-proxy.grok.com/v1/billing`, authenticated with the account's cached `~/.grok/auth.json` bearer token, returns a real `monthlyLimit`/`weeklyLimit`+`used`+`billingPeriodEnd` body; the ACP-only research behind the original answer was incomplete. Implemented in [Task-216: Grok Real Usage/Quota Fetch](../../08-Task/done/Task-216-Grok-Real-Usage-Quota-Fetch.md).
- `Q-6` Is `grok login --device-auth` sufficient for headless/managed-home connect, and does a fresh `GROK_HOME` isolate auth cleanly (`~/.grok/auth.json`) the way `CODEX_HOME` does?
- `Q-7` **ANSWERED (Task-212, 2026-07-11):** Grok-as-handoff-source is **enabled** via `chat_history.jsonl` + `loadGrokTranscriptEvents` / `seedGrokTranscriptFromDisk` (not SQLite). `supportsHandoffSource(grok)=true`; live Grok→Codex handoff verified. SQLite extractor not required.
- `Q-8` `grok agent serve` (WebSocket) vs `grok agent stdio` — stdio matches the Codex model and is the plan of record; revisit `serve` only if multi-client sharing is needed later.

### Source Refs

- Live empirical verification during CP authoring (scratchpad `test_grok_acp*.py` + `grok_acp_log*.txt`): ACP handshake, `session/update` event schema, `session/request_permission` round-trip, real `write` executed. Grok docs shipped at `~/.grok/docs/user-guide/` (`14-headless-mode.md`, `15-agent-mode.md`, `07-mcp-servers.md`, `22-permissions-and-safety.md`).
- `requirements/10-Refactor/New-System/04-Detailed-Coding-Plan.md` (`ProviderRuntimeAdapter` / P2 contract)
- `requirements/10-Refactor/New-System/07-Claude-Adapter-Plan.md` (the "07 plan" adapter blueprint)
- `requirements/10-Refactor/New-System/04-07-Phase7-Providers-Capability-Packaging.md` (capability model + placeholder→first-class rules)
- `requirements/06-System-Tech-Design/SD-06-AI-Provider-Integration.md`, `SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md`, `SD-16-Agent-Spawn-And-Tool-Calling-Design.md`
- `requirements/07-Coding-Plan/todo/CP-40-Gemini-Adapter-Plan.md` (closest full-provider-rollout template; its parity matrix is the onboarding checklist)
- Runner: `apps/local-runner/internal/runner/provider_registry.go`, `provider_event.go`, `provider_accounts.go`, `codex_appserver_process.go`, `codex_appserver.go`, `codex_adapter.go`, `codex_event_mapper.go`, `gemini_acp_transport.go`, `yolo_resolver.go`, `claude_mcp_server.go`, `claude_permission_mcp.go`, `google_drive_mcp_provider_config.go`, `compat.go`, `interactive_service.go`, `runner.go`
- Desktop: `apps/desktop-flowpilot/src/types/contract.ts`, `components/settings/AiProvidersSettings.tsx`, `components/settings/CheckVersionSettings.tsx`, `components/ProviderAccountsPanel.tsx`, `components/ChatInput.tsx`, `state/store.ts`, `client/HttpWsRunnerClient.ts`; `packages/flowpilot-client-core/src/domain/adminModels.ts`, `adminLogic.ts`, `runner.ts`, `data/runnerAdminRepository.ts`, `data/supabaseAdminRepository.ts`
- Prior-art docs (feature families to mirror): Codex `04-03`/`05`, Claude `07`, Gemini `CP-40`/`Task-164..167`; approvals `SS-08`/`SD-09`/`Task-182`; MCP `CP-13`/`CP-29`/`Task-055`/`Task-056`/`Task-195`; YOLO `Task-028/030/031/050`; spawn/multi-agent `SD-16`/`Task-082..095`/`CP-19`/`CP-36`; resume/sync `SD-14`/`SD-15`/`Task-067..078`; accounts `Task-018/036/038/039/059`/`BUG-092..095`; quota `Task-015/080`; change-audit `CA-055/056/060/067..072/079/083/084/091/105/109/119/132`

## 1. Goal

Implement Grok Build as a real controlled-mode provider adapter for desktop chat and runner-driven workflow turns, using the same provider-neutral abstraction already used by Codex, Claude, and Gemini. FlowPilot must start, stream, gate, resume, and finalize Grok turns through the runner; support `ask_user`, `spawn_agent`, and multi-agent orchestration; isolate multiple Grok accounts by `GROK_HOME`; and surface Grok in the desktop settings (detection + add account) and account panel (current account, limits, models, switch) — without reintroducing provider-specific plumbing in the desktop client.

## 2. Input Documents

- [SS-05: Workflow AI Provider](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md)
- [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- [SS-12: Multiple Agents](../../05-System-Specs/SS-12-Multiple-Agents.md)
- [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md)
- [SD-16: Agent Spawn And Tool Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- [04 - Detailed Coding Plan](../../10-Refactor/New-System/04-Detailed-Coding-Plan.md)
- [07 — Claude Provider Adapter Plan](../../10-Refactor/New-System/07-Claude-Adapter-Plan.md)
- [04-07 — Phase 7: Providers Capability Packaging](../../10-Refactor/New-System/04-07-Phase7-Providers-Capability-Packaging.md)
- [CP-40: Gemini Controlled Adapter Over ACP Transport](../todo/CP-40-Gemini-Adapter-Plan.md)

## 3. Implementation Strategy

- overall approach:
  - Reuse the controlled-mode contract: `ProviderRuntimeAdapter`, `TurnBridge`, `ProviderEvent`, `InteractiveService`, provider registry.
  - Treat **ACP over `grok agent stdio`** as the transport substrate. Generalize the existing `gemini_acp_transport.go` helpers into a shared ACP layer; copy the **Codex app-server dispatcher/process model** for the persistent shared process, session multiplexing, and account-switch teardown.
  - Add Grok through provider-neutral extension points only. Do not rewrite Codex/Claude/Gemini adapters or alter their runtime behavior.
  - Mirror the proven per-provider structure: dedicated transport/process code, dedicated event mapper, provider adapter implementing `SendTurn`, live-runner registry enablement, focused contract/integration tests, then desktop UI wiring.
- sequencing logic:
  - The ACP contract is **already validated** (see AI Quick View / Source Refs). Start from the captured wire shapes rather than re-deriving them; the first coding slice is the transport/process/dispatcher, then the adapter + event mapper, then permission/MCP/YOLO, then accounts + desktop UI, then parity hardening.
  - Land the runner adapter behind the placeholder until it passes the parity bar; enable in the live registry only after `Task-207` (chat/stream/resume) and `Task-208` (permission) pass.
- dependencies:
  - Provider-neutral runtime contract (`provider_event.go`, `provider_registry.go`).
  - Existing ACP helpers (`gemini_acp_transport.go`) and Codex dispatcher (`codex_appserver.go`, `codex_appserver_process.go`).
  - Provider-account discovery/isolation (`provider_accounts.go`, `runner.go`).
  - Runner-hosted FlowPilot MCP server (`claude_mcp_server.go`) if `ask_user`/`spawn_agent` go through ACP `mcpServers`.

### 3.1 Required Parity Matrix

| Capability | Codex/Claude baseline | Grok requirement | Live evidence | Validation |
| --- | --- | --- | --- | --- |
| Normal chat + streaming + final message | Desktop turns via runner emit normalized stream events + persist history. | Grok ACP `session/update` `agent_message_chunk` → `message_delta`/`message_completed`; `turn_completed` reaches shared finalizer. | Verified (streamed chunks + `turn_completed`). | `GR-01`, `GR-10` |
| Task / bugfix mode | `changeType=task|bugfix` flows through the same interactive run. | Grok accepts task/bugfix turns with identical prompt/context/finalizer behavior. | — | `GR-02` |
| Workflow-step routing | `workflow_step_auto` resolves provider from model. | `grok-*`/`grok-build` models route to Grok without special desktop logic. | — | `GR-03` |
| Tool events | `tool_started`/`tool_completed` render in shared UI. | Grok `tool_call`/`tool_call_update` (title, rawInput, rawOutput, diff) → tool events; `FileEvents` derived from write/edit `tool_call_update` diffs. | Verified (`list_dir`, `write` with diff). | `GR-11` |
| YOLO off (approval gate) | Dangerous actions emit `permission_required`; deny stops the action. | `session/request_permission` → `TurnBridge.RequestApproval`; reply `{outcome:{selected, optionId}}`; deny blocks. Covers write/exec **and** Drive MCP (desktop 2026-07-11). | **Done** — write live + Drive MCP live (BUG-273 closed; Task-221 won't-do). | `GR-04` |
| YOLO on | Runner auto-approves eligible actions; `ask_user` still surfaces. | Task-218 `--always-approve` + runner auto-approve; `ask_user` never auto-approved. | **Done** — write + Drive MCP auto under YOLO=on (desktop). | `GR-05`, `GR-06` |
| User questions | Ask-user routes to `user_question_required`. | FlowPilot `ask_user` via ACP `mcpServers` (or native `ask_user_question` bridge shim) blocks for desktop answer. | — | `GR-06` |
| Spawn agents | `spawn_agent` → `TurnBridge.SpawnAgent`, wait true/false, child history, agent panel. | Grok `spawn_agent` (MCP tool or native `spawn_subagent` shim) routes to the same bridge, inherits provider/yolo/context. | Native `spawn_subagent` present in tool list. | `GR-07`, `GR-22` |
| Multi-agent orchestration | Cohorts, join barriers, bounded hub re-invocation, agent graph/bus events. | Grok children participate in the shared orchestrator identically; no Grok-only orchestration. | — | `GR-22` |
| Skill injection | Selected skills prepended with exact content/order. | Grok prompt prep calls the same selected-skill injector (`promptPrep`); no reformatting. | — | `GR-08` |
| Context injection | Feature history + change-audit + chat summary injected before the turn. | Grok uses the same shared prompt assembly path. | — | `GR-09` |
| Flow rules | `r-ca`/`r-bug`/`r-task` run after completion via `finishTurn`. | Grok turns reach `finishTurn`; gate-repair prompts run on Grok. | — | `GR-10` |
| MCP (FlowPilot + external) | Codex/Claude reach FlowPilot MCP (ask_user/spawn_agent) + Google Drive without hiding runner tools. | ACP `session/new{mcpServers:[flowpilot,(google-drive)]}`; ambient compat scanning disabled. | ACP `mcpServers` param accepted; ambient auto-load observed (must disable). | `GR-17`, `GR-21` |
| Summary generation | Manual + idle summaries use a cheap model. | Grok chats summarize via the shared summarizer (cheap Grok model or controlled replacement). | — | `GR-13` |
| Cross-account | Deterministic account IDs from provider+home; explicit active account. | Managed `GROK_HOME` accounts; active-account mismatch handled; cross-account resume proven or safely blocked. | — | `GR-12`, `GR-14` |
| Resume / history | History survives runner restart; account changes handled. | Grok persists real ACP `sessionId`; resume via `session/load` or typed mismatch. | Real server `sessionId` returned by `session/new`. | `GR-12`, `GR-15` |
| Cross-provider handoff | Codex/Claude source extractors + summary-based handoff; Grok can be a target. | Grok-as-target immediately; Grok-as-source (+ summary-based) only after a `~/.grok/sessions` extractor is proven, else typed unsupported. | — | `GR-28` |
| Usage / quota failures | Usage-limit classified separately from login; not retried. | Grok `402 personal-team-blocked:spending-limit` → typed terminal usage-limit error + switch/retry-on-other-account. | Verified signal (`402`, `spending-limit`). | `GR-19` |
| Account metadata / limits | Panels show model, quota/usage, auth; degrade cleanly. | Grok account card shows email/plan/models; usage lines only if Grok exposes them, else explicit absence. | — | `GR-20`, `GR-25` |
| Detection + version baseline | `flowpilot providers list --json` + tested-baseline check. | `grok` binary detected; version vs `CompatTestedGrokVersion` shown. | `grok --version` → `0.2.93`. | `GR-18`, `GR-25` |
| Interrupt | Ctx-cancel kills/cancels the turn. | `<-ctx.Done()` → best-effort ACP cancel → return `ctx.Err()`. | — | `GR-16` |
| Attachments / vision | Advertised only when supported. | `Vision=false` until ACP image input is proven (`initialize` reported `image=false`). | `promptCapabilities.image=false`. | `GR-27` |
| History replay state | Reopened chats replay resolved gates/tools/children. | Grok replay renders resolved rows consistently. | — | `GR-26` |
| Token usage + context-window reporting | `EventTokenUsageUpdated` + `TokenUsageSnapshot.ModelContextWindow` render in desktop; absence degrades. | Map Grok `turn_completed._meta` token counts + `initialize` `totalContextTokens`; render or explicit absence. | Verified (`totalTokens`/`inputTokens`/… in `turn_completed`; `totalContextTokens:500000`). | `GR-24` |
| Drive sync / cross-PC restore / stale-account recovery | `LocateSessionFile` (Codex jsonl / Claude projects) + ndjson sync recover stale provider-account ids. | Grok sessions are `~/.grok/sessions/` **SQLite** — needs a `LocateSessionFile` grok branch or a typed-unsupported path; must not corrupt history. | — | `GR-23` |
| Concurrent / grouped approval + question cards | N simultaneous gates each spawn a goroutine and render as one grouped card without wedging the run. | Multiple concurrent `session/request_permission`/`ask_user` requests resolve independently via the dispatcher. | — | `GR-29` |
| MCP-ready-before-prompt timing | Prompt withheld until `tools/list` arrives, else first turn drops FlowPilot tools. | Withhold `session/prompt` until FlowPilot MCP connected (Grok inits MCP async via `_x.ai/mcp/init_progress`). | Verified (async MCP init notification). | `GR-30` |
| Resumed-turn MCP/approval re-attach | Resumed turns keep approval + `ask_user` + MCP. | A resumed Grok turn re-attaches `mcpServers` + permission channel (BUG-087 class). | — | `GR-31` |
| Session-id integrity guards | Real (not synthetic) id resume; adopt a post-prompt session id if it differs. | Old synthetic id + no real-scope map → explicit fail (no fresh `session/new`); `session/prompt` result id is adopted. | Real server `sessionId` returned by `session/new`. | `GR-32` |
| Grok as hub agent + review-loop-until-clean | Hub drives `autoOrchestrate`, `SubmitFlowControl`, cohort review loop to convergence within cap/extend. | Grok can *be* the hub and a review-loop participant, not only spawn children. | — | `GR-33` |
| Replay typed user prompts | Timeline shows the *typed* user input, not the composed prompt. | Grok replay extracts user prompt vs composed prompt (BUG-046/083 class). | — | `GR-34` |
| Reasoning-effort mapping | `TurnRequest.ReasoningEffort` maps to the provider's effort control. | Explicit map to Grok ACP effort (`none/minimal/low/medium/high/xhigh/max`, verified in `initialize`). | Verified (`reasoningEfforts` in `initialize`). | `GR-35` |
| Child stop + delete-cascade | Stopping/deleting a parent stops children; no orphaned home/session artifacts. | Grok parent stop/delete cascades to children (BUG-086 class). | — | `GR-36` |
| Persistent command allowlist (exec "don't ask again") | `ApprovalDetails.Kind=="exec"` drives the per-project allowlist (BUG-246). | Grok exec approvals set `Kind=="exec"` so the allowlist engages. | — | `GR-04` |
| User-initiated spawn injection | `EventAgentSpawnedByUser` + `UIInitiated` inject a parent-context note into the parent's next provider turn. | A UI-initiated child is injected into the Grok parent's next turn (BUG-121/122). | — | `GR-22` |
| Reserved tool-name safety | `spawn_agent` renamed to `flowpilot_spawn_agent` to dodge a reserved model function name (BUG-124). | Grok's FlowPilot spawn/ask tool names must not collide with native `spawn_subagent`/`ask_user_question`; normalize at the UI boundary. | Native `spawn_subagent`/`ask_user_question` in tool list. | `GR-07` |
| GitNexus structure context | Structure-context provider feeds the shared prompt assembly. | Grok uses the same shared context assembly (no Grok-specific path). | — | `GR-09` |

## 4. Work Breakdown

Each `P-item` maps to a proposed child Task (§Metadata). IDs are proposals to be created from this CP.

- `P-1` Freeze the Grok ACP contract from captured wire shapes (→ inputs for `Task-206`).
  - Convert the verified `initialize`/`session/new`/`session/prompt`/`session/update`/`session/request_permission`/`turn_completed` payloads into typed Go structs and golden fixtures.
  - Record the `x.ai/*` extension surface actually used (`_x.ai/session_notification`, `_x.ai/mcp/servers_updated`, `_x.ai/mcp/init_progress`, permission option kinds) and mark the rest non-exhaustive.
  - Decide the true day-one `ProviderCapabilities` set with proof per flag.

- `P-2` Generalize the ACP transport + build the Grok process/dispatcher layer (`Task-206`).
  - Extract reusable ACP primitives from `gemini_acp_transport.go` (session/new param builder, `agent_message_chunk` text extractor, `flowpilot` MCP server entry builder, response `sessionId` reader) into a shared ACP module; keep Gemini behavior byte-stable.
  - Add `grok_process.go`: a `grokDispatcher` modeled on `codexDispatcher` (`waiters` map for request/response, per-`sessionId` notification subs, a single `inbound` handler for `session/request_permission` + any `fs/*` requests, one read-loop, `fail()` drain) and `ensureGrokProcess(ctx, scopeKey, cwd, env)` modeled on `ensureCodexAppServer` (spawn `grok agent stdio`, run `initialize`, hold one shared `*grokProcessHandle` on a new `*Runner` field with mutex + a `grokBinaryName` var + a `grokAgentEnabled()` gate).
  - Launch env hard-sets `GROK_CLAUDE_MCPS_ENABLED=false`, `GROK_CURSOR_MCPS_ENABLED=false`, `GROK_HOME=<accountHome>`; strip inherited secrets.
  - Account-switch teardown: bind handle to `scopeKey`; on mismatch `handle.close()` + respawn.

- `P-3` Build `grokAdapter` implementing `ProviderRuntimeAdapter` (`Task-207`).
  - `Key()==ProviderKeyGrok`; `Capabilities()` from `P-1`; `SendTurn` honoring every `TurnRequest` field (`Cwd`→`session/new{cwd}`, `ModelName`/`ReasoningEffort`, `SelectedSkills`+context via shared `promptPrep`, `Attachments` as ACP prompt content blocks, `ProviderSessionID` resume, `OfferReviewOutcomeTool` gating).
  - Always emit a terminal event and return through shared finalization.
  - Set `clientCapabilities.fs.{read,write}TextFile=false` (`P-6`).

- `P-4` Grok event mapper `grok_event_mapper.go` (`Task-207`), modeled on `codex_event_mapper.go`.
  - `session/update`: `agent_message_chunk`→delta/completed, `agent_thought_chunk`→(thought, mapped or dropped per UI), `tool_call`→`tool_started`, `tool_call_update`→`tool_completed` (+ `EventFileChanged` from write/edit diffs), `plan`→(optional), token usage from `turn_completed._meta` → `token_usage_updated`.
  - `turn_completed{stop_reason}`→`turn_completed`/`turn_failed`; leave `ProviderTurnID` empty (core stamps it).

- `P-5` Permission channel + inbound routing (`Task-208`).
  - Route inbound `session/request_permission` → build Grok `ApprovalDetails` (tool title, rawInput, kind, cwd) → `bridge.RequestApproval` → encode decision back to the exact ACP `optionId` Grok offered (allow→`allow-once`/`allow-edits-session`, deny→`reject-once`); mirror Codex's decision-vocabulary mapper.
  - Handle `pending_interaction`/`interaction_resolved` notifications for UI state; ensure a denied/timed-out gate yields a controlled `PermissionRejected`/cancel outcome.

- `P-6` YOLO + posture (`Task-208` + `Task-218`).
  - Add `GrokPermissionMode` to `YoloPosture` (`yolo_resolver.go`): YOLO=true→`bypassPermissions`+`RunnerAutoApprove`, YOLO=false→`default` + live gating.
  - **DONE (2026-07-11):** deny-by-default holds for write/exec and Google Drive MCP via `session/request_permission` → ApprovalCard (user live: deny blocks, YOLO=on auto). Task-218 rewrites `permission_mode=always-approve` → `default` under YOLO=false. PreToolUse not needed — Task-221 won't-do (`Q-3` closed for verified paths). Read-class native fast-path remains Grok-side and out of scope.
  - Ensure no persisted Grok allowlist (`~/.grok/config.toml`/`.claude/settings.json` compat) can bypass the runner toggle (Claude `CA-079`/`BUG-069` analog) — Task-218 auto-enforce.

- `P-7` MCP + `ask_user` + `spawn_agent` (`Task-209`).
  - Expose FlowPilot tools via ACP `session/new{mcpServers:[flowpilot]}` reusing `claudeMCP` + `mcpBaseURL` + `claudeMCPToolDefs` (they are provider-neutral) — or native `ask_user_question`/`spawn_subagent` shims (`Q-4`); pick one, document why.
  - `spawn_agent` → `TurnBridge.SpawnAgent`; support wait true/false, child persistence, agent panel, inherited provider/yolo/context; provider-neutral spawn-prompt composition (`BUG-128`).
  - Merge Google Drive server into the ACP `mcpServers` array via `flowpilotClaudeExtraMCPServers` (no `--strict-mcp-config` shadowing needed — ACP has a real field).
  - `OfferReviewOutcomeTool` gating parity (advertise `submit_review_outcome` only on hub turns + re-check on call).

- `P-8` Account model, isolation, detection, connect, switch, quota (`Task-210`).
  - Provider spec: `providerSpecs()` add `{Key:"grok", Label:"Grok", BinaryName:"grok", InstallHint, Models}`; `providerAuthStatus`, `providerInstallCommand` grok cases.
  - Homes/env: `managedProviderHomePrefix`→`.grokHome`; `NextAccountHomePath` prefix; `DiscoverProviderAccountHomes`+`discoverGrokAccountHomes`+`isValidGrokAccountPath` (`config.toml`/`auth.json`); `defaultAuthCandidates`+`accountAuthPaths`+`hasValidProviderAuthFile` (`~/.grok/auth.json`); `syncManagedProviderAccounts`/`syncProviderAccounts` loop add `"grok"`; `getEnvForExecution` + strip-list add `GROK_HOME`; `providerEnvSetCommand` grok export; registry adapter factory block + `ProviderKeyGrok` const + `providerKeyFromModel` `grok-` prefix; `/provider-accounts/defaults` + `/context` loops add `"grok"`.
  - Connect/login: `StartInteractiveAuth`/`AuthenticateProvider` grok → `grok login` (+ `grok login --device-auth` variant for headless/managed homes).
  - Switch: no new mechanics — reuses `ActivateProviderAccount`/`SetActiveAccount` (lazy per-turn resolve + ctx-cancel of in-flight turns). Verify Grok adapter factory resolves account lazily by `scopeKey=account.ID`.
  - Quota/usage: `loadAccountLaunchMetadata` grok branch (`loadGrokAccountMetadata`: email/plan from `~/.grok/auth.json`, usage lines if exposed); classify `402`/`spending-limit` in `isProviderUsageLimitError` + adapter error mapping as terminal (non-retryable) with switch/reset guidance.
  - Version baseline: `compat.go` add `CompatTestedGrokVersion` (seed `0.2.93`) + `CompatConfig`/`CompatVersionInfo`/`RunCompatCheck` + `compatRunVersion("grok")` + optional `grok agent stdio` handshake probe.

- `P-9` Desktop UI surface (`Task-211`).
  - Core enums: `contract.ts` `ProviderKey` `| "grok"`; `adminModels.ts` `SupportedModel.providerKey` `| "grok"`; `runner.ts` add `tested/installedGrokVersion` if baseline shown.
  - Detection/install: `AiProvidersSettings.tsx` rows already iterate `localProviders` (Grok appears once `GET /providers` returns it); generalize the gemini-only install button/message; add `grok` to the Add-Supported-Model provider select; optionally add a third `CheckVersionSettings` row.
  - Add account: extend the `as "claude"|"codex"|"gemini"` cast in `ProviderAccountsPanel` to include grok (connect flow already passes the key through).
  - Current account + switch: `ProviderAccountsPanel` `PROVIDERS` add `{key:"grok",label:"Grok"}`; `ProviderAccountSummary` is provider-agnostic (no change).
  - Picker/model: `ChatInput.tsx` `PROVIDER_CARDS` add Grok + `GrokIcon`; `VISION_PROVIDERS` only if proven; `store.ts` `pickDefaultModel` grok branch; `adminLogic.ts` `resolveProviderKeyForModel` `grok-` prefix.
  - Labels/colors: `store.ts` `providerLabel` grok; `styles.css` `--grok-brand` + `.provider-chip-grok*`; `AgentsPanel.tsx` provider-override chip (optional).
  - Mocks/tests: `MockRunnerClient.ts` grok entry; check `store.test.ts` enum assumptions.

- `P-10` Skills + context + flow gates + summaries + handoff parity (`Task-212`).
  - Skill/context injection through the shared `promptPrep`/injector (`CA-091`); no Grok-specific reformatting.
  - Flow gates: Grok turns reach `finishTurn`; `r-ca`/`r-bug`/`r-task` repair prompts run on Grok.
  - Summaries: manual + idle summary for Grok chats via shared summarizer.
  - Handoff: Grok-as-target immediately; Grok-as-source only after a `~/.grok/sessions` SQLite extractor is proven (else keep disabled, documented).

- `P-11` Registry enablement, resume wiring, live DOD (`Task-212`).
  - Add live `grok` registration in `ProviderRegistryFor`; keep placeholder fallback in the default registry.
  - Persist real ACP `sessionId` via `ProviderSessionStore.UpsertSession{ProviderKey:"grok"}`; add a `grokAdapter` branch to the resume re-seed in `interactive_service.go` (`session/load`), or typed mismatch.
  - Full live acceptance run (chat, approval-gated action, spawned child, summary, flow gate, resume, account switch).

- `P-12` Tests, operator notes, fallback policy (spans `Task-206..212`).
  - Contract/integration tests per capability (`GR-*`); regression coverage for every shared file touched (Codex/Claude/Gemini stay green).
  - Operator docs: install (`irm https://x.ai/cli/install.ps1 | iex` / platform equivalents), `grok login`, managed-home model, known gaps.
  - Keep placeholder until the parity bar passes.

## 5. Touched Areas

- files (runner, extend):
  - `apps/local-runner/internal/runner/provider_registry.go` (register `grok`, `providerKeyFromModel`, adapter factory)
  - `apps/local-runner/internal/runner/provider_event.go` (`ProviderKeyGrok`)
  - `apps/local-runner/internal/runner/yolo_resolver.go` (`GrokPermissionMode`)
  - `apps/local-runner/internal/runner/provider_accounts.go` (`managedProviderHomePrefix`, `DiscoverProviderAccountHomes`, `syncManagedProviderAccounts`, discover/validate helpers)
  - `apps/local-runner/internal/runner/runner.go` (`providerSpecs`, `NextAccountHomePath`, `defaultAuthCandidates`, `accountAuthPaths`, `hasValidProviderAuthFile`, `getEnvForExecution`, `providerEnvSetCommand`, `StartInteractiveAuth`, `AuthenticateProvider`, `providerAuthStatus`, `providerInstallCommand`, new `*Runner` grok-process fields)
  - `apps/local-runner/internal/runner/interactive_service.go` (append `grok` case to `isProviderUsageLimitError`; add `grokAdapter` resume re-seed branch; **append `grok` case to `defaultModelForProvider`** — else cross-provider child spawn onto Grok with no explicit model returns `""` → request failure; `SetActiveAccount` unchanged)
  - `apps/local-runner/internal/runner/summarizer.go` (**append `grok` case to `summarizerModelFor`** + the summary `supportsHandoffSource` gate so Grok chats can summarize)
  - `apps/local-runner/internal/runner/handoff_context.go` (**append `grok` case to `supportsHandoffSource`** only when the Grok-as-source extractor is proven; else leave false)
  - `apps/local-runner/internal/runner/session_file_locator.go` (**append `grok` case to `LocateSessionFile`** — Grok sessions are `~/.grok/sessions/` SQLite, so this is a real new branch or an explicit typed-unsupported return for cross-account/Drive restore)
  - `apps/local-runner/internal/runner/google_drive_mcp_provider_config.go` (`getProviderConfigPath`, `checkProviderGoogleDriveMcpConfig`, provider loop → include `grok`/`config.toml`)
  - `apps/local-runner/internal/runner/compat.go` (`CompatTestedGrokVersion` + config/info/probe — APPEND fields, never reorder/rename, so `CheckVersionSettings.tsx` stays valid for Codex/Claude)
  - `apps/local-runner/internal/runner/provider_account_terminal.go` (`loadAccountLaunchMetadata` → `loadGrokAccountMetadata`)
  - `apps/local-runner/internal/runner/runner.go` — additionally **append `grok` case to `resolvePromptExecutionAdapter`** (summarizer one-shot `grok -p`, the `P-13` exception)
  - `apps/local-runner/internal/cli/root.go` (`/providers`, `/provider-accounts/*` provider loops already generic; add `grok` to hard-coded `[]string{codex,claude,gemini}` loops)
  - `apps/local-runner/internal/runner/gemini_acp_transport.go` (extract shared ACP helpers; preserve Gemini behavior)
- files (runner, new):
  - `grok_process.go` (dispatcher + `ensureGrokProcess` + handle)
  - `grok_adapter.go` (`ProviderRuntimeAdapter`)
  - `grok_event_mapper.go`
  - `grok_acp.go` / shared `acp_transport.go` (generalized ACP primitives)
  - `grok_usage.go` (402/spending-limit + account metadata)
  - `grok_adapter_test.go`, `grok_event_mapper_test.go`, `grok_process_test.go`, golden fixtures
- files (desktop, extend): `types/contract.ts`, `components/settings/AiProvidersSettings.tsx`, `components/settings/CheckVersionSettings.tsx`, `components/ProviderAccountsPanel.tsx`, `components/ChatInput.tsx` (+ `GrokIcon`), `components/AgentsPanel.tsx`, `state/store.ts`, `client/HttpWsRunnerClient.ts`, `client/MockRunnerClient.ts`, `styles.css`; `packages/flowpilot-client-core/src/domain/adminModels.ts`, `adminLogic.ts`, `runner.ts`, `data/runnerAdminRepository.ts`
- modules: local runner provider runtime; ACP transport; desktop chat + settings + account panel; provider account resolution; workflow session persistence/resume; agent orchestration bridge; flow-gate finalization; summary generation
- database: no schema change (`workflow_provider_sessions`/`events`/`approvals`/`questions` and `ai_supported_models`/`default_provider` treat `provider_key` as opaque text). Optionally add `ai_supported_models` rows with `provider_key='grok'` via `supabaseAdminRepository.ts`.
- external systems: `grok` CLI / `grok agent stdio` ACP process; local `~/.grok` (config.toml, auth.json, sessions SQLite, mcp_credentials.json); `cli-chat-proxy.grok.com` / xAI API; optional Google Drive + other MCP servers over ACP `mcpServers`.

### 5.1 Base-Regression Guard (per `P-0`)

Every shared file above is **additive-only**. The change type and required guard per file:

| Shared file | Change type | Regression guard |
| --- | --- | --- |
| `gemini_acp_transport.go` | **NOT TOUCHED** | Highest-risk if refactored — do not. Grok gets a standalone `grok_acp.go`; Gemini's transport stays byte-identical. |
| `provider_registry.go` | Additive (`grok` reg + `providerKeyFromModel` `grok-` case appended last) | Existing `gpt-/gemini-/claude-` prefix cases unchanged; registry test proves default registry still placeholder-safe. |
| `provider_event.go` | Additive (`ProviderKeyGrok` const) | Trivial. |
| `yolo_resolver.go` | Additive (`GrokPermissionMode` field, read only by Grok) | Codex/Claude `YoloPosture` fields byte-identical; `resolveYoloPosture` codex/claude outputs unchanged. |
| `provider_accounts.go`, `runner.go` | Additive (`case "grok"` appended in ~15 switches, incl. `defaultModelForProvider`, `resolvePromptExecutionAdapter`) | Per-function test asserts codex/claude/gemini branches return prior values; `getEnvForExecution` strip-list add of `GROK_HOME` must not disturb `CODEX_HOME`/`GEMINI_HOME`/`HOME` stripping. |
| `interactive_service.go` | Additive (`defaultModelForProvider`, `isProviderUsageLimitError`, resume branch) | `defaultModelForProvider("codex"/"claude")` unchanged; usage-limit classification for other providers unchanged. |
| `summarizer.go`, `handoff_context.go`, `session_file_locator.go` | Additive (`grok` case appended) | Other-provider summary/handoff/locate behavior byte-identical. |
| `compat.go` | Additive (append struct fields) | JSON shape for Codex/Claude unchanged (append, never reorder) so `CheckVersionSettings.tsx` stays valid. |
| `google_drive_mcp_provider_config.go`, `cli/root.go` | Additive (provider loop/switch add `grok`) | Codex/Claude/Gemini config-path + defaults responses unchanged. |
| `claude_mcp_server.go`, `claude_permission_mcp.go`, `agent_orchestrator.go` | **REUSE only, no mutation** | Provider-neutral; if any "generalization" is tempting, stop — reuse as-is. |
| Desktop shared (`contract.ts`, `store.ts`, `ChatInput.tsx`, `AiProvidersSettings.tsx`, `styles.css`, …) | Additive (`\| "grok"`, new card/branch) | `AiProvidersSettings` install-button generalization must keep Gemini's affordance working; `store.test.ts` enum assumptions updated; Codex/Claude/Gemini UI unchanged. |

`GR-BR` + `E2E-36` are the executable proof of this table.

## 6. Data or Migration Steps

- schema: none. Reuse existing provider-session/event persistence; `provider_key` is free text.
- data backfill: none.
- config updates:
  - Document Grok resolution order: `XAI_API_KEY` env vs `grok login` cached `~/.grok/auth.json`; `GROK_HOME` override; per-account managed homes.
  - MCP config for Grok is written to the account home `config.toml` `[mcp_servers.*]` (same TOML shape as Codex) when a file-based server is needed; runtime FlowPilot tools go through ACP `session/new{mcpServers}` instead.
  - Always launch with `GROK_CLAUDE_MCPS_ENABLED=false` + `GROK_CURSOR_MCPS_ENABLED=false` (correctness + security).
  - Seed `CompatTestedGrokVersion` (initially `0.2.93`).

## 7. Validation Plan

- automated tests:
  - `GR-01` Normal chat: Grok streams `message_delta`, completes with `message_completed`+`turn_completed`, saves history, persists real ACP `sessionId`.
  - `GR-02` Task/bugfix: `changeType=task|bugfix` preserved, reaches finalizer.
  - `GR-03` Model routing: `grok-*`/`grok-build` → `ProviderKeyGrok` via `workflow_step_auto`.
  - `GR-04` YOLO=false approval: a `write`/shell action emits `permission_required`; deny (`reject-once`) blocks; allow (`allow-once`) executes (golden-fixture round-trip on the captured `session/request_permission`).
  - `GR-05` YOLO=true policy: eligible actions auto-approve via runner policy; stale Grok/`.claude` allowlists cannot bypass the runner toggle.
  - `GR-06` `ask_user`: Grok asks a blocking question → `user_question_required` → answer returns → turn continues.
  - `GR-07` `spawn_agent`: wait=true and wait=false; child runs persist, appear in agent panel, inherit context.
  - `GR-08` Skill injection: one-skill/multi-skill exact content+order via shared injector.
  - `GR-09` Context injection: feature history + change-audit + chat summary present in the prompt path.
  - `GR-10` Flow gates: `r-ca`/`r-bug`/`r-task` run after Grok completion; repair prompts stay on Grok.
  - `GR-11` Tool/file events: `tool_call`/`tool_call_update` → tool events; write/edit diffs → `file_changed`.
  - `GR-12` Cross-Grok account + resume: two `GROK_HOME` accounts selectable; resume safe under the right account or typed mismatch.
  - `GR-13` Summary: manual + idle summaries work for Grok chats.
  - `GR-14` Account discovery: `~/.grok`, `GROK_HOME`, managed `~/.grokHomeN` → deterministic account IDs.
  - `GR-15` Restart resume: runner restart + desktop reload preserve Grok history/resume metadata.
  - `GR-16` Failure/interrupt: process death, malformed JSON-RPC, and user interrupt → normalized failures + best-effort ACP cancel; run not wedged.
  - `GR-17` MCP readiness: FlowPilot MCP tools present in the same turn; ambient compat scanning disabled (no host secret leak).
  - `GR-18` Detection + version: `grok` detected via `flowpilot providers list --json`; version vs `CompatTestedGrokVersion` classified.
  - `GR-19` Usage/quota: `402 spending-limit` → typed terminal usage-limit error (not login, not recoverable-retry) + switch/reset guidance.
  - `GR-20` Account metadata: card shows email/plan/models; missing usage degrades explicitly.
  - `GR-21` External MCP: Google Drive visible to Grok in the same turn as `ask_user`/`spawn_agent`, or explicit unsupported state.
  - `GR-22` Child-agent lifecycle matrix: graph updates, gates, restart visibility, wait semantics, inherited YOLO, final result, provider-neutral spawn prompt.
  - `GR-25` Model/reasoning metadata: Grok supported models, labels, reasoning-effort controls resolve or are disabled explicitly.
  - `GR-26` History replay: reopened Grok chats replay resolved gates/tools/children without duplicate rows.
  - `GR-27` Attachments: `Vision=false` blocks image attachments before prompt loss; enable only after ACP image input proven.
  - `GR-23` Drive sync / cross-PC restore: sync a Grok chat, regenerate provider-account ids, restore/reopen; either same-provider recovery works (via a `LocateSessionFile` grok branch) or a typed unsupported state is returned without corrupting history; cross-PC `sessions.ndjson` move resumes or returns typed mismatch.
  - `GR-24` Token/context reporting: after a Grok turn, `token_usage_updated` populates and `ModelContextWindow` renders (or shows explicit absence); Codex/Claude display unchanged.
  - `GR-28` Handoff: Grok-as-target receives prior context; Grok-as-source (and summary-based handoff mode selection) is enabled only after a `~/.grok/sessions` extractor passes, else typed-unsupported; Codex/Claude source handoff stays green.
  - `GR-29` Concurrent/grouped approval: a Grok turn issuing two simultaneous gated tool calls surfaces both as a grouped card, each resolves independently, and the run does not hang.
  - `GR-30` MCP-ready-before-prompt: a Grok turn whose FlowPilot MCP init is slow (`_x.ai/mcp/init_progress`) still exposes `ask_user`/`spawn_agent` on the FIRST turn (prompt withheld until ready, bounded by timeout).
  - `GR-31` Resumed-turn MCP/approval re-attach: after resume, a gated Grok tool still surfaces the approval and `ask_user` still works (BUG-087 regression).
  - `GR-32` Session-id integrity: (a) resume with an old synthetic id + no real-scope mapping fails explicitly and does NOT create a fresh `session/new`; (b) if `session/prompt` returns a different `sessionId`, it becomes the stored id.
  - `GR-33` Grok as hub + review-loop: a Grok `autoOrchestrate` run drives a cohort review loop to convergence within cap/extend, using `SubmitFlowControl` and the shared review-loop template.
  - `GR-34` Replay typed user prompts: reopening a Grok chat shows the typed user input, not the composed prompt (BUG-046/083 regression).
  - `GR-35` Reasoning-effort mapping: `TurnRequest.ReasoningEffort` values map to Grok ACP effort ids; an unmapped value degrades to the model default explicitly.
  - `GR-36` Child stop + delete-cascade: stopping/deleting a Grok parent stops in-flight children and leaves no orphaned home/session artifacts (BUG-086 regression).
  - `GR-BR` Base-regression guard: for every shared file that gained a `grok` branch, the Codex/Claude/Gemini branch tests remain green and byte-identical; a dedicated test asserts `defaultModelForProvider`, `summarizerModelFor`, `supportsHandoffSource`, `resolvePromptExecutionAdapter`, and `LocateSessionFile` still return the exact prior values for codex/claude/gemini inputs.
- manual checks:
  - Install `grok`, `grok login`, then in desktop settings confirm Grok shows installed + version + a connected account.
  - Add a second Grok account (managed home) and switch between them; confirm history/resume correctness.
  - Normal chat, task, bugfix; YOLO off → deny a write → confirm no change; YOLO on → confirm runner-owned approval; `ask_user` card; spawn 2 children (blocking + non-blocking); `Gen summary`.
  - Exhaust/simulate quota → confirm terminal usage-limit + account-switch prompt.
- failure cases: binary missing; `grok login` incomplete / `XAI_API_KEY` absent; ACP `initialize`/`session/new` failure; malformed JSON-RPC; interrupted turn; permission `optionId` mismatch; ambient MCP secret exposure (must be prevented); `402` spending-limit; stale session id / resume mismatch; active-account mismatch; external MCP configured but not visible; child wait/approval/restart state inconsistent.

### 7.1 E2E Test Items

| ID | Area | Provider(s) | Test Item | Expected Result |
| --- | --- | --- | --- | --- |
| `E2E-01` | Basic chat | Grok | New Grok chat, prompt, stream, restart/reopen. | Text streams, final message persists, history readable after restart. |
| `E2E-02` | Skill injection | Grok, Codex, Claude | One/multi-skill prompt across providers. | Skill content/order preserved via shared injector. |
| `E2E-03` | Context summary | Grok, Codex, Claude | Long chat → summary → continue. | Summary injected into next Grok turn. |
| `E2E-04` | Same-account resume | Grok | Turn, continue in same session, inspect resume metadata. | Grok reuses captured ACP `sessionId`. |
| `E2E-05` | Restart resume | Grok, Codex, Claude | Restart runner/app, resume all providers. | Others unchanged; Grok resumes safely or explicit mismatch. |
| `E2E-06` | Cross-account safety | Grok | Chat under home A, switch to home B, resume. | No silent resume under wrong account; typed mismatch when unproven. |
| `E2E-07` | Approval gate | Grok, Codex, Claude | YOLO off, request a write/shell action, deny. | Approval card appears, denial blocks; others unchanged. |
| `E2E-08` | YOLO policy | Grok, Codex, Claude | YOLO on + a user question. | Eligible actions auto-approve; `ask_user` still surfaces. |
| `E2E-09` | Tool/file rendering | Grok | Trigger `write`/`list_dir`/shell. | Tool rows + file-change rows render from ACP updates. |
| `E2E-10` | Flow gates | Grok | Task/bugfix flow triggering `r-ca`/`r-bug`/`r-task`. | Gate + repair prompts run on Grok via finalizer. |
| `E2E-11` | Child agent spawn | Grok, Codex, Claude | Spawn blocking + non-blocking children. | Children persist, appear in panel, parent continues correctly. |
| `E2E-12` | Child lifecycle matrix | Grok | Graph visibility, gates, restart reopen, wait true/false, inherited YOLO. | Matches supported Codex/Claude semantics. |
| `E2E-13` | External MCP | Grok, Codex, Claude | Configure Google Drive MCP, use with YOLO on/off. | Grok sees FlowPilot + Drive tools, or explicit unsupported; others green; no secret leak. |
| `E2E-14` | Provider failure recovery | Grok | Force binary/auth/process failure. | Normalized error; run not wedged; history intact. |
| `E2E-15` | Usage/quota | Grok | Force/simulate `402` spending-limit. | Terminal usage-limit + switch prompt; not login text; not recoverable-retry. |
| `E2E-16` | Detection + add account | Grok | Fresh install → detect → connect account → appears active. | Settings shows installed+version; account connects; picker enables Grok. |
| `E2E-17` | Switch account | Grok | Two Grok accounts, switch active mid-session. | In-flight turn interrupts recoverably; next turn runs on new home. |
| `E2E-18` | Account metadata | Grok | Refresh account with/without usage + expired auth. | Email/plan/models show; missing data degrades; reset not blocked. |
| `E2E-19` | Handoff target | Grok, Codex, Claude | Handoff from Codex/Claude into Grok. | Grok receives prior context as target; source paths green. |
| `E2E-20` | Capability flags | Grok, Codex, Claude | Inspect capabilities after runs. | Grok advertises only proven flags; others unchanged. |
| `E2E-21` | Token/context UI | Grok, Codex, Claude | Inspect token usage + context-window after turns. | Grok reports real values or explicit unavailable; others unchanged. |
| `E2E-22` | Drive sync/restore | Grok, Codex, Claude | Sync a Grok chat, regenerate account ids, restore, reopen. | Same-provider recovery works or typed unsupported; no history corruption; others green. |
| `E2E-23` | Cross-PC ndjson sync | Grok | Move a Grok chat's `sessions.ndjson` to a second machine/home, resume. | Resumes or returns typed mismatch; no silent wrong-account resume. |
| `E2E-24` | Synthetic-id resume guard | Grok | Resume with an old synthetic id, no real-scope mapping. | Explicit failure; no fresh `session/new`. |
| `E2E-25` | Prompt-result session id | Grok | `session/prompt` returns a different `sessionId`. | The new id becomes the stored provider session id. |
| `E2E-26` | Handoff source + summary-based | Grok, Codex, Claude | Grok→Codex/Claude handoff; select hybrid/target_summary/raw modes. | Source disabled/typed-unsupported until extractor proven; Codex/Claude source stays green. |
| `E2E-27` | Concurrent/grouped approval | Grok | A Grok turn issues two simultaneous gated tool calls. | Both surface as a grouped card, resolve independently, run does not hang. |
| `E2E-28` | MCP-ready-before-prompt | Grok | Slow FlowPilot MCP init on the first Grok turn. | `ask_user`/`spawn_agent` still available on turn 1 (prompt withheld until ready). |
| `E2E-29` | Resumed-turn MCP re-attach | Grok | Resume a Grok chat, then trigger a gated tool + `ask_user`. | Approval surfaces and `ask_user` works after resume (BUG-087). |
| `E2E-30` | Replay typed user prompts | Grok | Reopen a multi-turn Grok chat. | Timeline shows typed user input, not the composed prompt. |
| `E2E-31` | Model/reasoning metadata | Grok | Select Grok models/aliases + reasoning effort; run chat + `workflow_step_auto`. | Labels/aliases/effort resolve or are disabled explicitly; routing hits `ProviderKeyGrok`. |
| `E2E-32` | History replay state | Grok | Reopen a Grok chat with tools/approvals/questions/child runs. | Resolved states render; no duplicate rows; no reopened gates. |
| `E2E-33` | Attachments fallback | Grok | Attach an image while `Vision=false`. | Blocked/errored before prompt loss; Codex/Claude vision paths unchanged. |
| `E2E-34` | Grok as hub / review-loop | Grok | A Grok `autoOrchestrate` run drives a cohort review loop. | Converges within cap/extend via `SubmitFlowControl`; shared review-loop template used. |
| `E2E-35` | Child stop + delete-cascade | Grok | Stop, then delete, a Grok parent with live children. | Children stop; no orphaned `~/.grokHomeN`/session artifacts. |
| `E2E-36` | Base-regression sweep | Grok, Codex, Claude, Gemini | Run the full existing Codex/Claude/Gemini suites after all Grok edits land. | All prior provider tests pass unchanged; shared-function values byte-identical for existing providers. |

## 8. Rollout and Fallback

- rollout order:
  - Keep Grok as placeholder in the default registry.
  - Land shared ACP extraction + `grok_process.go` (`Task-206`), then adapter + event mapper (`Task-207`), then permission/YOLO (`Task-208`), enable in the live registry, then MCP/agents (`Task-209`), accounts (`Task-210`), desktop UI (`Task-211`), parity hardening + live DOD (`Task-212`).
- fallback path:
  - If a capability cannot be proven on the ACP transport, keep its `ProviderCapabilities` flag false and surface reduced capability honestly (never fake Codex/Claude parity).
  - If FlowPilot MCP over ACP `mcpServers` proves unreliable, fall back to native `ask_user_question`/`spawn_subagent` bridge shims (`Q-4`) before disabling the feature.
  - If cross-account resume cannot be proven safe, return typed account mismatch rather than resuming under the wrong `GROK_HOME`.
  - If the shared ACP extraction risks regressing Gemini in one slice, first add Grok ACP helpers standalone, then refactor Gemini onto them in a follow-up.
- monitoring: runner logs around Grok process lifecycle, session ids, permission round-trips, and transport failures (never log credentials); desktop history/resume behavior; parity checks vs Codex/Claude event categories.

## 9. Risks

- `R-1` **Credential leakage via ambient MCP auto-discovery** (observed live). Mitigation: always disable compat scanning env + minimal inherited env + never log transport-surfaced secrets. This is a launch blocker until enforced and tested.
- `R-2` Grok Build is early (`0.2.93`); `x.ai/*` ACP extensions and permission option ids may change. Mitigation: golden fixtures + tested-baseline version gate + switch on `type`/`sessionUpdate` defensively.
- `R-3` Extracting shared ACP helpers from `gemini_acp_transport.go` could regress Gemini. Mitigation: preserve Gemini request/response byte-for-byte; regression tests before/after.
- `R-4` **Mitigated for product path (2026-07-11):** YOLO=false gates write/exec + Drive MCP via Task-208/218 (BUG-273 closed; Task-221 PreToolUse won't-do). Residual: read-class native fast-path is intentional Grok behavior; reopen hard-gate only if MCP stops emitting `session/request_permission`.
- `R-5` Quota/usage may only be observable as a turn-time `402`; the account card may not show live limits. Mitigation: classify the terminal error well; show limits only if a machine-readable source exists.
- `R-6` `ask_user`/`spawn_agent` schema/name conflicts between ACP MCP and Grok's native tools; prompt-composition drift across providers (`BUG-128`). Mitigation: single chosen path + parity tests.
- `R-7` Cross-account resume may be `GROK_HOME`-bound; do not treat homes as interchangeable.
- `R-8` Grok-as-source handoff needs a readable `~/.grok/sessions` extractor; keep disabled until proven.
- `R-9` Billing: `grok-build` requires an active xAI subscription/credits; a spent account blocks live turns (independent of integration correctness).

## 10. Definition of Done

- A `grokAdapter` implements `ProviderRuntimeAdapter` and the live registry returns it instead of a placeholder.
- Grok desktop chat runs through the same `/client/workflow-runs/.../turns` flow as Codex/Claude/Gemini; only normalized `ProviderEvent`s are emitted; no Grok-only desktop transport.
- `grok agent stdio` ACP transport is driven via a shared ACP layer + Codex-style dispatcher/process, with ambient MCP scanning disabled and no credential leakage.
- Streaming, final-message return, tool/file events, and `turn_completed` finalization work.
- [x] YOLO=false approval (live `session/request_permission` round-trip) for **write/exec and Google Drive MCP**, YOLO=true auto-approval policy, and `ask_user` question UI work through the runner-owned gate; deny blocks the action (Task-208/218; BUG-273 closed 2026-07-11; Task-221 PreToolUse won't-do).
- Grok invokes `spawn_agent` with blocking + non-blocking children and participates in multi-agent orchestration through the shared bridge.
- Selected skills and context injection use the exact shared prompt assembly path (multi-skill tests pass).
- Grok runs in normal chat, task, bugfix, and workflow-step modes; `grok-*`/`grok-build` route to `ProviderKeyGrok`.
- `r-ca`/`r-bug`/`r-task` and flow-gate repair prompts run correctly after Grok turns; manual + idle summaries work.
- [x] MCP: FlowPilot tools (+ Google Drive) are visible in the same turn; Drive MCP under YOLO=off surfaces the standard ApprovalCard (deny blocks / approve runs); YOLO=on auto-runs without card (desktop live 2026-07-11; BUG-273 / Task-221 close-out).
- Detection + version baseline: `grok` is detected with version vs `CompatTestedGrokVersion`; desktop settings shows it installed.
- Add account (`grok login` / `--device-auth`) creates an isolated `GROK_HOME`; multiple accounts are deterministic and switchable like Codex/Claude; switching interrupts in-flight turns recoverably and the next turn runs on the new home.
- Current-account panel shows email/plan/models; `402 spending-limit` is classified as a terminal usage-limit with switch/reset guidance.
- Real ACP `sessionId` persistence + resume (or typed mismatch) survive runner restart and account change; Grok-as-target handoff works, Grok-as-source only if an extractor is proven.
- `ProviderCapabilities` is accurate for streaming, resume, approval, file events, skill selection, MCP, interrupt, vision — every true flag has a passing test/live check.
- Token usage renders and `ModelContextWindow` displays for Grok turns, or absence is explicit (`GR-24`).
- Drive sync / cross-PC restore / stale-account recovery works for Grok via a `LocateSessionFile` branch, or returns a typed-unsupported state without corrupting history (`GR-23`).
- Concurrent/grouped approval + question cards work for Grok without wedging the run (`GR-29`); the prompt is withheld until FlowPilot MCP is ready (`GR-30`); resumed turns re-attach MCP + approval (`GR-31`).
- Session-id integrity guards pass: no resume on a synthetic-only id, and post-prompt session-id adoption (`GR-32`); replay shows the typed user prompt (`GR-34`).
- `ReasoningEffort` maps to Grok ACP effort ids or degrades to default explicitly (`GR-35`); child stop/delete cascades with no orphaned artifacts (`GR-36`).
- Grok can be a hub agent and a review-loop participant, not only spawn children (`GR-33`); cross-provider handoff target works and source is proven or typed-unsupported (`GR-28`).
- **PLUGIN-ONLY / ZERO BASE REGRESSION (`P-0`) is proven:** every shared file is additive-only, `gemini_acp_transport.go` is untouched, and `defaultModelForProvider`/`summarizerModelFor`/`supportsHandoffSource`/`resolvePromptExecutionAdapter`/`LocateSessionFile` return byte-identical values for codex/claude/gemini inputs (`GR-BR`, `E2E-36`).
- Existing Codex/Claude/Gemini behavior is unchanged; every touched shared file keeps regression coverage.
- A live acceptance run records one real Grok desktop turn covering chat, an approval-gated action, a spawned child agent, a generated summary, a flow-rule gate, resume, and account switch.

### 10.1 DOD Verification Checklist

| DOD Item | Required Verification |
| --- | --- |
| `grokAdapter` implements `ProviderRuntimeAdapter`. | Unit test constructs the adapter, asserts `Key()==ProviderKeyGrok`, verifies explicit `Capabilities()`, and calls `SendTurn` against a fake ACP transport driven by captured fixtures. |
| Live registry returns real Grok adapter. | Registry test: default registry stays placeholder-safe; live registry resolves Grok only when the ACP process/deps are available. |
| Shared ACP transport + dispatcher. | Tests prove the generalized ACP helpers still produce Gemini's exact request/response, and the `grokDispatcher` multiplexes sessions + routes `session/request_permission` to `inbound`. |
| Ambient MCP scanning disabled + no leak. | Test asserts the spawned process env includes `GROK_CLAUDE_MCPS_ENABLED=false`/`GROK_CURSOR_MCPS_ENABLED=false`; a log-scrub test asserts transport-surfaced credentials are never logged. |
| Normalized events only. | Event-mapper tests turn captured `session/update`/`turn_completed` frames into shared `ProviderEvent`s (start, delta, message complete, tool/file, fail, complete). |
| YOLO=false approval gate. | **DONE** — unit/fake-dispatcher + live write gate (Task-208); desktop Drive MCP (`google-drive__authGetStatus`) ApprovalCard with deny blocks (BUG-273, 2026-07-11). |
| YOLO=true runner policy. | **DONE** — unit tests + Task-218 `--always-approve` / no card; desktop Drive MCP auto-runs under YOLO=on (2026-07-11). |
| MCP Drive YOLO gate (BUG-273). | **DONE / no PreToolUse** — Task-221 cancelled won't-do; path is Task-208 `handleInbound` + Task-218 `permission_mode=default` under YOLO=off; user verified approve/deny/YOLO-on + 2–3× stability. |
| `ask_user` question UI. | Integration test: Grok calls `ask_user` → `user_question_required` → answer → turn continues. |
| `spawn_agent` + multi-agent. | Integration test: wait=true/false children persist, appear in agent panel, inherit context; orchestrator graph/bus events emit. |
| Skill + context injection exactness. | Prompt-assembly test compares Grok skill/context prompt content+order with Codex/Claude for one and many skills. |
| Modes + routing. | Mode tests run Grok in normal/task/bugfix/`workflow_step_auto`; `grok-*` routes to `ProviderKeyGrok`. |
| Flow rules + summaries. | Gate tests run `r-ca`/`r-bug`/`r-task` on Grok; summary tests cover manual + idle. |
| Detection + version baseline. | Test: `detectProvider("grok")` finds the binary + version; `RunCompatCheck` classifies vs `CompatTestedGrokVersion`. |
| Accounts: isolate/detect/connect/switch. | Tests cover `~/.grok` + `GROK_HOME` + managed `~/.grokHomeN` deterministic IDs; connect creates isolated home; `SetActiveAccount` interrupts in-flight turns; next turn uses the new home. |
| Quota classification. | Test maps `402`/`spending-limit` to a terminal usage-limit error (not login, not recoverable-retry). |
| Resume + persistence. | Restart/resume test persists ACP `sessionId`, restarts runner, resumes or returns typed mismatch. |
| Desktop surface. | Manual + component tests: Grok appears in settings (installed/version), account panel (email/plan/models/switch), and chat picker (chip/icon/model list/default model). |
| Capability truthfulness. | Capability test asserts every true flag has a passing behavior test; `Vision` stays false until proven. |
| Others unchanged. | Codex/Claude/Gemini adapter + registry + account tests remain green; shared-file regression coverage added. |
| Token/context reporting. | Test asserts `EventTokenUsageUpdated` + `ModelContextWindow` populate from Grok `turn_completed._meta`/`initialize`, or render explicit absence (`GR-24`). |
| Drive sync / cross-PC restore. | Sync/restore test proves same-provider stale-account recovery via a Grok `LocateSessionFile` branch, or a typed-unsupported state; cross-PC ndjson resume proven (`GR-23`). |
| Handoff target + source. | Target test passes; Grok-as-source (+ summary-based modes) enabled only after `~/.grok/sessions` extractor tests pass, else typed-unsupported; Codex/Claude source stays green (`GR-28`). |
| Concurrent/grouped approval. | Two simultaneous gated Grok tool calls surface as a grouped card, resolve independently, run does not hang (`GR-29`). |
| MCP-ready-before-prompt. | Test proves the prompt is withheld until FlowPilot MCP connects; slow init still yields `ask_user`/`spawn_agent` on turn 1 (`GR-30`). |
| Resumed-turn MCP/approval. | Resumed Grok turn re-attaches `mcpServers` + permission channel; gated tool + `ask_user` still work (`GR-31`, BUG-087). |
| Session-id integrity. | Synthetic-id resume fails explicitly (no fresh `session/new`); post-prompt session-id adoption proven (`GR-32`). |
| Replay typed user prompts. | Reopened Grok chat shows typed user input, not composed prompt (`GR-34`, BUG-046/083). |
| Reasoning-effort mapping. | `ReasoningEffort` maps to Grok ACP effort ids; unmapped value degrades to default (`GR-35`). |
| Hub + review-loop. | Grok `autoOrchestrate` run drives a cohort review loop to convergence via `SubmitFlowControl` (`GR-33`). |
| Child stop + delete-cascade. | Stopping/deleting a Grok parent stops children; no orphaned home/session artifacts (`GR-36`, BUG-086). |
| Persistent exec allowlist. | Grok exec approvals set `ApprovalDetails.Kind=="exec"` so the per-project "don't ask again" allowlist engages (`GR-04`, BUG-246). |
| Reserved tool-name safety. | FlowPilot spawn/ask tool names do not collide with native `spawn_subagent`/`ask_user_question`; normalized at the UI boundary (`GR-07`, BUG-124). |
| Base-regression guard (`P-0`). | `gemini_acp_transport.go` untouched; `defaultModelForProvider`/`summarizerModelFor`/`supportsHandoffSource`/`resolvePromptExecutionAdapter`/`LocateSessionFile` return byte-identical values for codex/claude/gemini; full existing suites green (`GR-BR`, `E2E-36`). |
| End-to-end parity pass. | Manual script records one real Grok desktop run: chat, approval, spawned child, summary, flow gate, resume, account switch. |

### 10.2 Current Verification Status

_Updated through Task-221/BUG-273 close-out (2026-07-11). Earlier rows retain Task-206..218 evidence; MCP Drive YOLO gate row added below._

| Area | Status | Evidence |
| --- | --- | --- |
| ACP transport contract (`initialize`/`session/new`/`session/prompt`/`session/update`/`session/request_permission`/`turn_completed`) | **Verified live, twice** | Original CP-authoring probe (`grok 0.2.93`) plus a second independent live probe during Task-206 implementation (`~/.grok/bin/grok.exe agent stdio`, real `grok.com`-authenticated account), captured in `apps/local-runner/internal/runner/testdata/grok_acp/live_probe_raw.txt`. Corrected two assumptions from the original CP text: (a) `reasoningEfforts` for grok-4.5 lists only `high/medium/low`, not the full `none..xhigh/max` set; (b) turn completion is signaled by the `session/prompt` RPC's own response (`stopReason`+`_meta` token usage), not a separate notification. |
| Environment hazard: a different `grok` binary can shadow the real one on PATH | **New finding, mitigated** | The npm package `@vibe-kit/grok-cli` installs its own unrelated `grok` executable with no `agent stdio`/ACP support at all; on a machine where it precedes the real xAI Grok Build binary on PATH, naive `grok agent stdio` would launch the wrong tool. Mitigated via `FLOWPILOT_GROK_BIN` override (`grokBinaryName`, `grok_process.go`) mirroring the existing `FLOWPILOT_CODEX_BIN` pattern. |
| Approval round-trip (server→client `session/request_permission` with `options[]`; reply `{outcome:{selected, optionId}}`) | **Verified live (2026-07-10) + unit-tested; Task-218 auto-enforces** | Decision-vocabulary + YOLO on/off routing unit-tested. Live write gate: `permission_mode="always-approve"` bypassed channel; Task-218 now rewrites to `"default"` under YOLO=false at process ensure (plus `--always-approve` only when YOLO=true). See Task-208 / Task-218. |
| MCP Drive tool gate under YOLO (BUG-273) | **DONE live desktop 2026-07-11 — PreToolUse won't-do** | User: YOLO=off → ApprovalCard for `google-drive__authGetStatus`; deny blocks (no email); YOLO=on → no card + auto-run; stable 2–3× same prompt. Mechanism = Task-208 `handleInbound` (Grok emits MCP permission) + Task-218 posture + Drive in `config.toml` — **not** proxy pending store, **not** Task-209 prompt reinforcement, **not** Task-221 PreToolUse (cancelled). |
| Tool + file events (`tool_call`/`tool_call_update` with diff) | **Verified live** | A second live probe (Task-206) drove a real `read_file` tool call end to end and captured the exact `tool_call`/`tool_call_update` frames now encoded as golden fixtures in `grok_process_test.go`. A live **write/exec** tool call was not captured in this pass (only read was exercised), so `grokPermissionKind`'s exec/file classification is implemented per-docs but not independently live-verified for a write. |
| Real server session id + resume shape | **Verified live (2026-07-10) — real bug found and fixed** | `session/new`/`session/prompt` real `sessionId` confirmed live. `session/load{sessionId,cwd,mcpServers}` (`Q-2`) is now exercised end to end against a real resumed session (`TestLiveRealGrokChatStreamAndResume`): a fresh turn, then a resumed turn that correctly recalls turn-1 context. This live run caught a real bug the ACP-spec-only implementation missed: `session/load` and `session/prompt` responses nest `sessionId` under `result._meta.sessionId`, not top-level like `session/new` — `grokACPResponseSessionID` (`grok_acp.go`) only checked the top level, so every resume failed with "no sessionId" until a `_meta` fallback was added. See Task-206 `DOD-7` / Task-207 `DOD-6`. |
| MCP over ACP `mcpServers` + ambient-scan hazard | Hazard mitigated (env flags); **Drive MCP live-exercised on desktop (2026-07-11)** | Ambient marketplace MCP still a residual risk (Atlassian plugin probe). FlowPilot + Drive wiring unit-tested; desktop Grok + Drive email prompt proves end-to-end MCP use + YOLO gate (BUG-273). ask_user/spawn_agent live closed under Task-209. |
| Sub-agents (native `spawn_subagent`/`ask_user_question`) | **Could not confirm** | The second live probe's `initialize` result carried no tool list at all (only `availableCommands`, i.e. slash commands) — no `spawn_subagent`/`ask_user_question` entries were observed. CP-46's original claim is neither confirmed nor refuted by this pass; Task-209 proceeded on the MCP-over-ACP path (T-1's stated preference) without attempting the native-shim fallback, so the reserved-tool-name-collision risk (BUG-124/GR-07) is unverified either way. |
| Usage/quota | Signal only (unchanged) | Turn-time `402 personal-team-blocked:spending-limit` still the only known signal (`Q-5`); classified in `isProviderUsageLimitError`. No machine-readable quota endpoint found. |
| Adapter / registry / desktop UI / accounts | **Built** | `grokAdapter`/`grokDispatcher`/`grok_process.go`/`grok_permission.go`/`grok_event_mapper.go` implement chat/stream/resume/YOLO/permission/MCP-wiring/token-usage/reasoning-effort-mapping; live registry enablement is wired behind `FLOWPILOT_GROK_AGENT` (off by default, mirrors Codex); ~15 additive provider switches across `runner.go`/`provider_accounts.go`/`compat.go`/`cli/root.go` cover detect/connect/switch/quota-classification; desktop `contract.ts`/`adminModels.ts`/`adminLogic.ts`/`ChatInput.tsx`/`ProviderAccountsPanel.tsx`/`store.ts`/`styles.css`/`AgentsPanel.tsx`/`MockRunnerClient.ts`/`AiProvidersSettings.tsx`/`CheckVersionSettings.tsx`/`runner.ts` all carry the `"grok"` case. 43 Grok-specific Go tests pass (34 fake-transport/unit + 6 free live-detect/read-only checks + 3 opt-in `FLOWPILOT_LIVE_GROK=1` live E2E tests, all against a real logged-in account); the full existing suite is unchanged (same 15 pre-existing, unrelated failures as the clean baseline, verified before and after). |
| Full CP-46 DOD parity | **Task-212 live smoke closed 2026-07-11** | Task-212 closed with credentialed desktop smoke (skills/context, r-ca/r-task/r-bug, Gen summary, Grok→Codex handoff, resume continue, token UI, orchestration/stop). Formal E2E-01..36 matrix not fully tabulated — accepted as smoke. Prior: session/load + YOLO permission fixes. Handoff source: **enabled** via jsonl (`supportsHandoffSource(grok)=true`), not SQLite. Task-210 multi-account connect/switch/resume closed 2026-07-15. Residual optional: exhaustive E2E table. |

## 11. Completion Notes

- result: **done** 2026-07-11. Core CP-46 goals shipped and live-smoked via Task-206..212 (+ spin-offs 213–216, 218, 220–221).
- residual: none — Task-210 closed 2026-07-15 (multi-account connect, switch, cross-account chat continue, post-restart resume).
- formal E2E-01..36 matrix: accepted as smoke via Task-212 (not a full tabulated matrix).
- upstream: this CP and Task-210 both `done/`.
