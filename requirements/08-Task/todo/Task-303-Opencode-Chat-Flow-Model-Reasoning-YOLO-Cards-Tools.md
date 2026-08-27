# Task-303: Opencode Chat/Flow — Model / Reasoning / YOLO / Cards / Tools + Parity Hardening

## Metadata

- Document ID: `Task-303`
- Title: `Opencode Chat/Flow — Model / Reasoning / YOLO / Cards / Tools + Parity Hardening`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-27`
- Last Updated: `2026-08-27`
- Parent Documents: [CP-57: Opencode Provider Integration](../../07-Coding-Plan/todo/CP-57-Opencode-Provider-Integration.md), [Task-301: Opencode Controlled Adapter MVP](./Task-301-Opencode-Controlled-Adapter-MVP.md), [Task-302: Opencode Settings — Detect/Install/Models/MCP/Account](./Task-302-Opencode-Settings-Detect-Install-Models-MCP-Account.md)
- Child Documents: `None`
- Related Documents: [Task-208: Grok Permission Channel And YOLO Posture](../done/Task-208-Grok-Permission-Channel-And-Yolo-Posture.md), [Task-209: Grok MCP, Ask-User, And Spawn-Agent Parity](../done/Task-209-Grok-MCP-Ask-User-Spawn-Agent-Parity.md), [Task-211: Grok Desktop UI Surface](../done/Task-211-Grok-Desktop-UI-Surface.md), [Task-215: Per-Model Reasoning-Effort Detection](../done/Task-215-Per-Model-Reasoning-Effort-Detection-And-Model-Aware-UI.md)
- Replaces: `None`
- Tags: `opencode, chat, flow, model, reasoning, yolo, approval, ask-user, spawn-agent, parity`

## AI Quick View

### Summary

- Make **Chat** and **Flow** behave identically for `opencode` as for Codex/Claude/Grok: per-turn `model` (`opencode/`+`opencode-go/`), `reasoningEffort` (`--variant`), `yolo` (`--auto` + `RunnerAutoApprove`), `chatPosture` (`scan`/`plan`/`code`) propagate via `TurnRequest` to the adapter's launch flags; the run's `providerKey` pins correctly for duration of run.
- Render provider-neutral **cards** `permission_required` → grouped approval queue, `user_question_required` → `QuestionCard` with `multiSelect` + `__google_drive_picker__`, `tool_started/completed` → `TuiToolGroup` + `EventFileChanged`, all through shared `TurnBridge` and shared desktop `Timeline`/`chat_history_replay`.
- Wire **MCP per-turn readiness** (`waitReady` before `session/prompt`), **spawn_agent** `wait` true/false + inherited `provider/yolo/context`, **skill/context injection** reuse, **flow gates** `r-ca`/`r-bug`/`r-task` repair on Opencode, **resume/history** with real `ses_*`, **handoff** Opencode-as-target immediate, **summarizer** cheap model, and **parity hardening** (`OC-BR`, token/context, image gating, agent catalog).

### Current Ask

- Close Chat/Flow parity for Opencode: model/reasoning/yolo/posture per turn, approval/question/tool card parity, MCP `ask_user`/`spawn_agent` via ACP `mcpServers`, flow-gate reach-through, and the full `E2E-01..27` parity matrix with base-regression guard.

### Key Decisions

- `T-1` **Model routing is single source** `providerKeyFromModel` (`runner/provider_registry.go:202`) + `packages/flowpilot-client-core/src/domain/adminLogic.ts:3 resolveProviderKeyForModel` — both get the same `opencode/`+`opencode-go/` prefix appended last.
- `T-2` **Reasoning is per-model but falls back** to `FALLBACK_REASONING_OPTIONS` (`low/medium/high/xhigh/max`) when `SupportedModel.supportedReasoningEfforts` is empty (Claude pattern) — `desktop/ChatInput.tsx:94-128 reasoningOptionsFor(model)` already data-driven; `provider_registry.go` Opencode factory must pass mapped `variant` to `ensureOpencodeProcess` respawn key so changing effort respawns correctly like Grok `grokEffort`.
- `T-3` **YOLO sync, not async** (default): Opencode `--auto` is a per-turn launch flag, not a `config.toml` rewrite like Grok `always_approve`. Keep `store.ts toggleYoloForActiveProvider` sync path for Opencode (stay in `if(selectedProvider!=="grok")` sync branch); add `OpencodePermissionMode` to `YoloPosture` but do not add a new HTTP endpoint unless live probe proves config rewrite is required.
- `T-4` **Approval kind must be `exec`/`file`/`mcp`** for the `"don't ask again"` allowlist (`ApprovalDetails.Kind=="exec"` `BUG-246`) — Opencode inbound permission builder must set `Kind` correctly so the runner's per-project allowlist engages.
- `T-5` **MCP readiness gates prompt**: withhold `session/prompt` until FlowPilot MCP `tools/list` arrives (Claude `claudeMCPReadyDefaultTimeout` / Grok `init_progress` analog `grok_adapter.go:299`), else first turn drops `ask_user`/`spawn_agent` (class `BUG-114`).
- `T-6` **Spawn tool names**: avoid collision like Codex `flowpilot_spawn_agent` (`codex_adapter.go:70`); Opencode FlowPilot tool must be `flowpilot_spawn_agent` normalized back to `spawn_agent` at UI boundary, and `ask` must be `ask_user` on server `flowpilot` (match Grok reinforcement `grokAskUserReinforcement`).
- `T-7` **Vision stays `false`** until ACP image input (`promptCapabilities.image`) is proven; image attachments go via path fallback (write `.tmp/images/<turn>/` then append paths to prompt text) like Grok `CA-483` — desktop gates on `SupportsImages` (`ChatInput.tsx:83 VISION_PROVIDERS`).

### Constraints

- **PLUGIN-ONLY / ZERO BASE REGRESSION (CP-57 `P-0`):** additive-only; `gemini_acp_transport.go`/`grok_acp.go` untouched; `provider_registry.go` `newAdapter` vs `newAdapterForTurn` choice must not change Codex/Claude/Grok signatures; `store.ts` `PROVIDER_CARDS` chips, `AgentsPanel.tsx` `providerOverride`, `ProviderAccountsPanel.tsx` hardcode 4 providers only gain an appended 5th; `Chats` `workflow_step_auto` already provider-agnostic.
- Desktop `providerLabel` (`store.ts:2256` → `grok→Grok`) and `PROVIDER_LABELS` in `McpSettings.tsx:22` / `GoogleDriveSettings.tsx:95` only gain an `opencode:"OpenCode"` entry.
- Skill/context injection must reuse `injectSelectedSkills` path — no Opencode-specific formatter.
- No fabricated token limit; `Opencode` `ModelContextWindow` may stay `nil` → desktop shows `—` not `0`.

### Open Questions

- `Q-1` If Opencode treats `--variant`/`--auto` as launch flags (like Grok), should process respawn be per-`model+variant+auto` like `grokProcesses map[string]*grokProcessHandle` or per-`sessionId` like Codex? Default single-process-per-scope with respawn on any flag change (mirror Grok) is safe.
- `Q-2` Does `ask_user` surface as MCP tool or as `question` permission? Verify during `P-5` wiring; keep both shims ready.

### Source Refs

- `CP-57` Work Breakdown `P-5`..`P-11` (Key Decisions `P-5`..`P-11`), parity rows `OC-04`..`OC-17`, `OC-21`..`OC-28`, `OC-35`, `E2E-01`..`E2E-27`.
- `Task-301` (adapter MVP), `Task-302` (detection/models).
- Runner: `runner/provider_registry.go:202 providerKeyFromModel`, `runner/interactive_service.go:8995 defaultModelForProvider`, `runner/runner.go:937 resolvePromptExecutionAdapter`, `runner/yolo_resolver.go:26 YoloPosture`, `runner/claude_mcp_server.go:19`, `runner/agent_orchestrator.go:236 spawnChildRun`, `runner/summarizer.go:31 summarizerModelFor`, `runner/handoff_context.go:142 supportsHandoffSource`, `runner/grok_transcript_loader.go:208 isGrokRealSessionID` / `runner/session_file_locator.go:19 LocateSessionFile`
- Desktop: `desktop/types/contract.ts:11 ProviderKey`, `desktop/state/store.ts:187 pickDefaultModel`, `desktop/components/ChatInput.tsx:71 PROVIDER_CARDS, 83 VISION_PROVIDERS, 94 FALLBACK_REASONING`, `desktop/components/AgentsPanel.tsx:415 providerOverride chips`, `desktop/components/ProviderAccountsPanel.tsx:7 PROVIDERS`, `desktop/components/settings/AiProvidersSettings.tsx:327 <option>`, `desktop/state/store.ts:2256 providerLabel`

## 1. Goal

Opencode Chat and Flow runs select `model`/`reasoning`/`yolo`/`posture` per turn like any other provider, render approval/question/tool/file cards identically via the shared `ProviderEvent` stream, and pass the full `E2E-01..27` parity matrix with no regression to Codex/Claude/Grok/Gemini.

## 2. Parent Links

- coding plan: `CP-57` Work Breakdown `P-5`..`P-11` (Key Decisions `P-5`..`P-11`)
- tech design: `SD-06`, `SD-11`, `SD-16`, `SD-14`
- system spec: `SS-05`, `SS-11`, `SS-12`
- specific upstream ids: `CP-57 Work Breakdown P-5` (permission/YOLO), `P-6` (ask_user/spawn), `P-8` (UI), `P-9` (resume), `P-10` (skill/context/gates/summaries)

## 3. Trigger

Settings now shows Opencode and the adapter can stream, but Chat/Flow still cannot pin Opencode model/reasoning/yolo per turn nor show grouped approval/question cards — DOD for user-visible parity is open.

## 4. Exact Change

- `T-1` **Model routing + default model (desktop+runner) — exclusive to this task:**
  - Runner `apps/local-runner/internal/runner/provider_registry.go:202` already appended `opencode/`+`opencode-go/` in Task-301; verify `providerKeyFromModel("opencode/gpt-5.6-terra")=="opencode"` and `providerKeyFromModel("opencode-go/gpt-5.6-luna")=="opencode"` + `providerKeyFromModel("grok-4.5")` still `"grok"` (regression guard `TestProviderKeyFromModelBaseRegressionPlusOpencode`).
  - Runner `apps/local-runner/internal/runner/interactive_service.go:8995 defaultModelForProvider("opencode")` → `"opencode/muse-spark-1.2-contributor-free"` (verified working smoke model) else `supportedModels` first `opencode/*`; `apps/desktop-flowpilot/src/state/store.ts:187 pickDefaultModel(provider==="opencode")` same heuristic → returns first enabled `opencode` model from `ai_supported_models`. **Exclusive to this task** — Task-301/302 must not touch `defaultModelForProvider`.
  - Desktop `packages/flowpilot-client-core/src/domain/adminLogic.ts:3 resolveProviderKeyForModel` already appended `opencode/` prefix in Task-302; add fallback `if (modelId.startsWith("opencode-")) return "opencode"` for bare ids if any.
  - Runner `apps/local-runner/internal/runner/runner.go:937 resolvePromptExecutionAdapter` one-shot `case "opencode"` → `opencode run --format json --model opencode/... --variant high --auto` + `usesPromptArg=true` **exclusive to this task** (Task-301 must not); wire `summarizerModelFor("opencode")` in `apps/local-runner/internal/runner/summarizer.go:31` → `"opencode/gpt-5.4-nano"` cheap tier.
- `T-2` **Reasoning `→ --variant` (runner+desktop):**
  - Runner `apps/local-runner/internal/runner/opencode_reasoning.go` (`opencodeReasoningVariantID(canonical) (string,bool)` mapping `low→low, medium→medium, high→high, xhigh→max, max→max, minimal→minimal, ultra→max`); `provider_registry.go` Opencode factory `grokEffort`-like: mapped `variant` becomes part of `ensureOpencodeProcess` key so changing effort respawns process; empty `→` omit flag.
  - Desktop `apps/desktop-flowpilot/src/components/ChatInput.tsx:94 FALLBACK_REASONING_OPTIONS` keep as fallback; `reasoningOptionsFor(model)` already data-driven from `SupportedModel.supportedReasoningEfforts` (populated via `detectOpencodeModels` → `ai_supported_models` `supported_reasoning_efforts`); add `ultra` label `REASONING_EFFORT_LABELS` if opencode ever reports it; `apps/desktop-flowpilot/src/components/ChatPosturePanel.tsx:206-210` static `<option>` list already includes `xhigh/max` → add `ultra` option; `WorkflowsSettings.tsx:123 REASONING_OPTIONS` sync.
- `T-3` **YOLO + posture (runner) — exclusive wiring in this task:**
  - Runner `apps/local-runner/internal/runner/yolo_resolver.go:26` already added `OpencodePermissionMode` field in Task-301; **this task** wires `resolveYoloPosture(req.YoloMode, req.ForceShellBridge, req.ChatPosture)` → `opencodeAutoApprove` + `opencodeApprovalMode` (`--auto` vs gated `permission` array); `req.ChatPosture scan/plan` forces `RunnerAutoApprove=false` + gated mode (read-only policy via `chat_posture.go:254` already).
  - Desktop `apps/desktop-flowpilot/src/state/store.ts:1091 toggleYoloForActiveProvider` — keep Opencode on sync branch (`if(selectedProvider!=="grok")` sync); do not add new async endpoint unless live probe proves config rewrite needed; `ChatInput.tsx:994 yolo-toggle` delegates to store (no provider fork).
- `T-4` **Permission channel → `bridge.RequestApproval` (runner):**
  - In `apps/local-runner/internal/runner/opencode_adapter.go` `handleInbound(req Method="question"|"permission"|"session/request_permission")` route to `bridge.RequestApproval(ApprovalDetails{Command, Cwd, Reason, Kind, Decisions:[]ApprovalDecisionOption{{Value:"approve",Label:"Approve"},{Value:"deny",Label:"Deny"}}})` where `Kind=exec` for shell `command` present, `file` for `applyPatch`-like, `mcp` for `mcpServer/elicitation`, `other` otherwise (mirror `codex_adapter.go:471 codexApprovalKind`); handle `deny`→`denied` mapping; expiry/interrupt → deny so engine never hangs (like Grok `grok_adapter.go:694`).
  - Ensure `Kind=="exec"` for shell so the desktop's `"don't ask again"` allowlist (`EngineSettings` per `BUG-246`) engages; derive via `opencodeApprovalKind(method)`.
- `T-5` **MCP readiness gate + `ask_user`/`spawn_agent` via ACP `mcpServers` (runner):**
  - `opencode_adapter.go` `SendTurn` register `bridge` on `claudeMCP` under `token`, build `mcpServers = append(grok/hosted flowpilot entries, grokACPExtraMCPServers(extraMCPServers(yolo)))` via shared `flowpilotClaudeExtraMCPServers(home, yolo)` + `writeClaudeMcpConfig`-like helper if Opencode needs file; withhold `session/prompt` until `claudeMCP.waitReady(ctx, token, claudeMCPReadyDefaultTimeout)` (mirrors `claude_adapter.go:179` / `grok_adapter.go:299`).
  - Expose FlowPilot tools as MCP `flowpilot` tools on server `flowpilot`: `ask_user(prompt,options[],multiSelect?)`, `spawn_agent(agent,prompt,provider,wait?)`, `submit_review_outcome`; route `ask_user` → `bridge.AskQuestion` (like `codex_adapter.go:351` / `grok_adapter.go:577`), `spawn_agent` → `bridge.SpawnAgent` (like `codex_adapter.go:360` `parseSpawnAgentInput`); support both `ask_user` and native `question` permission shim — document choice.
  - Spawn inherits `providerKey/mode/reasoningEffort/yolo` from parent via `defaultModelForProvider` when `in.Provider==""`; normalize tool name collision `flowpilot_spawn_agent` like Codex `codexSpawnAgentToolName` if Opencode reserves `spawn_agent`.
- `T-6` **Skill/context/flow-gates (runner):**
  - Verify `promptPrep` in `provider_registry.go` Opencode factory does `r.injectSelectedSkills(workspace, req.Prompt, req.SelectedSkills) + opencodeToolReinforcements` (mirror Grok `+grokToolReinforcements`); no Opencode-specific formatter.
  - Ensure `interactive_service.go` `finishTurn` → `runFlowGate` path reaches Opencode turns (like `GR-10`); gate-repair prompt reuses Opencode adapter (keep providerKey pinned).
  - Ensure summaries for Opencode chats via `summarizerModelFor("opencode")` reuse shared `SummarizeChatTranscript`.
- `T-7` **Resume/history/handoff (runner) — exclusive to this task:**
  - `ProviderSessionStore.UpsertSession` already in Task-301; **this task** wires cross-run `opencodeRunSessionIndex` if shared process like Grok `grokRunSessionIndex` (`opencode_adapter.go:85` — `Key()` is `:83`) to map `RunID→sessionId` across `model/variant/auto` respawns; `session_file_locator.go:19 LocateSessionFile` Opencode branch (+ `isOpencodeRealSessionID` analog to `grok_transcript_loader.go:208 isGrokRealSessionID`) for Drive restore; keep synthetic `thread-*` fail. **Exclusive** — Task-301 must not add `LocateSessionFile`.
  - `handoff_context.go:142 supportsHandoffSource` add `opencode` case only after `opencode_transcript_loader.go` proven (keep `false` in this task, note dependency).
- `T-8` **Desktop Chat/Flow UI parity (desktop):**
  - `types/contract.ts:11` already `| "opencode"` in Task-302; `adminModels.ts:89` same; `ChatInput.tsx:71` `PROVIDER_CARDS` add `{value:"opencode", label:"OpenCode", icon:<OpencodeIcon/>}` + `styles.css` `--opencode-brand` + `.provider-chip-opencode` + `.pill-prov.prov-opencode`; `ChatInput.tsx:83` `VISION_PROVIDERS` keep `opencode` **excluded** (stay `false` until image proven, else add); `ChatInput.tsx:384` `selectedModelInfo` + `374 availableModels` already data-driven once catalog populated.
  - `ProviderAccountsPanel.tsx:7` already `| "opencode"` in Task-302; verify `compactUsageLines` stays generic (only `gemini` filters `3.1`).
  - `AgentsPanel.tsx:30 resolveMainAgentDisplay` add `opencode-` prefix branch → `"opencode"`; `415 providerOverride chips` add `opencode` chip (5th); `379 isClaudeSource` → add `isOpencodeSource = source==="opencode"||path.includes(".opencode")` and card `prov-opencode` class; `store.ts:2256 providerLabel` already `"OpenCode"` in Task-302; `ChatInput.tsx:83 VISION` decision mirrored.
  - `AiProvidersSettings.tsx:327` `<option value="opencode">` already in Task-302.
  - TUI `tui/app/helpers.go:1441 modelsForProvider` + `chat_posture.go` already data-driven; `tui/app/app.go:743 ProviderInstallMsg` + `746` status string already generic; ensure `tui/app/picker_model_reasoning_test.go` adds opencode `variant` list.
- `T-9` **Token/context + image gating:**
  - Map ACP `turn_completed` tokens/`cost` → `EventTokenUsageUpdated` (Task-301 mapper does this; `step_finish.tokens` is one-shot contrast only); UI `ChatInput.tsx:144 usageSummaryLine(provider, usage, fallbackContextWindow=selectedModelInfo.contextWindowTokens, activeAccount)` already provider-agnostic — auto-works when Opencode emits `ModelContextWindow` from `initialize` or catalog `selectedModelInfo.contextWindowTokens` fallback (`Task-215` wiring).
  - Image gating: `VISION_PROVIDERS` excluded keeps ChatInput attach disabled; if later `Vision=true`, implement full Task-052 payload `PromptAttachment{Data base64}` → ACP blocks and `image.go` thumbnail — out of scope here, keep `false`.

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/provider_registry.go:202` (`providerKeyFromModel` + `opencodeReasoningVariantID` wiring + respawn-key integration; `defaultModelForProvider:8995` + `resolvePromptExecutionAdapter:937` are **this task exclusive**), `apps/local-runner/internal/runner/opencode_adapter.go` (`handleInbound` permission/MCP wiring + `opencodeReasoningVariantID` + `opencodeApprovalKind` + MCP `waitReady` gate), `apps/local-runner/internal/runner/opencode_process.go` (respawn key includes `variant+auto`), `apps/local-runner/internal/runner/opencode_event_mapper.go` (permission→`permission_required` plus `Ask` shim), `apps/local-runner/internal/runner/yolo_resolver.go:26` (`OpencodePermissionMode` wiring), `apps/local-runner/internal/runner/summarizer.go:31` (`summarizerModelFor` opencode), `apps/local-runner/internal/runner/handoff_context.go:142` (keep `supportsHandoffSource` false), `apps/local-runner/internal/runner/session_file_locator.go:19` (Opencode `LocateSessionFile` + `grok_transcript_loader.go:208` — **this task exclusive**), `apps/local-runner/internal/runner/interactive_service.go:8995` (`defaultModelForProvider` opencode), `apps/local-runner/internal/runner/runner.go:937` (`resolvePromptExecutionAdapter` opencode one-shot), `apps/desktop-flowpilot/src/types/contract.ts:11`, `apps/desktop-flowpilot/src/state/store.ts:187` (`pickDefaultModel` opencode branch + `providerLabel:2256`), `apps/desktop-flowpilot/src/components/ChatInput.tsx:71` (`PROVIDER_CARDS`, `OpencodeIcon`, `VISION_PROVIDERS:83`, `FALLBACK_REASONING:94`), `apps/desktop-flowpilot/src/components/AgentsPanel.tsx:415` (`resolveMainAgentDisplay`, chips, `isOpencodeSource`), `apps/desktop-flowpilot/src/styles.css` (`--opencode-brand`), `apps/desktop-flowpilot/src/client/MockRunnerClient.ts` (add opencode mock if missing)
- modules: provider model/reasoning/yolo mapping, inbound permission/MCP routing, spawn_agent orchestration, skill/context/flow-gate, resume/history/handoff, Chat UI `PROVIDER_CARDS` chips + agent panel
- routes: existing `POST /client/workflow-runs/.../turns`, `POST /client/approvals/{id}/decision`, `POST /client/questions/{id}/answer`, `POST /client/workflow-runs/{id}/spawn-agent`, `GET /providers` (no new route unless Opencode needs async YOLO endpoint)
- tables: `ai_supported_models` (read only in this task; writes in Task-302), `workflow_provider_sessions` (via `ProviderSessionStore`)

## 6. Acceptance Check

- Chat `model` dropdown groups `opencode/*` + `opencode-go/*` under `OpenCode`; picking `opencode/gpt-5.6-terra` pins `providerKey="opencode"` for the run and survives `workflow_step_auto` resolution.
- Chat `Reasoning` dropdown for an `opencode` model shows per-model `supportedReasoningEfforts` from `detectOpencodeModels` (e.g. `minimal/low/medium/high/max`) when catalog has them, else shows `FALLBACK_REASONING_OPTIONS`; switching model resets invalid effort (like Grok `ChatInput.tsx:389` effect).
- `YOLO off` → shell `rm -rf ./dist` in Opencode turn emits `permission_required` grouped card; `Deny` blocks action and produces controlled failure; `YOLO on` → same action auto-approves via runner policy while `ask_user` still surfaces `user_question_required` (not auto-approved).
- MCP readiness: with FlowPilot MCP `google-drive` configured, Opencode turn has both `flowpilot:ask_user` and `google-drive:*` visible in the same turn; `opencode mcp` lists them.
- `ask_user` → `QuestionCard` with `options[]` + `multiSelect` + `__google_drive_picker__`; answering resumes turn; `spawn_agent` `wait=true` blocks parent until child completes and shows in agent panel, `wait=false` fires non-blocking.
- Tool/file cards: `tool_started/tool_completed` from Opencode `tool_call` render via `TuiToolGroup` + `EventFileChanged` derived from diffs; grouped approvals do not wedge (parallel `ca195` queue).
- Skill/context/flow-gate: `SelectedSkills` multi-skill order preserved in Opencode prompt prep; feature history + audit + summary injected; `r-ca`/`r-bug`/`r-task` repair prompts run on Opencode (stay on same provider) and emit `flow_gate_violation` when violated.
- Token/context: ACP `turn_completed` tokens/`cost` maps to status bar `Context 12k / 128k used` via `usageSummaryLine` fallback `selectedModelInfo.contextWindowTokens` (`step_finish.tokens` is one-shot contrast); `Vision=false` keeps image attach chip disabled for Opencode.

### 6.1 Definition of Done (DOD)

- [ ] `DOD-1` `providerKeyFromModel` `opencode/`+`opencode-go/` → `ProviderKeyOpencode` proven and base regression for `gpt-/claude-/gemini-/grok-` holds. (`TestProviderKeyFromModelOpencodeBothNamespaces`.)
- [ ] `DOD-2` `defaultModelForProvider("opencode")` returns `opencode/muse-spark-1.2-contributor-free` else first enabled `opencode` model from registry. (`TestDefaultModelForProviderOpencode`.)
- [ ] `DOD-3` Desktop `pickDefaultModel` opencode branch returns an enabled `opencode` model, not `undefined`; switching `PROVIDER_CARDS` `opencode` → `grok` and back preserves defaults. (`store.pickDefaultModel` test.)
- [ ] `DOD-4` Reasoning dropdown for opencode model shows `supportedReasoningEfforts` from `detectOpencodeModels`; fallback works when catalog row has no `supportedReasoningEfforts`. (`ChatInput.reasoningOptionsFor` test.)
- [ ] `DOD-5` `resolvePromptExecutionAdapter` opencode one-shot branch produces `opencode run --format json --model opencode/... --variant high --auto` correctly; `summarizerModelFor("opencode")=="opencode/gpt-5.4-nano"`. (`TestResolvePromptExecutionAdapterOpencode`.)
- [ ] `DOD-6` YOLO: runner `YoloPosture` Opencode branch maps `yolo=false + chatPosture scan/plan` → gated `permission` (no `--auto`) and `yolo=true` → `--auto` + `RunnerAutoApprove=true`; `Kind=="exec"` for shell so allowlist engages. (`TestYoloResolverOpencode`.)
- [ ] `DOD-7` Inbound permission (`question`/`permission`) → `bridge.RequestApproval` with `ApprovalDetails.Kind` mapped and decision encoded back to Opencode's `optionId`; deny blocks, allow executes, expiry → deny. (`TestOpencodeInboundPermission` via fake dispatcher `session/request_permission` fixture.)
- [ ] `DOD-8` MCP `waitReady` gate: test simulates async `_x.ai/mcp/init_progress`-like notification for Opencode and asserts `session/prompt` is withheld until `waitReady` succeeds, else first turn drops FlowPilot tools. (`TestOpencodeMCPReadyBeforePrompt`.)
- [ ] `DOD-9` `ask_user` → `user_question_required` (`options`/`multiSelect` parsed like `claudeAskUserParams`) → desktop answer → turn continues. (`TestOpencodeAskUserViaMCP`.)
- [ ] `DOD-10` `spawn_agent` via MCP → `bridge.SpawnAgent` with `wait` true/false + inherited `provider/yolo/context` + normalized `flowpilot_spawn_agent` name; parent continues / blocks correctly. (`TestOpencodeSpawnAgentBothWaits`.)
- [ ] `DOD-11` Skill injection exact order for opencode `SelectedSkills` multi-skill. (`TestOpencodeSkillInjection`.)
- [ ] `DOD-12` Flow gates `r-ca`/`r-bug`/`r-task` via `finishTurn` on Opencode turns; repair prompt stays on Opencode provider. (`TestOpencodeFlowGates` with `runFlowGate`.)
- [ ] `DOD-13` Token/context: ACP `turn_completed` tokens/`cost` → `EventTokenUsageUpdated` and status bar fallback `selectedModelInfo.contextWindowTokens`. (`TestOpencodeTokenUsageReporting`.)
- [ ] `DOD-14` Desktop UI parity: `PROVIDER_CARDS` includes `opencode` with icon, `AgentsPanel` `providerOverride` chips include `opencode`, `isOpencodeSource` badge renders `prov-opencode`, `ProviderAccountsPanel` already has opencode (Task-302) and does not regress. (Snapshot/screenshot test `AgentPanel Opencode chips`.)
- [ ] `DOD-15` Vision stays `false` until proven; `VISION_PROVIDERS` excludes `opencode` in this task (future `opencode/skills` add path fallback like Grok `.tmp/images` if `image` ever reported). (`TestOpencodeCapabilitiesVisionFalse`.)
- [ ] `DOD-16` Base-regression: `providerSpecs` order unchanged (`codex,claude,gemini,grok,opencode` appended at `runner.go:1669`), `providerKeyFromModel` / `providerSpecs` / `provider_registry` unit tests green for old providers (not `domain_hardcode_guard_test.go:16`).
- [ ] `DOD-17` Manual parity: one real desktop Opencode run covering chat with `opencode/gpt-5.6-terra` + `reasoning high` + `yolo off` approval deny + `ask_user` + spawned child + `Gen summary` + flow gate `r-ca` + resume after restart (recorded).

### 6.2 Test Signatures

```go
// runner/provider_registry_test.go
func TestProviderKeyFromModelOpencodeBothNamespaces(t *testing.T)
func TestDefaultModelForProviderOpencode(t *testing.T)
func TestResolvePromptExecutionAdapterOpencode(t *testing.T)

// runner/yolo_resolver_test.go
func TestYoloResolverOpencode(t *testing.T)
func TestYoloResolverOpencodeForceShellBridgeAndReadOnlyPosture(t *testing.T)

// runner/opencode_reasoning_test.go
func TestOpencodeReasoningVariantID(t *testing.T)

// runner/opencode_adapter_test.go
func TestOpencodeInboundPermission(t *testing.T)
func TestOpencodeMCPReadyBeforePrompt(t *testing.T)
func TestOpencodeAskUserViaMCP(t *testing.T)
func TestOpencodeSpawnAgentBothWaits(t *testing.T)
func TestOpencodeSkillInjection(t *testing.T)
func TestOpencodeFlowGatesViaFinishTurn(t *testing.T)
func TestOpencodeTokenUsageReporting(t *testing.T)
func TestOpencodeCapabilitiesVisionFalse(t *testing.T)
```

```ts
// desktop ChatInput.test.tsx / store.test.ts (TS)
it('picks default model for opencode', async () => {})
it('shows reasoning options for opencode model', async () => {})
it('includes opencode in provider cards', async () => {})
```

## 7. Out of Scope

- Live `session/request_permission` **full** `file/mcp` permission matrix beyond `exec` allowlist — keep `file`/`mcp` as `warn` in this task if not proven; hardening is follow-up.
- Handoff `supportsHandoffSource opencode=true` and `LocateSessionFile` **directory-per-session** restore beyond single-file smoke — directory handling is parity hardening after MVP; **single-file** `LocateSessionFile` branch is in scope (T-7).
- Pruning stale `opencode/deepseek-v4-flash-free` row or vision image-block attachments — covered in Task-302 / future vision task.
- Refactoring `gemini_acp_transport.go` / `grok_acp.go` — still disallowed (`P-0`).

## 8. Completion Notes

- result:
- implementation notes: `provider_registry.go` `providerKeyFromModel` both prefixes + `defaultModelForProvider` + `resolvePromptExecutionAdapter` one-shot, `yolo_resolver.go` `OpencodePermissionMode`, `opencode_adapter.go` `handleInbound` permission kind + `waitReady` gate + `ask_user`/`spawn_agent` MCP wiring, `desktop/ChatInput.tsx` `PROVIDER_CARDS`/`OpencodeIcon`/`VISION_PROVIDERS`, `AggregatedPanel` `providerOverride` chips, `store.ts` `pickDefaultModel` + `providerLabel`.
- verification:
- follow-ups: image `Vision=true` only after ACP `promptCapabilities.image` proven; process respawn key per `model+variant+auto` like Grok `grokProcesses map` if Opencode treats them as launch flags (live probe during Task-301 will decide).
- upstream docs updated: `CP-57` Work Breakdown `P-5`..`P-11`, rows `OC-04`..`OC-17`, `OC-35`
