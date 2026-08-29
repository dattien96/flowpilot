# CP-57: Opencode Provider Integration (Controlled Adapter + Settings + Chat/Flow Parity)

## Metadata

- Document ID: `CP-57`
- Title: `Opencode Provider Integration (Controlled Adapter + Settings + Chat/Flow Parity)`
- Phase: `coding_plan`
- Status: `in_progress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-27`
- Last Updated: `2026-08-29` (moved todo→inprogress; CA-679 lands) (exhaustive desktop+runner scan synced)
- Parent Documents: [SS-05: Workflow AI Provider](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md), [SS-12: Multiple Agents](../../05-System-Specs/SS-12-Multiple-Agents.md), [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md), [SD-16: Agent Spawn And Tool Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md), [SD-11: MCP Connection Flows](../../06-System-Tech-Design/SD-11-MCP-Connection-Flows.md)
- Child Documents: [Task-300: Opencode ACP Transport And Process/Dispatcher](../../08-Task/inprogress/Task-300-Opencode-ACP-Transport-And-Process-Dispatcher.md), [Task-301: Opencode Controlled Adapter MVP](../../08-Task/inprogress/Task-301-Opencode-Controlled-Adapter-MVP.md), [Task-302: Opencode Settings — Detect/Install/Models/MCP/Account](../../08-Task/inprogress/Task-302-Opencode-Settings-Detect-Install-Models-MCP-Account.md), [Task-303: Opencode Chat/Flow — Model/Reasoning/YOLO/Cards/Tools](../../08-Task/inprogress/Task-303-Opencode-Chat-Flow-Model-Reasoning-YOLO-Cards-Tools.md)
- Related Documents: [CP-46: Grok Build Controlled Adapter Over ACP](../done/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md), [CP-40: Gemini Controlled Adapter](../todo/CP-40-Gemini-Adapter-Plan.md), [07 — Claude Provider Adapter Plan](../../10-Refactor/New-System/07-Claude-Adapter-Plan.md), [04-07 — Phase 7: Providers Capability Packaging](../../10-Refactor/New-System/04-07-Phase7-Providers-Capability-Packaging.md), [SD-14: Codex Cross-Account Chat Resume And Home Sync](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md)
- Replaces: `None`
- Tags: `opencode, ai-providers, adapter, acp, mcp, local-runner, desktop-chat, settings`
- Feature Keys: `ai-providers`

## AI Quick View

### Summary

- Add **Opencode** (`opencode` / `opencode-go`, verified `1.18.23`) as a first-class controlled-mode provider alongside Codex, Claude, Gemini, Grok — reusing the same `ProviderRuntimeAdapter` / `TurnBridge` / `ProviderEvent` abstraction (`apps/local-runner/internal/runner/provider_registry.go:88`).
- Opencode is **not** a single LLM; it is a **zen proxy** that routes `opencode/<model>` and `opencode-go/<model>` to 40+ upstream models (`opencode models` lists `opencode/gpt-5.6-terra`, `opencode/claude-opus-5`, `opencode/grok-4.6`, `opencode/muse-spark-1.2-contributor-free`, …). One adapter must handle many `ProviderKeyOpencode` model prefixes, not one model per provider.
- **Transport verified 2026-08-27**: `opencode run --format json --model opencode/muse-spark-1.2-contributor-free --dir <cwd> "hello"` emits `{"type":"step_start"}` → `{"type":"text","text":"ok"}` → `{"type":"step_finish","reason":"stop"}` + `_meta` tokens; config `model: opencode/deepseek-v4-flash-free` fails with `ProviderModelNotFoundError` (`--print-logs` shows `Did you mean: deepseek-v4-flash`). CLI help shows `opencode acp` (ACP server), `opencode serve` (headless server), `opencode providers`, `opencode models`, `opencode stats`. Controlled interactive parity must use **ACP** (`opencode acp`), not one-shot `run`, to get permissions/MCP/sub-agents/session resume — `run --format json` stays as the **summarizer one-shot exception** (Grok `P-13` analog).
- Settings + Chat/Flow must reach parity with Codex/Claude/Grok: Settings detects installed binary/version, installs CLI, lists supported models, one-click writes Google/Jira MCP into Opencode's `opencode.json` `mcpServers`, shows account/usage (with explicit "token limit not exposed" for zen); Chat/Flow supports per-turn `model`/`reasoning` (`--variant`)/`yolo` (`--auto`)/`chatPosture` and renders `ask_user` + approval + tool cards through `TurnBridge`.

### Current Ask

- Author the code-grounded plan that closes Opencode from `DefaultProviderRegistry` **without** opencode (like Grok `provider_registry.go:221-247`) to live `ProviderRegistryFor` `Available` without regressing Codex/Claude/Gemini/Grok, and that makes Settings and Chat/Flow behave identically for Opencode.

### Key Decisions

- `P-0` **PLUGIN-ONLY — ZERO BASE REGRESSION.** Opencode is additive. Every shared/base file gets an appended `opencode` branch only; no reordering of existing `codex/claude/gemini/grok` cases, no signature change, no mutation of `gemini_acp_transport.go`/`claude_mcp_server.go`. Each touched shared file ships with a regression test proving Codex/Claude/Gemini/Grok branches still behave identically (mirrors `CP-46 P-0`).
- `P-1` Drive Opencode through **`opencode acp` (ACP JSON-RPC 2.0)** for the interactive turn loop. `opencode run --format json` is **summarizer-only** (one-shot, cheap `opencode/gpt-5.4-nano` or `opencode/muse-spark-1.2-contributor-free`). `opencode serve` is deferred (WebSocket, future multi-client).
- `P-2` Adapter implements `ProviderRuntimeAdapter` (`Key==ProviderKeyOpencode`, `Capabilities`, `SendTurn`) and emits only normalized `ProviderEvent`. No Opencode-only desktop flow.
- `P-3` Copy the **Codex/Grok process/dispatcher model**: one persistent `opencode acp` stdio process per `scopeKey` (account), async dispatcher with `waiters` + per-`sessionId` notification subs + single inbound handler + `fail()` drain, teardown on account switch. Do NOT use a spawn-per-turn model.
- `P-4` Build a **standalone Opencode ACP module** (`opencode_acp.go`, `opencode_process.go`, `opencode_event_mapper.go`); do NOT refactor `gemini_acp_transport.go` or `grok_acp.go`. Share only provider-neutral helpers (`claudeMCP` reuse via ACP `mcpServers` if applicable).
- `P-5` Permission = **ACP server→client request** (mirrors Grok `session/request_permission` and Codex inbound). Map `permission` (`question`/`plan_enter`/`plan_exit`/tool allowlist) to `bridge.RequestApproval` and encode decision back with the exact option ids Opencode offers. YOLO derives from runner `YoloPosture` SSOT; never trust Opencode's persisted `permission` array.
- `P-6` **`clientCapabilities.fs.* = false`** initially so Opencode uses its own file tools and routes each write through the permission channel; derive `EventFileChanged` from `tool_call` diffs.
- `P-7` **MCP** via ACP `session/new{mcpServers:[flowpilot, google-drive, jira]}` reusing the runner-hosted `claudeMCP` HTTP MCP server. The one-click Settings writer must update **Opencode's own config** (`~/.config/opencode/opencode.json` `mcpServers`), not Codex `config.toml` nor Claude `settings.json`.
- `P-8` **Model routing**: `providerKeyFromModel` adds `opencode/` + `opencode-go/` prefixes (appended last, per `CP-46 P-0`). `defaultModelForProvider("opencode")` returns `opencode/muse-spark-1.2-contributor-free` (verified working) or `opencode/gpt-5.4-nano`; `opencode/muse-spark-1.2-contributor` family is the built-in agent default (`opencode.json: agent.build.model`).
- `P-9` **Reasoning** = Opencode `--variant` (`high`/`medium`/`low`/`minimal`/`max`); map `TurnRequest.ReasoningEffort` via `opencodeReasoningVariantID()` (appended mapper, do not touch `grokReasoningEffortID`).
- `P-10` **YOLO** = Opencode `--auto` (`--auto Auto-approve permissions that are not explicitly denied (dangerous!)`) plus runner `YoloPosture`. Map `YoloMode=true` → `--auto` + `RunnerAutoApprove`; `ask_user` never auto-approved.
- `P-11` **Account = Opencode zen account** (`opencode providers` / `opencode auth` / `~/.config/opencode/auth.json`-like storage). Unlike `GROK_HOME` multi-home, Opencode is single zen token proxying many upstreams. Show email/plan from `opencode providers`; do **not** claim per-model token limit — zen does not expose upstream limits; surface `opencode stats` cost/tokens instead with explicit "limit N/A" note.
- `P-12` **Capability flags stay `false` until proven.** `Vision=false` (`initialize` analog `promptCapabilities.image` unconfirmed for Opencode ACP); `TokenUsageUpdated` only when ACP `turn_completed` `tokens`/`cost` (via `_meta` like Grok) is observed — `step_finish.tokens` is one-shot `run --format json` contrast only.

### Constraints

- Additive-only per `P-0`; `gemini_acp_transport.go` stays byte-identical.
- Runner owns workflow state, approvals, prompt assembly, finalizer, orchestration; desktop needs only capability rendering + enum additions.
- Security: never log plaintext credentials echoed by MCP `servers_updated`; strip inherited secrets when spawning `opencode acp`.
- Persist Opencode session as real `sessionId` via `ProviderSessionStore.UpsertSession{ProviderKey:"opencode"}`; synthetic `thread-*` never resumes.
- Existing `grok`/`claude`/`codex`/`gemini` branches must keep exact prior behavior and gain regression coverage.

### Open Questions

- `Q-1` Exact ACP resume request for Opencode: `session/load{sessionId,cwd,mcpServers}` vs extended shape? Is session portable across zen accounts or account-bound?
- `Q-2` Which `ProviderCapabilities` can Opencode truthfully claim day-one? `run --format json` proves streaming/text/tokens/sessionId; approval/MCP/sub-agent/skill require ACP live proof on `opencode acp 1.18.23`.
- `Q-3` Does Opencode ACP expose `ask_user` as MCP tool or native `question` permission? `opencode.json` shows `permission:[{permission:"question",pattern:"*",action:"deny"}]` — `ask_user` may be a `question` permission request, not an MCP `ask_user` tool, requiring a different bridge shim than Claude/Grok.
- `Q-4` Where does Opencode store `sessions` on disk for `LocateSessionFile` / Drive restore? `~/.config/opencode` vs `~/.local/share/opencode` vs project `.opencode`?
- `Q-5` Quota is `opencode stats` (cost/tokens per session/model) not per-model limit — should Settings show `stats` aggregated usage and hide `monthlyLimit` chip for Opencode?
- `Q-6` `grok-4.6` appears both as native Grok and as `opencode/grok-4.6` upstream — how to avoid model-picker ambiguity (`opencode/grok-4.6` vs `grok-4.5` native) without duplicating cards?

### Source Refs

- Live verification `2026-08-27`: `opencode --help` / `opencode run --help` / `opencode models` / `opencode run --format json --print-logs` (session `ses_*`, `ProviderModelNotFoundError`, `step_finish` tokens `total:9660`), `~/.config/opencode/opencode.json` (`model: opencode/deepseek-v4-flash-free`, `agent.build.model`, `agent.plan.model: xai/grok-4.6`).
- `requirements/07-Coding-Plan/done/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md` (template for parity matrix, P-0 guard, work breakdown)
- `requirements/07-Coding-Plan/todo/CP-40-Gemini-Adapter-Plan.md` (G-01..G-27 parity checklist)
- Runner: `apps/local-runner/internal/runner/provider_registry.go:202` (`providerKeyFromModel`), `provider_event.go:10` (`ProviderKey` consts), `provider_accounts.go`, `yolo_resolver.go`, `claude_mcp_server.go`, `interactive_service.go:8995` (`defaultModelForProvider`), `summarizer.go:31`, `handoff_context.go:142` (`supportsHandoffSource`), `session_file_locator.go:19` (`LocateSessionFile`) / `grok_transcript_loader.go:208` (`isGrokRealSessionID`), `compat.go:20`, `codex_adapter.go:138`, `grok_adapter.go:83` (`Key()`) / `grok_adapter.go:85` (`grokRunSessionIndex`), `claude_adapter.go:71`, `gemini_adapter.go:42`, `placeholder_adapters.go`
- Desktop: `apps/desktop-flowpilot/src/types/contract.ts:11` (`ProviderKey`), `components/settings/AiProvidersSettings.tsx:22` (`detectModels/installProvider/connectAccount`), `components/ChatInput.tsx:71` (`PROVIDER_CARDS`), `state/store.ts:263` (`ChatPosture`), `client/MockRunnerClient.ts`
- `requirements/10-Refactor/New-System/04-Detailed-Coding-Plan.md` (P2 contract), `requirements/10-Refactor/New-System/07-Claude-Adapter-Plan.md`, `requirements/06-System-Tech-Design/SD-06-AI-Provider-Integration.md`

## 1. Goal

Make **Opencode** a first-class controlled-mode provider:

1. **Settings page** can detect if Opencode CLI is installed, offer one-click install of the correct CLI version, list supported models, one-click write Google/Jira MCP into Opencode's settings, and show account info + token-limit/usage (degrading explicitly to "N/A" when zen does not expose per-model limits).
2. **Chat & Flow** support per-turn `model` / `reasoning` (`--variant`) / `yolo` (`--auto`) / `chatPosture` (`scan`/`plan`/`code`) and render provider-neutral `ask_user` / approval / tool cards and `EventFileChanged` through `TurnBridge`.
3. No desktop-only Opencode transport; all turns go through `ProviderRuntimeAdapter.SendTurn` → normalized `ProviderEvent` → shared finalizer/orchestration (`finishTurn`, `agent_orchestrator`, flow gates).
4. Additive, regression-gated, capability-honest (a flag stays `false` until a test or live check proves it).

## 2. Input Documents

- [SS-05: Workflow AI Provider](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md)
- [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- [SS-12: Multiple Agents](../../05-System-Specs/SS-12-Multiple-Agents.md)
- [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md)
- [SD-16: Agent Spawn And Tool Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- [SD-11: MCP Connection Flows](../../06-System-Tech-Design/SD-11-MCP-Connection-Flows.md)
- [SD-14: Codex Cross-Account Chat Resume And Home Sync](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md)
- [04 - Detailed Coding Plan](../../10-Refactor/New-System/04-Detailed-Coding-Plan.md)
- [07 — Claude Provider Adapter Plan](../../10-Refactor/New-System/07-Claude-Adapter-Plan.md)
- [04-07 — Phase 7: Providers Capability Packaging](../../10-Refactor/New-System/04-07-Phase7-Providers-Capability-Packaging.md)
- [CP-46: Grok Build Controlled Adapter Over ACP](../done/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md)
- [CP-40: Gemini Controlled Adapter](../todo/CP-40-Gemini-Adapter-Plan.md)

## 3. Implementation Strategy

- **overall approach:**
  - Reuse the controlled contract: `ProviderRuntimeAdapter`, `TurnBridge`, `ProviderEvent`, `InteractiveService`, `ProviderRegistry`.
  - Treat **ACP over `opencode acp`** as transport substrate; generalize only by copying Codex/Grok dispatcher shapes, not by refactoring their transports.
  - Add Opencode through provider-neutral extension points only; desktop needs only enum/label/icon/color additions.
  - Mirror the proven per-provider stack: transport/process → event mapper → adapter → registry enablement → settings UI → chat/flow UI → parity hardening.
- **sequencing logic:**
  - Validate ACP wire shapes from a captured `opencode acp` session (like Grok `P-1`) before claiming any capability; keep `run --format json` only for the summarizer one-shot (Task-300 must NOT freeze `run --format json` `step_finish` as ACP).
  - Land transport/process/dispatcher first (Task-300 live ACP probe), then adapter + mapper (Task-301), then permission/YOLO + MCP + `ask_user`/`spawn_agent` (Task-303), then Settings (detect/install/models/MCP/account/limits — Task-302), then DOD hardening.
  - Keep `DefaultProviderRegistry` **without** opencode (like Grok `provider_registry.go:221-247` — only Codex available, Claude/Gemini placeholder); enable `Available` in `ProviderRegistryFor` (live registry) only when `FLOWPILOT_OPENCODE_AGENT` on and account resolvable (Task-301 gate).
- **dependencies:**
  - Provider-neutral runtime contract (`provider_event.go`, `provider_registry.go`).
  - Codex/Grok dispatcher shape (`codex_appserver.go`, `grok_process.go`) as reference to copy.
  - Provider-account discovery (`provider_accounts.go`, `runner.go`).
  - Runner-hosted FlowPilot MCP server (`claude_mcp_server.go`) for `ask_user`/`spawn_agent` if ACP exposes `mcpServers`.
  - Tooling check (`internal/tooling/check.go`) + compat baseline (`compat.go`).

### 3.1 Required Parity Matrix

| Capability | Codex/Claude/Grok baseline | Opencode requirement | Live evidence | Validation |
|---|---|---|---|---|
| Normal chat + streaming + final message | Desktop turns via runner emit normalized stream events + persist history | ACP `session/update` text chunks → `message_delta`/`message_completed`; ACP `turn_completed` → `turn_completed` (`step_finish` is one-shot `run --format json`, not ACP) | `run --format json` `step_start/text/step_finish` verified for **one-shot** `opencode run`; ACP `session/update` / `turn_completed` shapes **must be captured via `opencode acp` in Task-300** (not `run`) | `OC-01`, `OC-10` |
| Task / bugfix mode | `changeType=task|bugfix` flows through same interactive run | Opencode accepts task/bugfix turns identically | — | `OC-02` |
| Workflow-step routing | `workflow_step_auto` resolves provider from model | `opencode/*` + `opencode-go/*` → `ProviderKeyOpencode` | `opencode models` lists both prefixes | `OC-03` |
| Tool events | `tool_started`/`tool_completed` render in UI | Opencode `tool_call` → mapper → tool events; `FileEvents` from write/edit diffs | `run` log shows `shell tool using ... powershell.EXE` | `OC-11` |
| YOLO off (approval gate) | Dangerous actions → `permission_required`; deny blocks | ACP permission request → `TurnBridge.RequestApproval`; deny blocks | `permission` `question`/`plan_*` observed in `opencode.json` | `OC-04` |
| YOLO on | Runner auto-approve eligible; `ask_user` still surfaces | `YoloMode=true` → launch `--auto` + `RunnerAutoApprove`; `ask_user` never auto-approved | `opencode run --help` `--auto` flag | `OC-05`/`OC-06` |
| User questions | `ask_user` → `user_question_required` | Opencode `ask_user` (MCP or `question` permission) blocks for desktop answer | — | `OC-06` |
| Spawn agents | `spawn_agent` → `TurnBridge.SpawnAgent`; wait true/false | Opencode `spawn_agent` via MCP or native sub-agent shim → same bridge | Native sub-agent list unconfirmed for Opencode ACP | `OC-07`/`OC-22` |
| Skill injection | Selected skills prepended in order | Opencode `promptPrep` calls same `injectSelectedSkills`; `promptPrep` appends opencode tool reinforcements like Grok `grokToolReinforcements` | — | `OC-08` |
| Context injection | Feature history + audit + summary injected | Opencode uses same shared prompt assembly | — | `OC-09` |
| Flow rules | `r-ca`/`r-bug`/`r-task` via `finishTurn` | Opencode turns reach `finishTurn`; gate-repair prompts run on Opencode | — | `OC-10` |
| MCP (FlowPilot + external) | FlowPilot `ask_user`/`spawn_agent` + Google Drive visible together | ACP `session/new{mcpServers:[flowpilot, google-drive, jira]}`; disable ambient discovery | `opencode mcp` command exists | `OC-17`/`OC-21` |
| Setting: detect installed | `tooling.json` + `flowpilot providers list` | `opencode --version` (1.18.23) + `CompatTestedOpencodeVersion`; `CheckVersionSettings` row | `OPENCODE=1` env observed | `OC-25`/`OC-18` |
| Setting: install CLI | One-click install of correct version | `providerInstallCommand` `opencode` → `npm i -g opencode-ai` or `irm https://opencode.ai/install | iex` (win) | `opencode upgrade` exists | `OC-18` |
| Setting: supported models | Show models for this provider | `opencode models` → sync to `ai_supported_models` with `provider_key='opencode'` | 40+ models listed | `OC-25` |
| Setting: MCP one-click | Google/Jira write to provider's config | Settings writes `~/.config/opencode/opencode.json` `mcpServers` (not Claude/Codex path) | `opencode mcp` manage | `OC-29` |
| Setting: account + token limit | Panel shows model, quota, auth | Opencode `providers`/`stats` → show usage/cost; token limit chip hides with tooltip "Zen proxy — limit N/A, see upstream provider" | `opencode providers`/`stats` | `OC-20`/`OC-30` |
| Reasoning | `reasoningEffort` knob | `--variant` mapper (`opencodeReasoningVariantID`) | `opencode run --help` `--variant` | `OC-35` |
| Token usage + context window | `EventTokenUsageUpdated` + `ModelContextWindow` | Map ACP `turn_completed` `tokens`/`cost` + `initialize` context window to desktop | `tokens:{total,input,output,reasoning,cache}` verified via `turn_completed._meta` (like Grok) — `step_finish.tokens` is one-shot contrast only | `OC-24` |
| Resume / history | Real sessionId persisted; resume after restart | `ProviderSessionStore.UpsertSession{ProviderKey:"opencode", sessionId}` + `session/load` branch | `sessionID: ses_*` observed | `OC-12`/`OC-15` |
| Cross-account | Deterministic account IDs | Single zen account; multi-account = multiple `OPENCODE_CONFIG` dirs or `OPENCODE_HOME` isolation; explicit mismatch handling | — | `OC-12`/`OC-14` |
| Cross-provider handoff | Source extractors + summary handoff | Opencode-as-target immediate; Opencode-as-source after `sessions` extractor proven | — | `OC-28` |
| Interrupt | `ctx.Done()` → cancel | ACP `session/cancel` → `ctx.Err()` | Grok analog proven | `OC-16` |
| Vision | `Vision=false` until proven | Opencode image input unconfirmed → keep `false` | — | `OC-27` |

## 4. Work Breakdown

Each `P-item` below maps to a finalized child Task (IDs: Task-300..303). **Namespace note:** `Key Decisions` `P-*` and `Work Breakdown` `P-*` are distinct namespaces — tasks cite **Work Breakdown §4 `P-*`**. Mapping:

| Work Breakdown | Child Task | Key Decisions covered |
|---|---|---|
| `P-1` Freeze ACP contract | Task-300 | `P-1` ACP transport |
| `P-2` ACP transport/process | Task-300 | `P-3`, `P-4` process model |
| `P-3` Adapter | Task-301 | `P-2`, `P-3`, `P-4` |
| `P-4` Event mapper | Task-301 | `P-12` caps |
| `P-5` Permission/YOLO | Task-301 (MVP stub) + Task-303 (full) | `P-5`, `P-10` |
| `P-6` ask_user/spawn_agent | Task-303 | `P-5` (permission `session/request_permission` → `bridge.RequestApproval`) + `P-7` (MCP `mcpServers`) — not `P-6` (`P-6` is `clientCapabilities.fs.*=false`) |
| `P-7` Settings | Task-302 | `P-7`, `P-11` |
| `P-8` Chat/Flow UI | Task-303 | `P-8`, `P-9` |
| `P-9` Resume/history/handoff | Task-301 (persist + `session/load` resume in `SendTurn`) + Task-303 (`LocateSessionFile` + Drive/handoff) | `P-2`, `P-12` |
| `P-10` Skill/context/gates/summaries | Task-303 | `P-8`, `P-9`, `P-12` |
| `P-11` Tests/operator notes | Task-300..303 | `P-0`, `P-12` |

- `P-1` **Freeze Opencode ACP contract from captured wire shapes** (→ Task-300, pre-adapter).
  - Capture `initialize` / `session/new{cwd,mcpServers,permission}` / `session/prompt` / `session/update` (`text`/`tool_call`/`tool_call_update`/`plan` analog) / permission request / `turn_completed` payloads from `opencode acp` `1.18.23` (`step_finish` is one-shot `run --format json`, not ACP — keep separate as contrast).
  - Record `x.ai/*`-like extensions Opencode actually emits; mark non-exhaustive.
  - Decide day-one `ProviderCapabilities` with proof per flag; record that default config `model: opencode/deepseek-v4-flash-free` is **broken** (verified) and must be replaced by `opencode/muse-spark-1.2-contributor-free` or `opencode/gpt-5.4-nano` for smoke, and `agent.build/plan` models (`opencode/deepseek-v4-flash-free`, `xai/grok-4.6`) must be validated.

- `P-2` **Opencode ACP transport + process/dispatcher** (Task-300).
  - Add `opencode_acp.go` + `opencode_process.go` modeled on `grok_process.go`/`codex_appserver.go`: `opencodeDispatcher` with `waiters` (request/response), per-`sessionId` notification subs, single `inbound` handler for permission/`question`/`fs/*`, one read-loop, `fail()` drain.
  - `ensureOpencodeProcess(ctx, scopeKey, cwd, env, model, variant, auto)` modeled on `ensureGrokProcess`/`ensureCodexAppServer`: spawn `opencode acp` (binary `opencode --pure --port 0` avoidance; `opencodeBinaryName` var + `opencodeAgentEnabled()` gate via `FLOWPILOT_OPENCODE_AGENT`), run `initialize`, hold one shared `*opencodeProcessHandle` on `*Runner` (`opencodeProcessMu` + `opencodeProcess` fields).
  - Launch env hard-sets `OPENCODE_*` isolation, strips inherited secrets, sets `OPENCODE_CONFIG`/`HOME` per managed account.

- `P-3` **Build `opencodeAdapter` implementing `ProviderRuntimeAdapter`** (Task-301).
  - `Key()==ProviderKeyOpencode`; `Capabilities()` from `P-1`; `SendTurn` honoring every `TurnRequest` field (`Cwd`→`session/new{cwd}`, `ModelName`/`ReasoningEffort` via `opencodeReasoningVariantID`, `SelectedSkills`+context via `promptPrep`, `Attachments` if proven, `ProviderSessionID` resume, `OfferReviewOutcomeTool` gating).
  - MCP-ready-before-prompt gate (mirror Claude `waitReady`/`Grok init_progress`): withhold `session/prompt` until FlowPilot MCP `tools/list` arrives, else first turn drops `ask_user`/`spawn_agent`.

- `P-4` **Event mapper** `opencode_event_mapper.go` (Task-301), modeled on `grok_event_mapper.go:58` / `codex_event_mapper.go`.
  - Map `session/update` text chunks → `message_delta`/`message_completed`, `tool_call` → `tool_started`, `tool_call_update` → `tool_completed` + `EventFileChanged` from diffs; ACP `turn_completed` → `turn_completed`/`turn_failed`; `turn_completed._meta` `tokens`/`cost` → `token_usage_updated` (fields per Task-300 ACP capture — `step_finish` is one-shot, not ACP); leave `ProviderTurnID` empty (core stamps it).

- `P-5` **Permission channel + YOLO posture** (Task-301 / Task-303).
  - Route inbound permission/`question` → build `ApprovalDetails` (tool title, rawInput, kind `exec`/`file`/`mcp`, cwd) → `bridge.RequestApproval` → encode decision back with exact option ids Opencode offers.
  - Add `OpencodePermissionMode` to `YoloPosture` (`yolo_resolver.go`): YOLO=true → `--auto`+`RunnerAutoApprove`, YOLO=false → default + live gating. `ask_user` never auto-approved.
  - Guard against stale `opencode.json` `permission` array bypassing runner toggle (Claude `CA-079` analog).

- `P-6` **`ask_user` + `spawn_agent` via MCP** (Task-303).
  - Expose FlowPilot tools via ACP `session/new{mcpServers:[flowpilot]}` reusing `claudeMCP` + `mcpBaseURL` + `claudeMCPToolDefs` (provider-neutral) **or** native `question` shim (Q-3); document choice.
  - `spawn_agent` → `TurnBridge.SpawnAgent`; support `wait` true/false, agent panel, inherited provider/yolo/context; normalize label `flowpilot_spawn_agent` collision avoidance like Codex `flowpilot_spawn_agent` `codex_adapter.go:70`.

- `P-7` **Settings page — Detect / Install / Models / MCP / Account / Limits** (Task-302).
  - **Detect**: `providerSpecs()` add `{Key:"opencode", Label:"Opencode", BinaryName:"opencode", InstallHint}`, `tooling/check.go:33` `CheckTool(name string, repoDir string)` via `CheckTool("opencode", repoDir)` + `opencode --version`, `compat.go` `CompatTestedOpencodeVersion="1.18.23"` + `CompatConfig`/`RunCompatCheck` + `compatRunVersion("opencode")`.
  - **Install**: `providerInstallCommand` opencode case → `npm i -g opencode-ai@latest` (fallback `irm https://opencode.ai/install | iex`), `AiProvidersSettings.tsx:239` `installProvider("opencode")` button appears once `GET /providers` returns `opencode`; generalize gemini-only install message.
  - **Models**: `opencode models` (or `opencode models --format json`) → `Task-213`-style auto-sync into `ai_supported_models` with `provider_key='opencode'`; `models [provider]` lists `opencode/*` and `opencode-go/*`; `store.ts` `pickDefaultModel` opencode branch.
  - **MCP**: `internal/runner/opencode_mcp_provider_config.go` + `apps/desktop-flowpilot/src/components/settings/McpSettings` extension: one-click Google/Jira writes `~/.config/opencode/opencode.json` `mcpServers` entry (HTTP MCP `{command, env, args}`), parallel to `claude_mcp_server.go` / `google_drive_mcp_provider_config.go` but targeting `opencode.json` path per `SD-11`. Add `OPENCODE_CONFIG` isolation per managed account.
  - **Account + Limits**: `provider_accounts.go` `managedProviderHomePrefix` → `.opencodeHomeN` or `OPENCODE_HOME`; `DiscoverProviderAccountHomes` + `isValidOpencodeAccountPath` (`opencode.json`/`auth.json`/`config.json`); `loadAccountLaunchMetadata` opencode branch (`loadOpencodeAccountMetadata`: email/plan from `opencode providers` + `opencode stats` cost/tokens). **Token limit N/A**: zen proxy does not expose upstream `monthlyLimit`; Settings chip shows `—` with tooltip "Opencode is a zen proxy — token limit depends on upstream provider (e.g. `opencode/claude-opus-5`), see `opencode stats` for usage/cost" and hides `TokenUsage` progress bar when `ModelContextWindow==nil`.
  - **Version baseline**: `compat.go:20` Opencode version gate; `CheckVersionSettings.tsx:219-233` **4th row** for Opencode (currently 3 rows: Claude/Codex/Grok).

- `P-8` **Chat / Flow UI parity** (Task-303).
  - `types/contract.ts:11` `ProviderKey` `| "opencode"`; `adminModels.ts` `SupportedModel.providerKey` `| "opencode"`; `ChatInput.tsx:71` `PROVIDER_CARDS` add Opencode card + `OpencodeIcon`; `AiProvidersSettings.tsx` `installProvider` generalize; `ProviderAccountsPanel` `PROVIDERS` add `{key:"opencode",label:"Opencode"}`; `store.ts` `providerLabel` opencode + `pickDefaultModel` opencode branch + `resolveProviderKeyForModel` `opencode/` prefix (`provider_registry.go:202`); `styles.css` `--opencode-brand`.
  - Reasoning picker `tui/app/picker_model_reasoning_test.go` adds opencode `variant` list; Chat `model`/`reasoning`/`yolo`/`chatPosture` controls propagate via `TurnRequest` to opencode adapter's launch flags (`--model`/`--variant`/`--auto`).
  - Card parity: `permission_required` → grouped approval queue (`ca195_parallel_approval_queue_test.go`), `user_question_required` → `QuestionCard` with `multiSelect`, `tool_started/completed` → `TuiToolGroup`, `file_changed` → `fileChanged` event, all provider-neutral (`GR-BR` analog).

- `P-9` **Resume / history / handoff** (Task-301 + Task-303 split).
  - Task-301: persist real `sessionId` via `ProviderSessionStore.UpsertSession{ProviderKey:"opencode"}` after `session/new`; resume via `session/load{sessionId,cwd,mcpServers}` or typed mismatch; synthetic `thread-*` never `session/load` — starts a fresh session (2026-08-29 reconciliation: Grok BUG-324 parity, shipped in Task-301; the original "explicit fail" was relaxed because `TestOpencodeAdapterSyntheticIdFails` now asserts fresh-session semantics and mid-chat continuation beats a hard error); registration in `ProviderRegistryFor` only (Task-301 gate), `DefaultProviderRegistry` **without** opencode like Grok `provider_registry.go:221-247`.
  - Task-303: `LocateSessionFile` opencode branch (+ `isOpencodeRealSessionID` analog to `grok_transcript_loader.go:208 isGrokRealSessionID`) for Drive restore / cross-account resume; `handoff_context.go:142 supportsHandoffSource` stays `false` until extractor proven; do not corrupt history.

- `P-10` **Skill / context / flow gates / summaries** (Task-303, parity with Grok `P-10`).
  - Skill/context injection through shared `promptPrep`/`injectSelectedSkills` (no Opencode-specific reformatting).
  - Flow gates: Opencode turns reach `finishTurn`; `r-ca`/`r-bug`/`r-task` repair prompts run on Opencode.
  - Summaries: manual + idle summary for Opencode chats via shared summarizer (`summarizerModelFor` opencode case → `opencode/gpt-5.4-nano` cheap).

- `P-11` **Tests, operator notes, fallback policy** (spans Task-300..303).
  - Contract/integration tests per `OC-*`; base-regression coverage for every shared file touched.
  - Operator docs: `opencode` install (`npm i -g opencode-ai`), `opencode auth login`, managed-home model, known gaps (token limit N/A, model `opencode/deepseek-v4-flash-free` broken).

## 5. Touched Areas

- **files (runner, extend):**
  - `apps/local-runner/internal/runner/provider_registry.go` (register `opencode`, `providerKeyFromModel` `opencode/`+`opencode-go/` cases appended last, adapter factory **deferred** — `newAdapter` vs `newAdapterForTurn` decided after Task-300 live probe, analog to Grok `provider_registry.go:455`)
  - `apps/local-runner/internal/runner/provider_event.go:10` (`ProviderKeyOpencode = "opencode"`)
  - `apps/local-runner/internal/runner/yolo_resolver.go` (`OpencodePermissionMode` / `opencodeAutoApprove` field, appended, read only by Opencode — Task-301 adds field, Task-303 wires full policy)
  - `apps/local-runner/internal/runner/provider_accounts.go` (`managedProviderHomePrefix`, `DiscoverProviderAccountHomes`, `NextAccountHomePath`, `getEnvForExecution` `OPENCODE_*` strip/injection, `isValidOpencodeAccountPath`, `syncManagedProviderAccounts` loop add `"opencode"` — Task-302)
  - `apps/local-runner/internal/runner/runner.go` (`providerSpecs:1669`, `defaultAuthCandidates`, `accountAuthPaths`, `hasValidProviderAuthFile`, `getEnvForExecution:1155`, `providerEnvSetCommand`, `StartInteractiveAuth`/`AuthenticateProvider` opencode → `opencode auth login`, `providerAuthStatus`, `providerInstallCommand` opencode case — Task-302)
  - `apps/local-runner/internal/runner/interactive_service.go:8995` (append `opencode` case to `defaultModelForProvider` + `isProviderUsageLimitError` + `resolvePromptExecutionAdapter` one-shot `runner.go:937` — **Task-303 exclusive**, not Task-302)
  - `apps/local-runner/internal/runner/summarizer.go:31` (append `opencode` case to `summarizerModelFor` only — Task-303 exclusive)
  - `apps/local-runner/internal/runner/handoff_context.go:142` (`supportsHandoffSource` stays `false` until extractor proven — Task-303 deferred)
  - `apps/local-runner/internal/runner/session_file_locator.go:19` (append `opencode` case to `LocateSessionFile` — **Task-303 exclusive**)
  - `apps/local-runner/internal/runner/google_drive_mcp_provider_config.go` + new `opencode_mcp_provider_config.go` (`getProviderConfigPath` include `opencode/opencode.json`, provider loop add `opencode` — Task-302, scope Google/Jira only)
  - `apps/local-runner/internal/runner/compat.go:20` (`CompatTestedOpencodeVersion="1.18.23"` + config/info/probe — APPEND fields, never reorder so `CheckVersionSettings.tsx:219-233` 4th row stays valid for Codex/Claude/Grok)
  - `apps/local-runner/internal/cli/root.go` (`/providers`, `/provider-accounts/*` loops add `opencode` — Task-302)
  - `apps/local-runner/internal/tooling/check.go:33` (`CheckTool(name string, repoDir string)` via `CheckTool("opencode", repoDir)` — path `apps/local-runner/internal/tooling/check.go`)
- **files (runner, new):**
  - `opencode_acp.go` (ACP param builders, `mcpServers` entry builder, `sessionId` reader, `agent_message_chunk` text extractor — copy shape from `grok_acp.go`, do NOT mutate Grok)
  - `opencode_process.go` (dispatcher + `ensureOpencodeProcess` + handle, modeled on `grok_process.go`)
  - `opencode_adapter.go` (`ProviderRuntimeAdapter` `provider_registry.go:88`)
  - `opencode_event_mapper.go`
  - `opencode_mcp_provider_config.go`
  - `opencode_adapter_test.go`, `opencode_event_mapper_test.go`, `opencode_process_test.go`, golden fixtures `testdata/opencode_acp/*`
  - `opencode_reasoning.go` (`opencodeReasoningVariantID` mapper)
- **files (desktop, extend):** `types/contract.ts:11`, `components/settings/AiProvidersSettings.tsx:22` (detect/install/models/MCP), `components/settings/CheckVersionSettings.tsx`, `components/ProviderAccountsPanel.tsx`, `components/ChatInput.tsx:71` (+ `OpencodeIcon`), `components/AgentsPanel.tsx`, `state/store.ts:263` (`ChatPosture` already provider-agnostic, add `providerLabel` + `pickDefaultModel`), `client/HttpWsRunnerClient.ts`, `client/MockRunnerClient.ts`, `styles.css`; `packages/flowpilot-client-core/src/domain/adminModels.ts`, `adminLogic.ts`, `runner.ts`, `data/runnerAdminRepository.ts`
- **modules:** local runner provider runtime; ACP transport; desktop chat + settings + account panel; provider account resolution; workflow session persistence/resume; agent orchestration bridge; flow-gate finalization; summary generation
- **database:** no schema change (`workflow_provider_sessions`/`events`/`approvals`/`questions` and `ai_supported_models`/`default_provider` treat `provider_key` as opaque text). Add `ai_supported_models` rows with `provider_key='opencode'` via sync (Task-302).
- **external systems:** `opencode` CLI / `opencode acp` ACP process; local `~/.config/opencode` (`opencode.json`, `auth.json`, `stats`), project `.opencode`; optional Google Drive / Jira MCP servers over ACP `mcpServers`

### 5.1 Base-Regression Guard (per `P-0`)

| Shared file | Change type | Regression guard |
|---|---|---|
| `gemini_acp_transport.go`, `grok_acp.go`, `codex_adapter.go`, `claude_adapter.go` | **NOT TOUCHED** | Do not mutate; Opencode gets standalone module |
| `provider_registry.go` | Additive (`opencode` reg in `ProviderRegistryFor` only, `opencode/` prefix appended last) | Existing `gpt-/gemini-/claude-/grok-` cases unchanged; registry test proves `DefaultProviderRegistry` has no opencode (like Grok `provider_registry.go:221-247`) |
| `provider_event.go` | Additive (`ProviderKeyOpencode`) | Trivial |
| `yolo_resolver.go` | Additive (`OpencodePermissionMode` field) | Codex/Claude/Grok `YoloPosture` fields byte-identical |
| `provider_accounts.go`, `runner.go` | Additive (`case "opencode"` appended in ~15 switches incl. `defaultModelForProvider`, `resolvePromptExecutionAdapter`) | Per-function test asserts codex/claude/gemini/grok branches return prior values |
| `interactive_service.go`, `summarizer.go`, `handoff_context.go`, `session_file_locator.go` | Additive (`opencode` case appended) | Other-provider summary/handoff/locate unchanged |
| `compat.go` | Additive (append struct fields) | JSON shape for Codex/Claude/Grok unchanged (append, never reorder) |
| `claude_mcp_server.go`, `agent_orchestrator.go` | **REUSE only, no mutation** | Provider-neutral; if generalization tempting, stop |
| Desktop shared (`contract.ts`, `store.ts`, `ChatInput.tsx`, `AiProvidersSettings.tsx`, `styles.css`) | Additive (`| "opencode"`, new card/branch) | `AiProvidersSettings` install-button generalization must keep Gemini/Grok affordance; `store.test.ts` enum assumptions updated; Codex/Claude/Grok UI unchanged |

`OC-BR` + `E2E-01..27` are the executable proof of this table (no `E2E-36` — E2E stops at 27).

### 5.2 Full Provider/Model Feature Mirror Checklist (exhaustive scan 2026-08-27, desktop+runner)

Scan roots: `apps/local-runner/internal/runner` (438 files) + `internal/tui` (252) + `internal/cli` + `apps/desktop-flowpilot/src` + `packages/flowpilot-client-core/src`. Every `switch providerKey` / `ProviderKey*` / `providerKeyFromModel` must gain `opencode` appended last.

| # | Area | Runner file(s) : line hint | Desktop file(s) | Mirror for `opencode` | CP task |
|---|---|---|---|---|---|
| 1 | `ProviderKey` const + `ProviderCapabilities` | `runner/provider_event.go:10` `ProviderKeyCodex/Claude/Gemini/Grok` | `desktop/types/contract.ts:11`, `packages/.../adminModels.ts:89` | `ProviderKeyOpencode="opencode"` + `Capabilities{Streaming,Resume,ApprovalEvents,FileEvents,SkillSelection,Mcp,Interrupt,Vision:false}` | `P-2/P-12` Task-301 |
| 2 | `ProviderRegistry` / `ProviderRegistryFor` / `DefaultProviderRegistry` / `Adapter(model,effort)` | `runner/provider_registry.go:122-553` (`newAdapter` vs `newAdapterForTurn` `104-119`) | `desktop/types/contract.ts:11` union consumed | Add registration in `ProviderRegistryFor` only (like Grok — `DefaultProviderRegistry` `:221-247` stays without opencode); freeze `newAdapter` vs `newAdapterForTurn` **after** Task-300 live ACP probe: if `model/variant/auto` are launch flags → `newAdapterForTurn` (like Grok), else `newAdapter` (like Claude/Codex) | `P-3` Task-301 |
| 3 | `providerKeyFromModel` + `defaultModelForProvider` + `resolveTurnModelAndEffort` | `runner/provider_registry.go:202`, `runner/interactive_service.go:8995`, `runner/workflow_state_machine.go:69` | `packages/.../adminLogic.ts:3 resolveProviderKeyForModel` | Append `opencode/` + `opencode-go/` (covers xAi + Go plan), `defaultModelForProvider("opencode")="opencode/muse-spark-1.2-contributor-free"` — **Task-303 exclusive** (not Task-302) | `P-8` Task-303 |
| 4 | `resolvePromptExecutionAdapter` (one-shot summarizer) | `runner/runner.go:937` `switch resolvedProvider` (`codex→--sandbox`, `claude→--print --effort`, `gemini→agy --print`, `grok→--single -p`) + `ExecutePrompt usesPromptArg` | — | Add `opencode` branch → `opencode run --format json -m <model> --variant <effort> --auto` + `usesPromptArg=true` — **Task-303 exclusive** (not Task-301) | `P-10` Task-303 |
| 5 | Detection / Install / Inventory `DetectProviders(Cached)` | `runner/runner.go:354-408` `providersCacheTTL=3s`, `internal/tui/client/client.go:692 GET /providers` | `desktop/components/settings/AiProvidersSettings.tsx:22` | `providerSpecs() add {Key:"opencode", Binary:"opencode"}` + `detectProvider` probe `opencode --version` + `invalidateProvidersCache` on auth/install | `P-7` Task-302 |
| 6 | `InstallProvider` / `runProviderInstallCommand` | `runner/runner.go:419`, `cli/root.go:262 POST /providers/install`, `tui/app/app.go:743 ProviderInstallMsg` | `desktop/AiProvidersSettings.tsx:239 installProvider` special `gemini?"AGY CLI"` | Opencode branch `npm i -g opencode-ai` fallback `irm https://opencode.ai/install|iex` | `P-7` Task-302 |
| 7 | Live model catalog probe | `runner/runner.go:1632 codexDebugModel` + `1872 detectGrokModels` `~/.grok/models_cache.json` → `types.go:28 ProviderModel{SupportedReasoningEfforts,ContextWindowTokens}` | `desktop/AiProvidersSettings.tsx:120 detectModels` inserts `source:"detected"` | `detectOpencodeModels()` via `opencode models` (40+ `opencode/*` + `opencode-go/*`); map `SupportedReasoningEfforts=variant` levels, `ContextWindowTokens` if exposed | `P-7` Task-302 |
| 8 | Managed homes / `NextAccountHomePath` / `DiscoverProviderAccountHomes` | `runner/provider_accounts.go:640 managedProviderHomePrefix ".codexHome/.claudeHome/.grokHome"` `727 discover*` `919 isValid*` | `desktop/ProviderAccountsPanel.tsx:7 PROVIDERS` hardcode 4 | Add `".opencodeHome"` + `isValidOpencodeAccountPath` (`opencode.json`/`auth.json`), add `{key:"opencode",label:"OpenCode"}` to `PROVIDERS` | `P-7` Task-302 |
| 9 | Auth file check `HasLocalAuthAtPath` / `DetectDefaultAccountHomePath` / `syncProviderAccounts` | `runner/runner.go:2253,4278`, `runner/provider_accounts.go:327 syncProviderAccounts` | — | Add opencode `auth.json`-like shape (`~/.config/opencode/auth.json` or `providers` creds) | `P-7` Task-302 |
| 10 | Env isolation `HOME/XDG_CONFIG_HOME/GEMINI_HOME/GROK_HOME/CLAUDE_CONFIG_DIR` | `runner/provider_registry.go:270 Codex CODEX_HOME`, `322 Claude HOME/XDG`, `395 Gemini GEMINI_HOME`, `460 Grok GROK_HOME`, `runner/runner.go:1155 getEnvForExecution` | — | `OPENCODE_HOME` / `OPENCODE_CONFIG` / `HOME` mapping for opencode managed homes | `P-7` Task-302 |
| 11 | Connect/Verify/Activate account + terminal auth | `runner/provider_accounts.go:60 Connect/Verify/Activate`, `cli/root.go:368 /provider-accounts/*`, `provider_account_terminal.go:186` | `desktop/ProviderAccountsPanel.tsx:196 connectAccount → client.connectProviderAccount` | `StartInteractiveAuth opencode → opencode auth login` (or `opencode login`) | `P-7` Task-302 |
| 12 | Reasoning vocab & per-turn threading | `runner/codex_appserver.go:47, claude_permission_mcp.go:59, grok_acp_types.go:15, tui/app/helpers.go:858` + `runner/provider_registry.go:500 grokEffort` | `desktop/ChatInput.tsx:94 FALLBACK_REASONING_OPTIONS`, `ChatPosturePanel.tsx:206-210`, `WorkflowsSettings.tsx:123`, `store.ts reasoningEffort` | Add `opencodeReasoningVariantID()` `high/medium/low/minimal/max` → `--variant` (mapper Task-301, UI fallback Task-303); keep Grok mapper untouched; add `ultra` label if opencode uses it | `P-9` Task-301 (mapper) + Task-303 (UI) |
| 13 | YOLO / `YoloPosture` SSOT | `runner/yolo_resolver.go:26 YoloPosture{CodexSandbox,ClaudePermissionMode,GrokPermissionMode,RunnerAutoApprove}` + `codex_adapter.go:621, grok_process.go:784 ApplyGrokYoloPosture POST /provider-accounts/grok-yolo-posture` | `desktop/store.ts:253 yoloMode/grokYoloPostureLoading`, `ChatInput.tsx:994 yolo-toggle`, `packages/.../contract.ts:29 yoloMode` | Add `OpencodePermissionMode` appended field; decide sync (`--auto` per-turn, like Codex/Claude) vs async endpoint `applyOpencodeYoloPosture` (like Grok). Default sync is safe. | `P-5/P-10` Task-301 |
| 14 | Chat posture `scan/plan/code` read-only | `runner/chat_posture.go:12`, `tui/app/chat_posture.go:1`, `desktop/ChatPosturePanel.tsx:11` `TurnRequest.ChatPosture` | — | No provider branch — adapter must force `untrusted`/gated modes for `scan/plan` | `P-5` Task-301 |
| 15 | `SkillSelection` injection + skill roots | `runner/runner.go:1308 injectSelectedSkills`, `skillpack/install.go:28` `claude→.claude/skills, agents→.agents/skills, grok→.grok/skills` | `desktop/store.ts:1293 loadSkills(provider)`, `ChatInput.tsx:245 /s` picker | Add `.opencode/skills` root; `opencodeAdapter.promptPrep = injectSelectedSkills + reinforcement` | `P-10` Task-303 |
| 16 | Runner-hosted MCP `claudeMCPServer` + per-turn `--mcp-config` | `runner/claude_mcp_server.go:19`, `claude_adapter.go:31, grok_adapter.go:36, codex_adapter.go syncCodexJiraMcpLive` + `google_drive_mcp_provider_config.go:1039 flowpilotClaudeExtraMCPServers(yolo)` | `desktop/McpSettings.tsx:22 PROVIDER_LABELS` + `GoogleDriveSettings.tsx:95` loops all providerAccounts | Add `opencode_mcp_provider_config.go` write `~/.config/opencode/opencode.json` `mcpServers`; `PROVIDER_LABELS["opencode"]="OpenCode"`; MCP `session/new{mcpServers:[flowpilot,google-drive,jira]}` | `P-7` Task-302 |
| 17 | `SpawnAgent` / cohort auto-orchestrate | `runner/agent_orchestrator.go:236, claude_mcp_server.go:271, codex_adapter.go:104 flowpilot_spawn_agent, grok_adapter.go:577 shim spawn_subagent` | `desktop/AgentsPanel.tsx:30 resolveMainAgentDisplay` `gpt-/claude-/gemini-` prefix, `415 providerOverride chips inherit/claude/codex/gemini/grok` | Add `opencode` chips + `isOpencodeSource` `.opencode` path badge + `prov-opencode` CSS; `defaultModelForProvider` covers child model | `P-6` Task-303 |
| 18 | `LocateSessionFile` + `is*RealSessionID` | `runner/session_file_locator.go:19 LocateSessionFile`, `runner/grok_transcript_loader.go:208 isGrokRealSessionID UUID`, `isCodexRealSessionID`, `chat_session_sync.go:447 resolveChatSessionTranscript` | — | Add `isOpencodeRealSessionID` + branch (`~/.config/opencode` sessions dir discovered `Q-4`) + reject `thread-*` | `P-9` Task-303 |
| 19 | Session store `UpsertSession` / `refreshResumeHandleLocked` / `Last*SessionID` | `runner/claude_adapter.go:237 persistSession`, `codex_adapter.go:285`, `grok_adapter.go:85 grokRunSessionIndex` (`:83` is `Key()`) runner-wide RunID→sessionId | — | `opencodeAdapter.recordSession` + `LastOpencodeSessionID` (+ `opencodeRunSessionIndex` if shared process like Grok) | `P-9` Task-301 |
| 20 | History replay `transcriptTurn` loaders | `runner/claude_transcript_loader.go, codex_transcript_loader.go, grok_transcript_loader.go:118, handoff_context.go:142 supportsHandoffSource` | `tui/app/chat_history_replay.go`, `desktop/Timeline.tsx` | `opencode_transcript_loader.go` (after store location proven) + `supportsHandoffSource` `opencode` stays `false` until proven (deferred) | `P-9/P-10` Task-303 |
| 21 | Cross-account resume / Drive sync restore | `runner/interactive_resume.go:2584`, `cross_account_resume_test.go`, `chat_session_sync.go:313 BuildChatSessionSyncManifest` `427 resolveGrokSessionSidecarFiles` | — | Handle directory vs single-file session copy for opencode (BUG-316 analog) | `P-9` Task-301 |
| 22 | Summarizer `summarizerModelFor` (cheap tier) | `runner/summarizer.go:31 summarizerModelFor` `claude→haiku, gemini→flash, codex/grok→""` | — | Add `opencode→opencode/gpt-5.4-nano` cheap via `summarizerModelFor` only; `resolvePromptExecutionAdapter` one-shot lives in row 4 | `P-10` Task-303 |
| 23 | Token usage `TokenUsageSnapshot{Last,Total,ModelContextWindow}` | `runner/*_event_mapper.go` `claudeTokenUsage, codexTokenUsage, grokPromptResultTokenUsage`, `provider_event.go:104 TokenUsageSnapshot` | `desktop/ChatInput.tsx:144 usageSummaryLine`, `store.ts:2863`, `ProviderAccountsPanel.tsx:67 compactUsageLines` filters `gemini` only | Map ACP `turn_completed` `tokens`/`cost` → `EventTokenUsageUpdated` (fields per Task-300 ACP capture — `step_finish.tokens` is one-shot, not ACP); add opencode `ModelContextWindow` from `initialize` if exposed | `P-12` Task-301 |
| 24 | `ProviderModel.ContextWindowTokens / MaxContextWindowTokens` | `runner/types.go:34, runner.go:1640` | `desktop/ChatInput.tsx usageSummaryLine fallback selectedModelInfo.contextWindowTokens` `Task-215` | Populate for opencode if `opencode models --json` exposes context window | `P-7` Task-302 |
| 25 | Compat `CompatTested*Version` | `runner/compat.go:20 "2.1.179" claude, "0.140.0" codex, "0.2.93" grok` | `desktop/CheckVersionSettings.tsx:219-233` (currently 3 rows Claude/Codex/Grok) | Add `CompatTestedOpencodeVersion="1.18.23"` + probe `opencode --version` + `compatRunVersion("opencode")` → **4th row** for Opencode | `P-11` Task-302 |
| 26 | Agent catalog `listAgents` | `runner/agent_catalog.go:27` reads `.claude/agents + .codex/agents` `parseAgentDefinition` | `desktop/AgentsPanel.tsx:379 isClaudeSource` | Add `.opencode/agents` dir + `isOpencodeSource` badge | `P-10` Task-303 |
| 27 | TUI picker `/provider /model /reasoning /yolo /mode /mode-setup` | `tui/app/app.go:594,928, helpers.go:1441 modelsForProvider`, `picker_model_reasoning_test.go:67` | `desktop/ChatInput.tsx:71 PROVIDER_CARDS +83 VISION_PROVIDERS`, `AgentsPanel.tsx`, `AiProvidersSettings.tsx:327 <option>` | Add `OpencodeIcon` + card + `VISION_PROVIDERS` decision, `PROVIDER_CARDS` chips CSS `provider-chip-opencode`, TUI `findProvider/providerForModel` includes opencode | `P-8` Task-303 |
| 28 | Image attachment gating `SupportsImages` vs path fallback | `tui/client/image.go`, `runner/codex_adapter.go:217 writeCodexImageAttachments base64 inline`, `runner/grok_adapter.go:332 writeGrokImagePathFallback .tmp/images`, `runner/provider_registry.go:242 Vision:true/false` | `desktop ChatInput.tsx:368 supportsVision = VISION_PROVIDERS.has(provider)` | Set `Vision:false` initially for opencode; add `opencode` to `VISION_PROVIDERS` only when ACP image input proven | `P-12` Task-301 |

> Every row is “append last” per `CP-46 P-0`; the 28-row table is the executable DOD for `Task-300..303`. Order is enforced by `providerKeyFromModel` / `providerSpecs` / `provider_registry` unit tests (P-0 regression), not `domain_hardcode_guard_test.go:16` (CP-42 role/step guard).

## 6. Data or Migration Steps

- **schema:** none. Reuse existing provider-session/event persistence; `provider_key` is free text.
- **data backfill:** none.
- **config updates:**
  - Resolution order: `OPENCODE_API_KEY` / `OPENCODE_CONFIG` env vs `opencode auth login` cached `~/.config/opencode/auth.json`; `HOME` override; per-account managed homes (`~/.opencodeHomeN` or `~/.config/opencode` per-account dir).
  - MCP config for Opencode is written to `~/.config/opencode/opencode.json` `mcpServers` (not Codex `config.toml` nor Claude `settings.json`); runtime FlowPilot tools go through ACP `session/new{mcpServers}` instead of file.
  - Always document `CompatTestedOpencodeVersion="1.18.23"` and broken default `opencode/deepseek-v4-flash-free` → replacement `opencode/muse-spark-1.2-contributor-free`.
  - Seed `ai_supported_models` with `opencode/models` output; prune stale `opencode/deepseek-v4-flash-free` entry.

## 7. Validation Plan

- **automated tests:**
  - `OC-01` Normal chat: Opencode streams `message_delta`, completes with `message_completed`+`turn_completed`, saves history, persists real `sessionId` (`ses_*`).
  - `OC-02` Task/bugfix: `changeType=task|bugfix` preserved, reaches finalizer.
  - `OC-03` Model routing: `opencode/*`/`opencode-go/*` → `ProviderKeyOpencode` via `workflow_step_auto`; `opencode/deepseek-v4-flash-free` rejected with typed error.
  - `OC-04` YOLO=false approval: `write`/shell → `permission_required`; deny (`reject`) blocks; allow → executes (golden-fixture round-trip).
  - `OC-05` YOLO=true policy: eligible actions auto-approve via runner policy; stale `opencode.json` `permission` array cannot bypass runner toggle.
  - `OC-06` `ask_user`: Opencode asks blocking question → `user_question_required` → answer returns → turn continues.
  - `OC-07` `spawn_agent`: wait=true and wait=false; child persists, appears in agent panel.
  - `OC-08` Skill injection: one-skill/multi-skill exact content+order via shared injector.
  - `OC-09` Context injection: feature history + audit + summary injected before turn.
  - `OC-10` Flow rules: `r-ca`/`r-bug`/`r-task` via `finishTurn`; gate-repair prompts run on Opencode.
  - `OC-11` Tool events: `tool_started`/`tool_completed` render; `FileEvents` from diffs.
  - `OC-12` Cross-account / resume: single zen account; synthetic `thread-*` never resumed (fresh session, Grok parity — 2026-08-29 reconciliation); `session/prompt` result `sessionId` adopted.
  - `OC-16` Interrupt: `ctx.Done()` → ACP `session/cancel` → `ctx.Err()`.
  - `OC-17` MCP readiness: withhold `session/prompt` until FlowPilot MCP `tools/list` arrives (first-turn drop guard).
  - `OC-18` Settings detect: `opencode --version` + compat baseline check; binary missing → `tooling.json` `missing`.
  - `OC-25` Models: `opencode models` sync produces `ai_supported_models` rows for `opencode`; model picker shows them.
  - `OC-29` MCP one-click: Settings Google/Jira button writes `opencode.json` `mcpServers` and is readable via `opencode mcp` list.
  - `OC-30` Account/limits: Settings account card shows email/plan/models; `stats` cost/tokens render; token limit chip shows `N/A` with tooltip and hides progress bar when `ModelContextWindow==nil`.
  - `OC-35` Reasoning: `TurnRequest.ReasoningEffort` → `opencodeReasoningVariantID` → `--variant`.
  - `OC-BR` Base regression: every touched shared file's Codex/Claude/Gemini/Grok branch returns prior values.
- **manual checks:**
  - Settings → `opencode` shows "Installed 1.18.23 (tested 1.18.23)"; install button on missing machine triggers `npm i -g opencode-ai`.
  - Settings → `Detect models` lists `opencode/gpt-5.6-terra` etc.; Chat model picker can select `opencode/gpt-5.4-nano` and `opencode/muse-spark-1.2-contributor-free`.
  - Settings → Google Drive `Connect` writes `~/.config/opencode/opencode.json` `mcpServers.google-drive`; `opencode mcp` confirms it.
  - Chat with Opencode: normal turn streams, YOLO off deny blocks write, YOLO on auto-approves, `ask_user` card appears and resumes, spawned child appears in agent panel.
  - Flow rag-harness on Opencode: `r-ca` gate reprompt runs on Opencode, not Codex.
- **failure cases:**
  - Opencode binary missing → `UnsupportedProviderRuntimeError` with install hint.
  - `ProviderModelNotFoundError: opencode/deepseek-v4-flash-free` → typed terminal error with suggestion list.
  - ACP `initialize`/`session/new` failure → normalized `turn_failed` recoverable.
  - Permission deny → `PermissionRejected` not wedge.
  - Synthetic `thread-*` resume → never `session/load`; a fresh session starts (2026-08-29 reconciliation: Grok BUG-324 parity — an explicit fail here cost mid-chat continuation; a real cross-account mismatch still refuses via the run-owner guard).
  - External MCP configured but not visible → capability stays `false` with diagnostic log.

### 7.1 E2E Test Items

| ID | Area | Provider(s) | Test Item | Expected Result |
|---|---|---|---|---|
| `E2E-01` | Basic chat | Opencode | New desktop Opencode chat, simple prompt, stream, restart/reopen | Text streams, final persists, history survives restart |
| `E2E-02` | Skill injection | Opencode, Grok, Claude | One-skill + multi-skill prompts on Opencode, repeat on Grok/Claude | Skill content/order preserved via shared injector |
| `E2E-03` | Context summary | Opencode, Grok, Codex | Long chat → `Gen summary` → continue | Summary injects into next Opencode turn without drift |
| `E2E-04` | Same-account resume | Opencode | Turn → continue in same runner session | Reuses real `ses_*` via scoped map |
| `E2E-05` | Restart resume | Opencode, Grok, Codex | Restart runner/app, resume all three | Opencode resumes only when safe else typed mismatch |
| `E2E-06` | Cross-account safety | Opencode, Grok, Codex | Chat under account A → switch to B → resume | No silent cross-account resume; Opencode returns mismatch |
| `E2E-07` | Synthetic resume guard | Opencode | Resume with `thread-*` + no real map | `thread-*` never `session/load`; fresh session starts (2026-08-29: Grok-parity reconciliation) |
| `E2E-08` | Prompt-result session id | Opencode | Turn where ACP `turn_completed` returns new `sessionId` via `_meta.sessionId` (like Grok `Task-207 DOD-6`) | Adopted as stored provider session id |
| `E2E-09` | Approval gate | Opencode, Grok, Codex | YOLO off, deny filesystem/shell action | Approval card, denial blocks, others unchanged |
| `E2E-10` | YOLO policy | Opencode, Grok, Codex | YOLO on, eligible + `ask_user` | Eligible follows policy; `ask_user` not auto-approved |
| `E2E-11` | Tool events | Opencode, Grok, Codex | Trigger tool start/completion | Shared provider-event UI renders |
| `E2E-12` | Flow gates | Opencode, Grok, Codex | Task/bugfix that triggers `r-ca`/`r-bug`/`r-task` | Gate violations + repair prompts stay on Opencode |
| `E2E-13` | Summary generation | Opencode, Grok, Codex | Manual + idle summary | Summaries persist and inject into later turns |
| `E2E-14` | Handoff target | Opencode, Grok, Codex | Handoff Codex/Claude → Opencode | Opencode receives context as target |
| `E2E-15` | Handoff source | Opencode, Grok, Codex | Handoff Opencode → Codex/Claude | Opencode-as-source disabled until extractor proven |
| `E2E-16` | Capability flags | Opencode, Grok, Codex | Inspect capabilities after run | Opencode advertises only proven flags |
| `E2E-17` | Child spawn | Opencode, Grok, Codex | Spawn blocking + non-blocking children | Children persist, agent panel correct |
| `E2E-18` | Failure recovery | Opencode, Grok, Codex | Force binary/auth/process failure | Normalized errors, no wedged run |
| `E2E-19` | Account metadata | Opencode | Refresh with `opencode providers` + `stats` | All buckets display; limit shows `N/A` with tooltip |
| `E2E-21` | External MCP | Opencode, Grok, Codex | Google Drive MCP with YOLO on/off | Opencode sees FlowPilot MCP tools or reports `false` with diagnostics |
| `E2E-22` | Child lifecycle | Opencode, Grok, Codex | Graph, approval, restart, wait true/false, YOLO | Opencode matches Codex/Grok semantics before agent capability expands |
| `E2E-23` | Drive sync/restore | Opencode, Grok, Codex | Sync chat, regen account ids, restore before open | Same-provider recovery works where supported else typed unsupported |
| `E2E-24` | Token/context UI | Opencode, Grok, Codex | Inspect token/context display after turns | Opencode reports real `tokens`/`cost` only else shows unavailable |
| `E2E-25` | Model metadata | Opencode | Edit/select `opencode/*` models + aliases | Display labels/aliases resolve correctly |
| `E2E-26` | History replay | Opencode, Grok, Codex | Reopen chats with tools/approvals/questions/children | Replay shows resolved states, no duplicate rows |
| `E2E-27` | Attachments fallback | Opencode, Grok, Codex | Image attachment while `Vision=false` | Opencode blocks before prompt loss |

## 8. Rollout and Fallback

- **rollout order:** keep `DefaultProviderRegistry` **without** opencode (like Grok `provider_registry.go:221-247`); enable `Available` in `ProviderRegistryFor` (live registry) only after `P-1` ACP live capture + `P-3` adapter wiring; land Task-300 transport (live `opencode acp` probe) → Task-301 adapter → Task-303 permission/MCP → Task-302 Settings UI → Chat/Flow UI → manual validation; feature-flag `FLOWPILOT_OPENCODE_AGENT=1` (default on, opt-out `0` like Grok `FLOWPILOT_GROK_AGENT`).
- **fallback path:** if ACP lacks structured permission/MCP surfaces, keep Opencode available only for `run --format json` one-shot (summarizer) and leave controlled chat disabled behind explicit partial-capability flag; if approval/question/agent semantics incomplete, expose reduced `Capabilities` instead of faking parity; if `LocateSessionFile` unproven, return typed unsupported without corrupting history.
- **monitoring:** runner logs `opencode acp` lifecycle, `sessionId`, transport failures; desktop history/resume; parity checks against Codex/Claude tool/approval/MCP surfaces; `opencode.json` broken default model alarm.

## 9. Risks

- `R-1` Opencode zen routing hides upstream failures as `ProviderModelNotFoundError`; adapter must surface suggestion diagnostics, not generic "Unexpected server error."
- `R-2` Default config `model: opencode/deepseek-v4-flash-free` is broken on `1.18.23`; silent fallback to a working default without user consent would mask config drift — must fail typed and surface suggestion.
- `R-3` `opencode.json` persists `permission` allowlist; adapter must override per turn so `YoloMode` stays authoritative (Codex/Grok `CA-079`/`BUG-069` analog).
- `R-4` `opencode models` lists 40+ models with `opencode/` and `opencode-go/` prefixes; naive picker merge duplicates `grok-4.6` etc. — needs disambiguation in `providerLabel`/`ModelSelect`.
- `R-5` Zen proxy exposes `stats` cost/tokens but **not** per-model token limits; claiming a limit would be misleading — must degrade explicitly.
- `R-6` Extracting ACP shapes from Grok/Codex may tempt refactoring `gemini_acp_transport.go`; per `P-0` this is disallowed — diff will be rejected if Gemini transport is touched.
- `R-7` Google/Jira MCP one-click must write to `opencode.json` atomically; partial write corrupts Opencode launch — need `writeFile+rename` + schema validation.
- `R-8` `opencode serve` WebSocket may be tempting for multi-client sharing before single-client `acp` stdio is proven — keep deferred; revisit only after `acp` parity passes.
- `R-9` Token `stats` is session-scoped, not account-wide; aggregating incorrectly across sessions would misreport usage — keep `stats` per-session and `providers` per-account separate.

## 10. Definition of Done

- A new Opencode adapter exists in the runner and implements `ProviderRuntimeAdapter` (`provider_registry.go:88`).
- Live runner registry (`ProviderRegistryFor`) can return a real Opencode adapter; `DefaultProviderRegistry` remains **without** opencode (like Grok `provider_registry.go:221-247` — only Codex available, Claude/Gemini placeholder) until Task-301 gate.
- Opencode desktop chat runs through the same `/client/workflow-runs/.../turns` flow as Codex/Claude/Grok; no Opencode-only endpoint.
- Opencode emits normalized `ProviderEvent` into the shared stream; no raw ACP messages reach desktop.
- Opencode session persistence + resume via real `ses_*` is documented and tested (synthetic `thread-*` never resumes; adopted id from ACP `turn_completed._meta.sessionId` like Grok `Task-207 DOD-6`).
- Capability reporting is truthful (`Streaming/Resume/Interrupt` gated by ACP proof; `Vision=false` until proven; `Mcp`/`ApprovalEvents`/`FileEvents` only when `P-5`/`P-6`/`P-4` pass).
- Legacy Codex/Claude/Grok/Gemini paths stay green; every shared file touched has regression coverage (`OC-BR`).
- Settings page: detect installed (`opencode --version`), one-click install correct version, `Detect models` syncs `opencode/*` + `opencode-go/*` into `ai_supported_models`, Google/Jira MCP one-click writes `opencode.json` `mcpServers`, account card shows `providers`/`stats` with token limit `N/A` tooltip and hides progress bar when `ModelContextWindow==nil`.
- Chat/Flow: model (`opencode/*` prefix), reasoning (`--variant`), YOLO (`--auto`), `chatPosture` (scan/plan `untrusted` / code) all propagate per turn; `ask_user` / approval / tool / `file_changed` cards render through `TurnBridge` identically to Grok/Claude.
- `r-ca`/`r-bug`/`r-task` and flow-gate repair prompts run correctly after Opencode turns; Opencode can `spawn_agent` with `wait` true/false and agent panel parity.
- Summaries (manual + idle) work for Opencode chats via `summarizerModelFor` cheap `opencode/gpt-5.4-nano`.
- End-to-end manual run records one real Opencode desktop run covering chat, approval-gated action, spawned child, summary, flow gate, and resume.

### 10.1 DOD Verification Checklist

| DOD Item | Required Verification |
|---|---|
| Opencode adapter implements `ProviderRuntimeAdapter` | Unit test constructs `newOpencodeAdapter`, asserts `Key()==ProviderKeyOpencode`, `Capabilities()` explicit, `SendTurn` against fake ACP transport with `session/update` / `turn_completed` fixtures (`step_start/text/step_finish` only for one-shot contrast) |
| Live registry returns real Opencode adapter | Registry test proves `DefaultProviderRegistry` has no opencode (like Grok) + live `ProviderRegistryFor` resolves `opencode` only when `opencode acp` available; `FLOWPILOT_OPENCODE_AGENT=0` opts out |
| Desktop Opencode chat uses shared turn endpoint | Manual `opencode` chat from desktop → network panel shows `POST /client/workflow-runs/.../turns`, not `/opencode/*` |
| Normalized events only | Event-mapper tests prove raw ACP `text`/`tool_call`/`tool_call_update`/`permission`/`turn_completed` become `ProviderEvent` `message_delta`/`message_completed`/`tool_started`/`tool_completed`/`permission_required`/`user_question_required`/`turn_failed`/`turn_completed` (`step_finish` is one-shot, not ACP) |
| Session persistence + resume | `TestOpencodeResumeAfterRestart` persists `ses_*`, restarts runner, resumes chat, history continuity verified; `TestOpencodeSyntheticIdFails` |
| Capability truthfulness | `TestOpencodeCapabilitiesMatchProvenSet` asserts every `true` flag has a passing `OC-*` test; `Vision` stays `false` |
| Base regression | `go test ./internal/runner -run 'TestProviderKeyFromModel|TestDefaultModelForProvider|TestYoloResolver|TestCompat'` green for codex/claude/gemini/grok after opencode branches added |
| Settings detect | `tooling/check_test.go` opencode case: `opencode --version 1.18.23` → `Status="ok"`; missing → `missing` with install hint |
| Settings install | Click `Install Opencode CLI` on missing machine → `provider_install_command_test.go` opencode case executes `npm i -g opencode-ai` and `tooling.json` flips to `ok` |
| Settings models | `TestOpencodeModelSync` runs `opencode models`, syncs 40+ rows with `provider_key='opencode'` into `ai_supported_models`, picker shows `opencode/gpt-5.6-terra` etc. |
| Settings MCP one-click | `TestOpencodeMcpWrite` clicks Google Drive connect → `~/.config/opencode/opencode.json` `mcpServers.google-drive` present with `command/env`; `opencode mcp` lists it |
| Settings account/limits | `TestOpencodeAccountMetadata` loads `opencode providers` + `opencode stats`; panel shows cost/tokens; token limit chip hides with tooltip "Zen proxy — limit N/A" when `ModelContextWindow==nil` |
| YOLO=false gate | `TestOpencodeApprovalDenied` requests dangerous action → `permission_required` → deny → action not executed, controlled failure |
| YOLO=true policy | `TestOpencodeYoloAutoApprove` proves eligible actions auto-approve only via runner policy; stale `opencode.json` allowlist cannot bypass |
| `ask_user` card | `TestOpencodeAskUser` makes Opencode call `ask_user` → `user_question_required` → desktop answer → turn continues |
| `spawn_agent` parity | `TestOpencodeSpawnAgent` spawns children `wait=true`/`wait=false` → child state + agent panel consistent |
| Skill injection exactness | `TestOpencodeSkillInjection` compares opencode selected-skill prompt content/order with Claude behavior |
| Context injection | `TestOpencodeContextInjection` verifies history/audit/summary/handoff included before model turn |
| Task/bugfix/workflow modes | `TestOpencodeModes` starts runs in `normal_chat`/`task`/`bugfix`/`workflow_step_auto` → provider/metadata/finalizer correct |
| Flow rules | `TestOpencodeFlowGate` triggers each of `r-ca`/`r-bug`/`r-task` on Opencode and verifies repair prompt stays on Opencode |
| Summary generation | `TestOpencodeSummarizer` `Gen summary` + idle summary inject into next Opencode turn; uses `opencode/gpt-5.4-nano` cheap model |
| Token/context UI | `TestOpencodeTokenUsage` asserts ACP `turn_completed` `tokens`/`cost` → `EventTokenUsageUpdated` and UI renders; absence degrades without crash (`step_finish.tokens` is one-shot contrast only) |
| End-to-end parity | Manual script records one real desktop Opencode run covering chat, approval, spawned agent, summary, flow gate, resume |
