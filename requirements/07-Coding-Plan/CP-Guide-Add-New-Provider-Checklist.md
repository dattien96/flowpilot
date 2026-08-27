# CP-Guide: Add New AI Provider Checklist (template for any provider)

## Metadata

- Document ID: `CP-Guide-Add-New-Provider`
- Title: `Add New AI Provider Checklist — template (CP-57 perfect example)`
- Phase: `coding_plan`
- Status: `approved`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-27`
- Last Updated: `2026-08-27`
- Parent Documents: [CP-57: Opencode Provider Integration](./todo/CP-57-Opencode-Provider-Integration.md)
- Child Documents: [Task-300: Opencode ACP Transport And Process/Dispatcher](../08-Task/todo/Task-300-Opencode-ACP-Transport-And-Process-Dispatcher.md), [Task-301: Opencode Controlled Adapter MVP](../08-Task/todo/Task-301-Opencode-Controlled-Adapter-MVP.md), [Task-302: Opencode Settings — Detect/Install/Models/MCP/Account](../08-Task/todo/Task-302-Opencode-Settings-Detect-Install-Models-MCP-Account.md), [Task-303: Opencode Chat/Flow — Model/Reasoning/YOLO/Cards/Tools](../08-Task/todo/Task-303-Opencode-Chat-Flow-Model-Reasoning-YOLO-Cards-Tools.md)
- Related Documents: [Skill: add-new-provider](../../.agents/skills/add-new-provider/SKILL.md)
- Replaces: `None`
- Tags: `ai-providers, checklist, acp, template`
- Feature Keys: `ai-providers`

## AI Quick View

### Summary

- This guide is the **mentionable** checklist for adding any new provider — copy CP-57's proven split `Task-300..303` and its 28-row mirror. Mention `CP-Guide-Add-New-Provider` or `add-new-provider` and the agent must enumerate all rows + produce the 4 Tasks without asking "already planned a/b/c?".
- Covers the full stack: ACP transport freeze → adapter/MVP → Settings (detect/install/models/MCP/account) → Chat/Flow parity (model/reasoning/YOLO/cards/tools) with zero base regression.
- Resume/cross-account "continue chat" is via `ProviderSessionStore` copy-chat (real `ses_*` + `thread-*` fail + `LocateSessionFile` + Drive `BuildChatSessionSyncManifest`), same as Codex/Claude/Grok.

### Current Ask

- Provide a single CP-doc counterpart to `.agents/skills/add-new-provider/SKILL.md` so planning can reference a versioned doc, not just a skill file.

### Key Decisions

- `P-0` PLUGIN-ONLY / ZERO BASE REGRESSION — appended `case "x"` last only (`CP-57:35`).
- `P-1` ACP `X acp` for interactive, `X run --format json` summarizer-only (`CP-57:36`).
- `P-2` Check the 28-row table below — every row is `append last` per `CP-46 P-0`.

### Constraints

- Keep `gemini_acp_transport.go` / `grok_acp.go` byte-identical.
- Capability flags stay `false` until proven live.

### Open Questions

- None — gaps are `Q-1..Q-6` in CP-57.

## 0. Non-negotiable contracts (P-0)

- **PLUGIN-ONLY / ZERO BASE REGRESSION.** Every shared file gets an appended `case "newProvider"` last, no reordering, no signature change, no mutation of `gemini_acp_transport.go`/`grok_acp.go`/`codex_adapter.go`/`claude_adapter.go`. Each touched file ships a regression test proving Codex/Claude/Gemini/Grok branches still byte-identical (`CP-57:35`, `CP-57:5.1`).
- **Capability-honest.** Flag stays `false` until a test or live `X acp` capture proves it (`P-12` `CP-57:47`).
- **Runner owns state.** Desktop only renders; all turns go `ProviderRuntimeAdapter.SendTurn` → `ProviderEvent` → `finishTurn/agent_orchestrator`.

## 1. Freeze ACP contract live (Task-300 = P-1/P-2)

- Capture `initialize` / `session/new{cwd,mcpServers,permission,model,variant,auto}` / `session/prompt` / `session/update{agent_message_chunk/tool_call/tool_call_update}` / `session/request_permission` (question/plan_enter/plan_exit) / `session/cancel` / `turn_completed{stopReason,_meta.tokens/cost,_meta.sessionId}` from a **live `X acp`  session** (golden fixtures `testdata/X_acp/*.json`). `X run --format json` is one-shot/summarizer only, never freeze as ACP (`CP-57:26,164-167`).
- Record `compatTestedVersion` and whether `model/variant/auto` are launch flags vs per-session params (decides `newAdapter` vs `newAdapterForTurn` `provider_registry.go:104-119`).

## 2. Transport + process/dispatcher (Task-300 = P-3/P-4)

- Add `X_acp.go` + `X_process.go` **copied** from `grok_process.go`/`codex_appserver.go` — one persistent `X acp` stdio process per `scopeKey` (account), `dispatcher{waiters, sessionSubs, single inbound handler, readLoop, fail() drain}`.
- `ensureXProcess(ctx, scopeKey, cwd, env, model, variant, auto)` + `XProcesses map[string]*XProcessHandle` on `*Runner` (`runner.go:155-175`), env isolation `HOME/XDG_CONFIG_HOME/X_HOME/X_CONFIG` + Windows `USERPROFILE/APPDATA` + strip host secrets, `redactXFrameForLog`.

## 3. Adapter MVP (Task-301 = P-2/P-3/P-6/P-12)

- `provider_event.go:10` `ProviderKeyX="x"` + `provider_registry.go:202` `providerKeyFromModel` append `prefix/` last + `X_adapter.go` `Key()==ProviderKeyX`, `Capabilities(){Streaming:true,Resume:true,FileEvents:true,Interrupt:true,SkillSelection:true, ApprovalEvents:false,Mcp:false,Vision:false}` honest, `SendTurn` honoring `Cwd/ModelName/ReasoningEffort->variant/SelectedSkills/ChatPosture/Attachments/ProviderSessionID` + MCP `waitReady` gate + persist real `ses_*` via `ProviderSessionStore.UpsertSession{ProviderKey:"x"}` + synthetic `thread-*` explicit fail + `XEventMapper`.
- `XReasoningVariantID()` mapper, `yolo_resolver.go` append `XPermissionMode`, gate `FLOWPILOT_X_AGENT` + `DefaultProviderRegistry` **without** x, live `ProviderRegistryFor` only.

## 4. Settings — Detect/Install/Models/MCP/Account (Task-302 = P-7/P-11)

- `runner.go:1669 providerSpecs() add {Key:"x",BinaryName:"x"}` + `tooling/check.go:33 CheckTool("x")` + `compat.go:20` + `CheckVersionSettings.tsx:219` 4th row.
- `detectXModels()` via `X models` → `ai_supported_models provider_key='x'` 40+ rows `source:"detected"`.
- `X_mcp_provider_config.go` writes `~/.config/x/x.json mcpServers` atomically, `OPENCODE_CONFIG` per `.xHomeN`.
- `provider_accounts.go:640 managedProviderHomePrefix ".xHome"` + `DiscoverXAccountHomes` + `isValidXAccountPath` + `StartInteractiveAuth` x → `x auth login`, account `loadXAccountMetadata` from `x providers`+`x stats`.

## 5. Full 28-row mirror (CP-57:5.2 executable DOD)

Every `switch providerKey` / `ProviderKey*` must gain `x` appended last:

| # | Area | Mirror for new provider |
|---|---|---|
| 1 | ProviderKey + Capabilities | `ProviderKeyX` + Streaming/Resume/ApprovalEvents/FileEvents/SkillSelection/Mcp/Interrupt/Vision:false |
| 2 | ProviderRegistry / DefaultProviderRegistry / Adapter(model,effort) | Register in `ProviderRegistryFor` only, freeze `newAdapter` vs `newAdapterForTurn` after live probe |
| 3 | providerKeyFromModel + defaultModelForProvider | Append `x/` + `x-go/` prefix |
| 4 | resolvePromptExecutionAdapter (one-shot summarizer) | Add `x` branch `X run --format json` |
| 5 | Detection/Install/Inventory | providerSpecs + detectProvider `x --version` |
| 6 | InstallProvider | x branch `npm i -g ...` |
| 7 | Live model catalog probe | detectXModels via `X models` |
| 8 | Managed homes / NextAccountHomePath / Discover | `.xHome` + isValidXAccountPath + PROVIDERS |
| 9 | Auth file check | x auth.json-like shape |
| 10 | Env isolation | X_HOME/X_CONFIG/HOME mapping |
| 11 | Connect/Verify/Activate + terminal auth | StartInteractiveAuth x → `x auth login` |
| 12 | Reasoning vocab & per-turn | XReasoningVariantID -> --variant |
| 13 | YOLO / YoloPosture SSOT | Add XPermissionMode appended field |
| 14 | Chat posture scan/plan/code | No branch — adapter forces untrusted/gated |
| 15 | SkillSelection injection | Add `.x/skills` root |
| 16 | Runner-hosted MCP | X_mcp_provider_config writes `~/.config/x/x.json mcpServers`; session/new{mcpServers:[flowpilot,google-drive,jira]} |
| 17 | SpawnAgent / cohort | x chips + isXSource badge + prov-x CSS |
| 18 | LocateSessionFile + is*RealSessionID | Add isXRealSessionID + branch, reject thread-* |
| 19 | Session store UpsertSession | XAdapter.recordSession + LastXSessionID |
| 20 | History replay transcriptTurn | X_transcript_loader (supportsHandoffSource stays false) |
| 21 | Cross-account resume / Drive sync | Handle directory vs single-file copy (BUG-316 analog) — see §6 |
| 22 | Summarizer | Add x→x/<cheap-nano> via summarizerModelFor only |
| 23 | Token usage | Map turn_completed tokens/cost → EventTokenUsageUpdated |
| 24 | ProviderModel.ContextWindowTokens | Populate if `X models --json` exposes |
| 25 | Compat CompatTested*Version | Add CompatTestedXVersion + 4th row probe |
| 26 | Agent catalog | Add `.x/agents` + isXSource badge |
| 27 | TUI picker | Add XIcon + card + VISION_PROVIDERS decision |
| 28 | Image attachment gating | Vision:false initially |

## 6. Resume / cross-account continue-chat (copy-chat)

- Real `ses_*` only, persisted via `ProviderSessionStore`, resumed via `session/load{sessionId,cwd,mcpServers}` (probe `Task-300 T-2b` whether portable across `X_CONFIG` accounts — `Q-1` `CP-57:59`). Synthetic `thread-*` explicit fail.
- Task-301: `UpsertSession` + `XRunSessionIndex` like `grokRunSessionIndex` (`grok_adapter.go:85`).
- Task-303: `LocateSessionFile` x branch + `isXRealSessionID` (`grok_transcript_loader.go:208`) for Drive restore/cross-account copy; `handoff_context.go:142 supportsHandoffSource` stays `false` until extractor proven; handle directory vs single-file sidecar copy (`chat_session_sync.go:313 BuildChatSessionSyncManifest` / `427 resolveGrokSessionSidecarFiles` `interactive_resume.go:2584`).

## 7. Chat/Flow UI parity (Task-303 = P-8/P-9/P-10)

- `contract.ts:11| "x"`, `adminModels.ts SupportedModel.providerKey | "x"`, `ChatInput.tsx:71 PROVIDER_CARDS` + XIcon, `ProviderAccountsPanel PROVIDERS add {key:"x",label:"X"}`, `store.ts providerLabel/pickDefaultModel`, reasoning picker + `variant` list, `yolo` toggle sync, cards `permission_required/user_question_required/tool_started/file_changed` provider-neutral, `AgentsPanel` `prov-x` + `isXSource`.

## 8. Validation

- Golden fixtures `testdata/X_acp/*`, `go test ./internal/runner` for all `OC-*` parity + `OC-BR` base-regression, E2E `E2E-01..27` (`CP-57:345-374`), manual one real desktop run covering chat + approval + `ask_user` + spawned child + summary + flow gate + resume.

## 9. Task split to copy (CP-57 Work Breakdown §4)

- P-1 freeze ACP contract = Task-300, P-2 transport/process = Task-300, P-3 adapter = Task-301, P-4 event mapper = Task-301, P-5 permission/YOLO = Task-301 MVP stub + Task-303 full, P-6 ask_user/spawn_agent = Task-303, P-7 settings = Task-302, P-8 UI = Task-303, P-9 resume/handoff = Task-301+303, P-10 skill/context/gates = Task-303, P-11 tests/notes = Task-300..303.

> Mention this doc or `.agents/skills/add-new-provider/SKILL.md` and the agent must enumerate the 28 rows + produce the 4 Tasks.
