# Task-301: Opencode Controlled Adapter MVP (Chat/Stream/Resume/Token)

## Metadata

- Document ID: `Task-301`
- Title: `Opencode Controlled Adapter MVP (Chat/Stream/Resume/Token)`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-27`
- Last Updated: `2026-08-27`
- Parent Documents: [CP-57: Opencode Provider Integration](../../07-Coding-Plan/todo/CP-57-Opencode-Provider-Integration.md), [Task-300: Opencode ACP Transport And Process/Dispatcher](./Task-300-Opencode-ACP-Transport-And-Process-Dispatcher.md)
- Child Documents: `None`
- Related Documents: [Task-207: Grok Controlled Adapter MVP](../done/Task-207-Grok-Controlled-Adapter-MVP.md), [Task-165: Gemini Controlled Adapter MVP](../done/Task-165-Gemini-Controlled-Adapter-MVP.md)
- Replaces: `None`
- Tags: `opencode, adapter, runner, provider-runtime, ai-providers`

## AI Quick View

### Summary

- Build the first `ProviderRuntimeAdapter` for Opencode (`opencodeAdapter`) on top of Task-300 transport/dispatcher.
- Support controlled normal chat: normalized `turn_started`/`message_delta`/`message_completed`/`tool_started`/`tool_completed`/`file_changed`/`turn_completed`/`turn_failed`, honoring every relevant `TurnRequest` field (`Cwd`, `ModelName`, `ReasoningEffort→variant`, `SelectedSkills`, `ChatPosture`, `Attachments`, `YoloMode→auto`).
- Persist the real `sessionId` (`ses_*`) and resume via it; register Opencode in live registry (`ProviderRegistryFor` only, `DefaultProviderRegistry` stays without opencode like Grok) with only conservatively-proven `ProviderCapabilities`; map ACP `turn_completed` tokens/`cost` → `EventTokenUsageUpdated` including `ModelContextWindow` when available (`step_finish.tokens` is one-shot `run --format json` contrast only).

### Current Ask

- Ship an Opencode adapter that can run a real desktop chat turn end-to-end (prompt in, streamed text + tool/file events out, `turn_completed`), with resume by real session id and token usage — without yet claiming full approval-gating breadth, full MCP, or agent-spawn (those extend in Task-303, but MVP must not hide the inbound permission path).

### Key Decisions

- `T-1` MVP may advertise `Streaming`, `Resume`, `FileEvents`, `Interrupt`, `SkillSelection` once proven; `ApprovalEvents` and `Mcp` stay `false` until Task-303 proves permission/MCP round-trip (mirrors Task-165 staged pattern) — keep `Vision=false`.
- `T-2` `clientCapabilities.fs.{read,write}TextFile=false` on `initialize` so Opencode uses its own tools; `FileEvents` derived from `tool_call` diffs, not client fs handler (CP-57 `P-6`).
- `T-3` Resume must use the real ACP-issued `sessionId` (`ses_*`) from `session/new`/`turn_completed` results (via `_meta.sessionId` fallback like Grok `Task-207 DOD-6`) — never synthetic `thread-*` — matching Claude/Codex/Grok real-session discipline. `step_finish` is one-shot `run --format json`, not ACP.
- `T-4` `SendTurn` must always emit `turn_completed` or `turn_failed` (leave `ProviderTurnID` empty; core stamps it) and persist new `sessionId` from `_meta` fallback.
- `T-5` Model/effort/skills/attachments from `TurnRequest` must be honored: `ModelName→opencode/<model>` param, `ReasoningEffort→variant` via `opencodeReasoningVariantID()`, `SelectedSkills`→shared `promptPrep`.
- `T-6` Token/context: map ACP `turn_completed` tokens/`cost` (`total/input/output/reasoning/cache` + `cost` from `turn_completed._meta`) → `EventTokenUsageUpdated` + `ModelContextWindow` from `initialize` if exposed; `step_finish.tokens` is one-shot contrast only.

### Constraints

- **PLUGIN-ONLY / ZERO BASE REGRESSION (CP-57 `P-0`):** additive-only; `gemini_acp_transport.go`/`grok_acp.go`/`codex_adapter.go`/`claude_adapter.go` untouched; shared switched-functions (`providerKeyFromModel` only in this task; `defaultModelForProvider` + `resolvePromptExecutionAdapter` one-shot belong to **Task-303**) get an appended `opencode` case only when owned.
- No Opencode-only desktop transport; reachable only through `/client/workflow-runs/.../turns`.
- Prompt assembly uses shared `injectSelectedSkills` path.
- Do not wire full `spawn_agent` in this MVP — inbound permission during MVP tests may be answered with a fixed generic "allow" double, not real policy (full policy in Task-303).
- Do not change Codex/Claude/Gemini/Grok behavior.

### Open Questions

- Whether resume is safe across `OPENCODE_CONFIG` changes (deferred to Task-303/Parity hardening).
- Whether `tool_call` thought chunks should map to visible `thinking` event or stay internal pending UX guidance.

### Source Refs

- `CP-57` Work Breakdown `P-3`, `P-4` (Key Decisions `P-2`, `P-3`, `P-4`, `P-6`, `P-12`); parity rows `OC-01`, `OC-02`, `OC-03`, `OC-11`, `OC-12`, `OC-15`, `OC-16`, `OC-24`, `OC-27`, `OC-35`.
- `Task-300` (transport/dispatcher + ACP launch-flag vs session-param probe for `newAdapter` vs `newAdapterForTurn`).
- `apps/local-runner/internal/runner/codex_adapter.go`, `grok_adapter.go:83 Key()` / `grok_adapter.go:85 grokRunSessionIndex`, `claude_adapter.go:71` (adapter template), `provider_event.go:10`, `provider_registry.go:202` (`providerKeyFromModel`), `provider_registry.go:104-119` (`newAdapter` vs `newAdapterForTurn`)

## 1. Goal

Let Opencode run a controlled desktop chat turn through `ProviderRuntimeAdapter.SendTurn`, emitting normalized `ProviderEvent`s, with real-session-id resume and token/context reporting, registered in the live provider registry with an honest minimal capability set.

## 2. Parent Links

- coding plan: `CP-57` Work Breakdown `P-3`, `P-4` (Key Decisions `P-2`, `P-3`, `P-4`, `P-6`, `P-12`)
- tech design: `SD-06`, `SD-11`
- system spec: `SS-05`, `SS-11`
- specific upstream ids: `CP-57 Work Breakdown P-3` (adapter), `P-4` (event mapper)

## 3. Trigger

After Task-300 provides working ACP transport/dispatcher, the runner can add a real adapter without duplicating JSON-RPC plumbing.

## 4. Exact Change

- `T-1` `apps/local-runner/internal/runner/opencode_adapter.go` (new) — **copy `grok_adapter.go:71 newGrokAdapter` + `grok_adapter.go:83 Key()` / `:230 Capabilities()` template**. Current: no `opencodeAdapter`; `provider_registry.go:88 type ProviderRuntimeAdapter interface { Key() ProviderKey; Capabilities() ProviderCapabilities; SendTurn(...) }` is the contract. Delta: implement `type opencodeAdapter struct { dispatcher *opencodeDispatcher; cwd string; bridges map[string]TurnBridge; allowReviewOutcome, yoloModes map[string]bool; mcpServer any; mcpBaseURL string; promptPrep func(TurnRequest)string }` copying `grok_adapter.go:71-81` fields; methods `Key() ProviderKey { return ProviderKeyOpencode }`, `Capabilities() ProviderCapabilities` (see T-8), `SendTurn(ctx context.Context, req TurnRequest, bridge TurnBridge) error` with signature exactly matching the interface (no extra params). Do NOT add desktop transport.
- `T-2` `apps/local-runner/internal/runner/provider_event.go:10` + `apps/local-runner/internal/runner/provider_registry.go:202` — Current: `provider_event.go:12 const ( ProviderKeyCodex="codex", ProviderKeyClaude="claude", ProviderKeyGemini="gemini", ProviderKeyGrok="grok" )` (`:10-18` grok appended last per CP-46 P-0); `provider_registry.go:198 func providerKeyFromModel(model string) (ProviderKey,bool)` switch has `case strings.HasPrefix(m,"gpt-")→Codex`, `gemini-`, `claude-`, `grok-`/`grok-build` (grok last). Delta: append `const ProviderKeyOpencode ProviderKey = "opencode"` at `provider_event.go:12` **after** grok; in `providerKeyFromModel` append `case strings.HasPrefix(m,"opencode/"), strings.HasPrefix(m,"opencode-go/"): return ProviderKeyOpencode,true` **appended last** (covers both xAi `opencode/gpt-5.6-terra` and Go `opencode-go/gpt-5.6-luna` plans, avoids `xai/grok-4.6` collision). Existing `gpt-/claude-/gemini-/grok-` cases byte-identical. Test: `TestProviderKeyFromModelBaseRegressionPlusOpencode` asserts `providerKeyFromModel("opencode/gpt-5.4-nano")==Opencode` and `("grok-4.5")==Grok` unchanged.
- `T-3` `apps/local-runner/internal/runner/opencode_event_mapper.go` (new) — **copy `grok_event_mapper.go:52 switch kind` + `grok_event_mapper.go:491 grokPromptResultTokenUsage`**. Current: `grok_event_mapper.go` maps `agent_message_chunk→message_delta`, `tool_call→tool_started`, `tool_call_update→tool_completed` + `EventFileChanged` from `path`+`newText` diff, `turn_completed{stopReason,_meta.tokens,cost,_meta.sessionId}→turn_completed/turn_failed` + `TokenUsageSnapshot`. Delta: implement `opencodeEventMapper` with `mapSessionUpdate(update map[string]any) ([]ProviderEvent,bool)` switching on Opencode `session/update` `kind` (`text`→`message_delta`/`message_completed`, `tool_call`→`tool_started`, `tool_call_update`→`tool_completed` + `EventFileChanged{Path: diff.path, OldText, NewText}` like Grok), filter `agent_thought_chunk` internal-only per Task-207 open question; implement `mapTurnCompleted(result map[string]any) ([]ProviderEvent,error)` reading `result["stopReason"]` → `turn_completed` vs `turn_failed`, `_meta.tokens:{total,input,output,reasoning,cachedReadTokens/cachedWriteTokens}` + `_meta.cost` → `EventTokenUsageUpdated{TokenUsage:&TokenUsageSnapshot{Last,Total,ModelContextWindow}}` where `ModelContextWindow` from `initialize.totalContextTokens` if exposed. Leave `ProviderTurnID` empty (core stamps). `step_finish` is one-shot `run --format json`, not ACP — do NOT map it. Copy `grokPromptResultTokenUsage` shape for opencode `turn_completed._meta` (Task-300 T-2b capture defines exact keys). Tests: `TestOpencodeEventMapperTextChunksToDelta`, `...DerivesFileChangedFromWriteDiff`, `...TurnCompletedToFailed`, `...TokenUsageToEvent`.
- `T-4` `apps/local-runner/internal/runner/opencode_adapter.go:SendTurn` — **copy `grok_adapter.go:SendTurn` pump pattern**. Current: `grok_adapter.SendTurn` does `ensureGrokProcess` → `session/new{cwd,mcpServers}` (or `session/load` resume via `_meta.sessionId`) → `session/prompt{sessionId,prompt:promptPrep(req)}`, pumps `sessionSubs[sessionId]` notification chan on own goroutine while `call(session/prompt)` blocks, `waitReady` gate for MCP (`claudeMCP.waitReady` / `grok_adapter.go:299 grokInitProgress`). Delta: implement `SendTurn` as `handle,err:=r.ensureOpencodeProcess(ctx,scopeKey,req.Cwd,env,req.ModelName,variant,auto)` → `sessionId,err:=opencodeACPSessionNew(cwd,mcpServers,permission,model,variant)` (or `opencodeACPSessionLoad(sessionId,cwd,mcpServers)` if `req.ProviderSessionID` is real `ses_*` from Task-300 T-2b) → `prompt:=r.injectSelectedSkills(cwd,req.Prompt,req.SelectedSkills)` via `a.promptPrep` wired in T-10 → `ch:=dispatcher.subscribe(sessionId)` → `waitReady` if `mcpServer!=nil` withholds `session/prompt` until `claudeMCP.waitReady(ctx,token,claudeMCPReadyDefaultTimeout)` (mirrors `claude_adapter.go:179`/`grok_adapter.go:299`), else first turn drops `ask_user`/`spawn_agent` (BUG-114 class) → `dispatcher.call(session/prompt)` + pump loop `for n:=range ch { mapSessionUpdate(n) → bridge.Emit(...) }` until terminal `turn_completed` drains, then `unsubscribe`. Handle `ctx.Done()→session/cancel` best-effort then `ctx.Err()`. Do NOT implement full `spawn_agent` — inbound permission stub in MVP returns generic `allow-once` (Task-303 does real policy).
- `T-5` `apps/local-runner/internal/runner/opencode_adapter.go:SendTurn` persist — **copy `grok_adapter.go:97 grokRunSessionIndex.remember` + `codex_adapter.go:UpsertSession`**. Current: Grok uses `grokRunSessionIndex struct { mu; byRun map[runID→sessionId]; bySession map[sessionId→runID] }` + `ProviderSessionStore.UpsertSession{ProviderKey:"grok",ProviderSessionID,ProviderThreadID}` after `session/new`; `claude_adapter.go:237 persistSession`. Delta: after `session/new` returns `sessionId:=opencodeACPResponseSessionID(result)` (handles `result["sessionId"]` or `result["_meta"].(map[string]any)["sessionId"]` like Grok bug Task-207 DOD-6), call `ProviderSessionStore.UpsertSession(ctx, ProviderSession{ProviderKey:ProviderKeyOpencode,ProviderSessionID:sessionId,ProviderThreadID:sessionId,RunID:req.RunID})` + `opencodeRunSessionIndex.remember(req.RunID,sessionId)` if shared process map like Grok (`grokRunSessionIndex` at `grok_adapter.go:85`); if later `turn_completed` result carries different `sessionId` under `result["_meta"]["sessionId"]` (prompt-result session id like Grok GR-32), adopt it via same `UpsertSession`+`remember`. Tests: `TestOpencodeAdapterSessionPersistenceAndResume`, `TestOpencodeAdoptsPostPromptSessionID`, `TestOpencodeSyntheticIdFails` (synthetic `thread-*` + no `byRun` map → explicit fail `err="no real session for thread-*"` not fresh `session/new`).
- `T-6` `apps/local-runner/internal/runner/opencode_reasoning.go` (new) + `provider_registry.go` factory — **copy `grokEffort` at `provider_registry.go:500-519` Task-220**. Current: `provider_registry.go:500 grokEffort` maps `low→low, medium→medium, high→high, xhigh→high/max, max→max` via `grokReasoningEffortID` (live-verified `initialize.reasoningEfforts`); `runner.go` grok launch flags `model/reasoningEffort/alwaysApprove`. Delta: add `func opencodeReasoningVariantID(effort string) (string,bool)` mapping `minimal→minimal, low→low, medium→medium, high→high, xhigh→max, max→max, ultra→max` (Opencode `--variant` values verified `opencode run --help --variant` in CP-57); unmapped `→ ("",false)` omit flag so model default applies (like Grok `grokEffort` fallback). Wire into `ProviderRegistryFor` factory closure at `provider_registry.go:455-524` grok block: after `scopeKey,env` resolve, compute `variant,ok:=opencodeReasoningVariantID(req.ReasoningEffort)` and pass `variant` to `ensureOpencodeProcess(ctx,scopeKey,cwd,env,req.ModelName,variant,auto)` as part of respawn key (`scopeKey+"|"+model+"|"+variant+"|"+fmt.Sprint(auto)`) like Grok `grokEffort:508` + `grokProcessKey` coexistence. If Task-300 T-2c proves Opencode caps are per-session not launch flags, keep single `opencodeProcess` per scopeKey and pass `variant` as `session/new` param instead — document choice.
- `T-7` `apps/local-runner/internal/runner/yolo_resolver.go:27-40` + `opencode_adapter.go:opencodeYolo` — Current: `yolo_resolver.go type YoloPosture struct { CodexSandbox, ClaudePermissionMode, GrokPermissionMode, RunnerAutoApprove bool }` plus `codex_adapter.go:621, grok_process.go:784 ApplyGrokYoloPosture` POST handlers; `Runner` has `grokDesiredAlwaysApprove bool` guarded `grokProcessMu`. Delta: append `OpencodePermissionMode string` **last** (`yolo_resolver.go:40` after `GrokPermissionMode`; codex/claude callers never read it → byte-identical for old providers). Add helper `func opencodeYoloPosture(req TurnRequest) (variantAuto bool, runnerAuto bool)` mapping `req.YoloMode→--auto` (`true→--auto` + `RunnerAutoApprove=true`, `false→no flag` + `false`), but `req.ChatPosture=="scan"||"plan"` forces `RunnerAutoApprove=false` + gated `permission:[{permission:"question",pattern:"*",action:"deny"}]` (read-only, like `chat_posture.go:12 scan/plan`). Add `req.ForceShellBridge` (V9-21): `ForceShellBridge true→permission mode stays gated for shell even when auto`. In MVP tests, inbound `session/request_permission` stub returns generic `allow-once` (Task-303 wires full `resolveYoloPosture` SSOT).
- `T-8` `apps/local-runner/internal/runner/opencode_adapter.go:Capabilities()` — **copy `grok_adapter.go:230 Capabilities` gated pattern**. Current: `grok_adapter.go:230 Capabilities(){ return ProviderCapabilities{ Streaming:true, Resume:true, FileEvents:true, Interrupt:true, ApprovalEvents: mcpServer!=nil && ... , SkillSelection:true, Mcp: mcpServer!=nil, Vision:false } }` (CP-46 P-11/MCP conditional). Delta: implement `func (a *opencodeAdapter) Capabilities() ProviderCapabilities { return ProviderCapabilities{ Streaming:true, Resume:true, FileEvents:true, Interrupt:true, SkillSelection:true, ApprovalEvents:false, Mcp:false, Vision:false } }` MVP honest (mirrors Task-165 staged `ApprovalEvents`/`Mcp` false until Task-303 proves round-trip); `Mcp` becomes `mcpServer!=nil` only after Task-303 `claudeMCP.waitReady` wiring like `grok_adapter.go:237` (`return a.mcpServer!=nil`). Keep `Vision=false` until `promptCapabilities.image` proven in T-2c `initialize`.
- `T-9` `apps/local-runner/internal/runner/opencode_adapter_test.go` + `opencode_event_mapper_test.go` + `opencode_reasoning_test.go` — Current: none. Delta: add fake-transport tests copying `grok_adapter_test.go` shapes: `TestOpencodeAdapterSendTurnStreamsAndCompletes` (fake `io.Pipe` stdio → `initialize`+`session/new`(`ses_test123`)+`session/update` `text` chunks → `message_delta`/`message_completed`→`turn_completed`), `...WithToolAndFileEvents` (`tool_call`→`tool_started`, `tool_call_update` with `path/newText`→`tool_completed`+`EventFileChanged`), `...SyntheticIdFails` (`ProviderSessionID:"thread-7"` + empty `byRun` → `err != nil` contains "no real session"), `...AdoptsPostPromptSessionID` (`turn_completed._meta.sessionId:"ses_new"` differs → second `UpsertSession` called), `...HandlesInterrupt` (`ctx cancel → session/cancel → ctx.Err()`), `...HandlesProcessFailure` (`dispatcher.fail` → `turn_failed`), `...ProviderKeyFromModelRouting`, `TestOpencodeEventMapperTurnCompletedToFailed`, `...TokenUsageToEvent` (ACP `turn_completed._meta.tokens` fixture), `TestOpencodeReasoningVariantID` table, `TestOpencodeInjectSelectedSkills` (wire `promptPrep` → `r.injectSelectedSkills` at `provider_registry.go:455` closure).
- `T-10` `apps/local-runner/internal/runner/provider_registry.go:221-247 DefaultProviderRegistry` + `:455 ProviderRegistryFor` live — **copy `provider_registry.go:455-524 grokAgentEnabled` block**. Current: `DefaultProviderRegistry` (`:221`) registers `codex` available, `claude`/`gemini` placeholder only — **no grok/opencode**; `ProviderRegistryFor` (`:256` live) does `if grokAgentEnabled(){ reg.register(ProviderRegistration{Key:ProviderKeyGrok, DisplayName:"Grok", Status:Available, ... newAdapterForTurn: func(model,effort string) ... scopeKey=account.ID, env[HOME/XDG_CONFIG_HOME/GROK_HOME], ... grokProcesses map keyed scope+model+effort+alwaysApprove })}` at `:455`. Delta: after grok block, append `if opencodeAgentEnabled(){ reg.register(ProviderRegistration{Key:ProviderKeyOpencode, DisplayName:"Opencode", Status:ProviderStatusAvailable, Capabilities: opencodeAdapter{}.Capabilities(), newAdapterForTurn: func(model,variant string) ProviderRuntimeAdapter { scopeKey,env:=resolveOpencodeAccountLikeGrok(); return newOpencodeAdapter(dispatcherFor(scopeKey,env,model,variant,auto)) } })}` OR `newAdapter` plain if T-2c proves per-session caps (document choice, keep `newAdapter` vs `newAdapterForTurn` at `provider_registry.go:104-119` distinction). ScopeKey = `account.ID` (`ProviderKey:"opencode"` home `OPENCODE_CONFIG` per Task-302), env = `HOME`+`XDG_CONFIG_HOME`+`OPENCODE_CONFIG` (Windows `USERPROFILE/APPDATA/...` via `windowsHomeDriveAndPath` like Grok `:460`). Keep `DefaultProviderRegistry` **without** opencode entry — verified by `TestDefaultProviderRegistryHasNoOpencode` (like Grok `provider_registry.go:221-247`). Gate via `opencodeAgentEnabled()` (`FLOWPILOT_OPENCODE_AGENT=0/false/no` opts out, on by default) copying `grok_process.go:33-38`.
- `T-11` Base-regression — Current: `provider_event.go` switch order, `provider_registry.go` map. Delta: edits additive; `providerKeyFromModel` cases `gpt-/gemini-/claude-/grok-/opencode/` unchanged order (opencode last); `go test ./internal/runner -run TestProviderKeyFromModel` + `go test ./...` in `apps/local-runner` unchanged vs baseline (snapshot before/after). Do NOT touch `gemini_acp_transport.go` zero diff.

## 5. Touched Areas

- files: new `apps/local-runner/internal/runner/opencode_adapter.go`, `opencode_event_mapper.go`, `opencode_reasoning.go`, `opencode_adapter_test.go`, `opencode_event_mapper_test.go`, `opencode_reasoning_test.go`; edits to `provider_event.go:10`, `provider_registry.go:202` + `255-553` (live `ProviderRegistryFor` only), `yolo_resolver.go` (add `OpencodePermissionMode` field appended — MVP adds field, Task-303 wires full policy), `runner.go` (add `opencodeProcessMu`/`opencodeProcess` fields only if not already in Task-300)
- modules: local runner provider runtime
- routes: existing `/client/workflow-runs/.../turns`
- tables: none (reuses provider-session persistence)

## 6. Acceptance Check

- Opencode adapter unit + fake-transport tests pass with ACP `session/update`/`turn_completed` fixtures (`step_start/text/step_finish` only for one-shot contrast; `ProviderModelNotFoundError` for `opencode/deepseek-v4-flash-free` handled as typed terminal).
- `DefaultProviderRegistry` has **no** opencode entry (like Grok `provider_registry.go:221-247`); `ProviderRegistryFor` returns Opencode `Available` with only proven caps when flag on and account resolvable.
- Model routing test covers `opencode/*` + `opencode-go/*` → `ProviderKeyOpencode`; `opencode/deepseek-v4-flash-free` → typed error with suggestions.
- Live (manual, credentialed) smoke run with `opencode/muse-spark-1.2-contributor-free` streams real text and completes via `opencode acp` (not one-shot `run`).

### 6.1 Definition of Done (DOD)

- [ ] `DOD-1` `opencodeAdapter` implements `ProviderRuntimeAdapter` and emits normalized `message_delta`/`tool_started`/`tool_completed`/`file_changed`/`turn_completed`. (`TestOpencodeAdapterSendTurnStreamsAndCompletes`.)
- [ ] `DOD-2` Adapter drives `opencode acp` through Task-300 process boundary using ACP `initialize`, `session/new`, `session/prompt`. (`opencode_adapter.go SendTurn`/`ensureOpencodeProcess`.)
- [ ] `DOD-3` Capabilities honest: `Streaming/Resume/FileEvents/Interrupt/SkillSelection` true, `ApprovalEvents/Mcp/Vision` false in MVP. (`TestOpencodeCapabilitiesMatchProvenSet`.)
- [ ] `DOD-4` Live `ProviderRegistryFor` returns Opencode `Available` with proven caps; `DefaultProviderRegistry` has no opencode entry. (`TestProviderRegistryForOpencodeUsesLiveWhenFlagOn`, `TestDefaultProviderRegistryHasNoOpencode`.)
- [ ] `DOD-5` `providerKeyFromModel` `opencode/`+`opencode-go/` routing proven; `gpt-/claude-/gemini-/grok-` unchanged. (`TestProviderKeyFromModelBaseRegressionPlusOpencode`.)
- [ ] `DOD-6` Real `ses_*` sessionId captured and persisted via `ProviderSessionStore.UpsertSession`; resume test reuses it. (Fake-transport resume + `TestLiveRealOpencodeChatStreamAndResume` manual.)
- [ ] `DOD-7` Synthetic `thread-*` with no real map → explicit fail, does not create fresh `session/new`. (`TestOpencodeSyntheticIdFails`.)
- [ ] `DOD-8` Post-prompt `_meta.sessionId` adoption via ACP `turn_completed` proven (`TestOpencodeAdoptsPostPromptSessionID`).
- [ ] `DOD-9` `ReasoningEffort` → `opencodeReasoningVariantID` mapping proven with table. (`TestOpencodeReasoningVariantID` + `TestOpencodeReasoningDegradesToDefault`.)
- [ ] `DOD-10` Token usage + `ModelContextWindow` populate reach `EventTokenUsageUpdated` (or explicit absence). (`TestOpencodeTokenUsageReporting` with ACP `turn_completed` tokens/`cost` per Task-300 live capture.)
- [ ] `DOD-11` Base-regression: `provider_event.go`/`provider_registry.go` additive; full suite unchanged vs baseline (`go test ./internal/runner -run TestProviderKeyFromModel` + `go test ./...`).
- [ ] `DOD-12` Prompt prep reuses `injectSelectedSkills` path (`provider_registry.go` opencode registration wires `promptPrep`).

### 6.2 Test Signatures

```go
// opencode_adapter_test.go
func TestOpencodeAdapterSendTurnStreamsAndCompletes(t *testing.T)
func TestOpencodeAdapterSendTurnWithToolAndFileEvents(t *testing.T)
func TestOpencodeAdapterSyntheticIdFails(t *testing.T)
func TestOpencodeAdapterAdoptsPostPromptSessionID(t *testing.T)
func TestOpencodeAdapterSessionPersistenceAndResume(t *testing.T)
func TestOpencodeAdapterHandlesInterrupt(t *testing.T)
func TestOpencodeAdapterHandlesProcessFailure(t *testing.T)
func TestOpencodeProviderKeyFromModelRouting(t *testing.T)
func TestOpencodeProviderKeyFromModelBaseRegressionPlusOpencode(t *testing.T)

// opencode_event_mapper_test.go
func TestOpencodeEventMapperTextChunksToDelta(t *testing.T)
func TestOpencodeEventMapperToolCallToToolStartedCompleted(t *testing.T)
func TestOpencodeEventMapperDerivesFileChangedFromWriteDiff(t *testing.T)
func TestOpencodeEventMapperTurnCompletedToFailed(t *testing.T)
func TestOpencodeEventMapperTokenUsageToEvent(t *testing.T)
func TestOpencodeCapabilitiesMatchProvenSet(t *testing.T)

// opencode_reasoning_test.go
func TestOpencodeReasoningVariantID(t *testing.T)
func TestOpencodeReasoningDegradesToDefault(t *testing.T)
func TestOpencodeInjectSelectedSkills(t *testing.T)
```

## 7. Out of Scope

- Full `session/request_permission` → `bridge.RequestApproval` policy with runner SSOT `YoloPosture` decision mapping, MCP `ask_user`/`spawn_agent`, YOLO `config.toml` rewrite analogue — Task-303.
- Multi-account isolation, quota display, desktop UI — Task-302/303.
- Cross-account resume safety and transcript extraction — Task-303 hardening.

## 8. Completion Notes

- result:
- implementation notes: `opencode_adapter.go` (`SendTurn`/`ensureOpencodeProcess`), `opencode_event_mapper.go` (`session/update` + ACP `turn_completed` + tokens), `provider_event.go:10`/`provider_registry.go:202` (`ProviderKeyOpencode`, appended `providerKeyFromModel`), `opencode_reasoning.go`.
- verification:
- follow-ups: `ReasoningEffort` wiring may need process respawn on change if Opencode treats `--variant` as launch flag like Grok (verify live; if so, make `grokProcesses map` analog `opencodeProcesses` keyed by `scopeKey+model+variant+auto`).
- upstream docs updated: `CP-57` rows `OC-01`, `OC-03`, `OC-08`, `OC-11`, `OC-15`, `OC-24`, `OC-35`
