# Task-401: Devin Controlled Adapter MVP & Polymorphic Event Mapper

## Metadata

- Document ID: `Task-401`
- Title: `Devin Controlled Adapter MVP & Polymorphic Event Mapper`
- Phase: `task`
- Status: `todo`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-20`
- Last Updated: `2026-09-20`
- Parent Documents: [CP-70: Devin Provider Integration](../../07-Coding-Plan/todo/CP-70-Devin-Provider-Integration.md)
- Child Documents: `None`
- Related Documents: [Task-301: Opencode Controlled Adapter MVP](../../08-Task/done/Task-301-Opencode-Controlled-Adapter-MVP.md), [Task-400: Devin ACP Transport](../../08-Task/todo/Task-400-Devin-ACP-Transport-And-Process-Dispatcher.md), [CP-70-note.md](../../07-Coding-Plan/note/CP-70-note.md), [CP-70-Test-Steps.md](../../07-Coding-Plan/todo/CP-70-Test-Steps.md)
- Replaces: `None`
- Tags: `devin, adapter, provider-runtime, event-mapper, polymorphic-chunks, local-runner`

## AI Quick View

### Summary

- Implement `devinAdapter` fulfilling the `ProviderRuntimeAdapter` interface (`Key()`, `Capabilities()`, `SendTurn()`), driving the underlying `devin acp` subprocess created in Task-400.
- Build `devin_event_mapper.go` to translate raw ACP notifications (`session/update`, `turn_completed`) into normalized `ProviderEvent` streams (`message_delta`, `tool_started`, `tool_completed`, `file_changed`, `token_usage`, `turn_completed`).
- Implement polymorphic stream parsing (CA-709): robustly handle both string chunks (`content: "..."`) and array chunks (`content: [{type: "text", text: "..."}]`).
- Handle Quota and billing errors correctly (BUG-361 / CA-759): recognize quota `stopReason` and map to `EventTurnFailed(Recoverable: false)` without blind turn-level timeouts.
- Implement Deny clean exit (CA-712 / CA-713): when the user denies permission and CLI aborts with zero text, synthesize a clear feedback message rather than leaving a blank turn.
- Register `ProviderKeyDevin = "devin"` into `agentpack/pack.go:338` (SSOT model prefix routing) and `ProviderRegistryFor` (gated by `FLOWPILOT_DEVIN_AGENT=1`), keeping `DefaultProviderRegistry` strictly without Devin.
- Enforce truthful MVP capabilities: `{Streaming: true, Resume: true, FileEvents: true, Interrupt: true, SkillSelection: true, ApprovalEvents: false, Mcp: false, Vision: false}`.

### Current Ask

- Build the core adapter, polymorphic event mapper, reasoning effort resolver, and session persistence layer.
- Prove complete turn lifecycle through fake-transport unit tests and golden-fixture replay tests before adding desktop UI in Task-402 and Task-403.

### Key Decisions

- `T-1` SSOT Model Routing: Add `devin/` prefix routing to `agentpack/pack.go:338` (`ModelProviderKey`), preserving prefix ordering for existing providers.
- `T-2` Registry Gating: Register Devin in `ProviderRegistryFor` only when `devinAgentEnabled()` is true; `DefaultProviderRegistry` remains completely unchanged.
- `T-3` Truthful Capability Staging: Set `ApprovalEvents: false`, `Mcp: false`, `Vision: false` in MVP. These flags remain false until Task-403 wires live MCP loopback and proves approval round-trip.
- `T-4` Quota Classification over Timeout: Per CA-759, map quota/billing errors to non-recoverable failures via `devinIsQuotaStopReason`; do NOT add arbitrary turn timeouts that collide with long code generation.
- `T-5` Synthetic Deny Message: When permission is denied and the turn finishes with 0 text, synthesize `[Devin aborted turn after permission was denied; no reply text generated.]` (CA-712/713).
- `T-6` Session Persistence SSOT: Persist real `sessionId` via `ProviderSessionStore.UpsertSession`. Reject synthetic `thread-*` IDs and adopt post-prompt `_meta.sessionId` updates.

### Constraints

- `agentpack/pack.go` edits must be strictly additive (append `case "devin"`).
- `provider_event.go` edits must append `ProviderKeyDevin = "devin"` at the end of the const block.
- Zero diff to existing provider adapters (`opencode_adapter.go`, `grok_adapter.go`, `gemini_adapter.go`, `codex_adapter.go`, `claude_adapter.go`).

### Open Questions

- `Q-1` ✅ Đã trả lời (F-23): `usage_update` notification `{used,size,_meta.inputTokens/outputTokens}` + `prompt.result.usage{totalTokens,inputTokens,outputTokens,cachedReadTokens}`; thêm `_cognition.ai/agent_stopped{modelLabel,ttftMs,tokensPerSec}` cho stats.
- `Q-2` ✅ Đã trả lời (F-18): slug `adjective-noun`.
- `Q-3` ⏳ `session/request_permission` optionIds chưa capture (F-25) — Task-400 trigger trước.

### Source Refs

- `CP-70` §3 Key Decisions `P-2`, `P-4`, `P-6`, `P-7`, `P-8`; §4 Work Breakdown `P-3`, `P-4`, `P-5`, `P-9`; §5.1 Base Regression Guard; §5.2 Rows 1, 2, 3, 12, 13, 18, 19, 23.
- `CP-70-note.md` Issues 1, 3, 4, 6, 7, 8, 9, 12, 13, 14.
- Reference code: `apps/local-runner/internal/runner/opencode_adapter.go`, `opencode_event_mapper.go`, `opencode_reasoning.go`.

---

## 1. Goal

Deliver a fully functional `ProviderRuntimeAdapter` implementation for Devin with normalized event streaming, polymorphic chunk handling, quota error classification, session persistence, and zero base regression for existing providers.

---

## 2. Parent Links

- Coding Plan: [CP-70: Devin Provider Integration](../../07-Coding-Plan/todo/CP-70-Devin-Provider-Integration.md) (Work Breakdown `P-3`, `P-4`, `P-5`, `P-9`, `P-11`)
- Tech Design: `SD-06`, `SD-11`
- System Spec: `SS-05`
- Upstream Task Reference: [Task-301: Opencode Controlled Adapter MVP](../../08-Task/done/Task-301-Opencode-Controlled-Adapter-MVP.md)

---

## 3. Trigger

With Task-400 providing the low-level ACP transport and process dispatcher, Task-401 constructs the bridge between FlowPilot's universal execution engine and Devin CLI.

---

## 4. Exact Change

- `T-1` **Provider Key & Constants (`provider_event.go`)**:
  - Append `ProviderKeyDevin = "devin"` to `ProviderKey` const definitions.
  - Add `case ProviderKeyDevin: return "Devin"` in `ProviderKey.DisplayName()`.

- `T-2` **Model Prefix Routing (`agentpack/pack.go:338`)**:
  - In `ModelProviderKey(model string) string`:
  - Add `if strings.HasPrefix(m, "devin/") { return runner.ProviderKeyDevin }` without altering precedence of existing prefixes.

- `T-3` **Devin Adapter Implementation (`devin_adapter.go`)**:
  - Define `devinAdapter struct`:
    - `runner *Runner`
    - `processHandle *devinProcessHandle`
    - `logger *log.Logger`
    - `mcpServer *claudeMCPServer` (nil in MVP)
  - Implement `ProviderRuntimeAdapter` interface:
    - `Key() string`: Returns `ProviderKeyDevin`.
    - `Capabilities() ProviderCapabilities`: Returns `{Streaming: true, Resume: true, FileEvents: true, Interrupt: true, SkillSelection: true, ApprovalEvents: false, Mcp: false, Vision: false}`.
    - `SendTurn(ctx context.Context, req TurnRequest, bridge TurnBridge) error`:
      1. Verify or initialize session (`session/new` vs `session/load`) — process handle đã qua `initialize → authenticate` từ Task-400 (F-3); adapter nhận typed `auth_required` nếu authenticate thất bại.
      2. If resuming with synthetic `thread-*` ID and no real session mapping, reject with typed error (DOD-7).
      3. Set up notification channel and inbound handler for permission requests.
      4. Call `session/prompt` with prepared prompt.
      5. Process updates via `devinEventMapper`, emitting `ProviderEvent` through `bridge.Send()`.
      6. Handle user cancellation (`ctx.Done()`) by sending `session/cancel`.
      7. Persist real `sessionId` via `runner.sessionStore.UpsertSession`.
      8. Detect Deny clean exit with zero text and emit synthetic explanation message (CA-712/713).

- `T-4` **Polymorphic Event Mapper (`devin_event_mapper.go`)**:
  - Implement `devinEventMapper`:
    - Parse `agent_message_chunk` supporting polymorphic text: both raw string and array `[{type: "text", text: "..."}]` (CA-709).
    - Map `tool_call` to `EventToolStarted`.
    - Map `tool_call_update` with diff to `EventToolCompleted` and `EventFileChanged`.
    - Map `turn_completed` stop reasons:
      - Normal finish → `EventTurnCompleted`.
      - Quota / billing error → `EventTurnFailed(Recoverable: false)` via `devinIsQuotaStopReason` (BUG-361 / CA-759).
    - Extract tokens and cost from `turn_completed._meta` and emit `EventTokenUsageUpdated`.

- `T-5` **Reasoning & Effort Resolver (`devin_reasoning.go`)**:
  - Implement `devinReasoningEffortID(effort string) (string, bool)`:
    - **Đã verify (F-20)**: effort là suffix trong model ID — map `low→-low`, `medium→-medium`, `high→-high`, `xhigh→-xhigh`, `max→-max`; apply qua `session/set_config_option{configId:"model", value:"<family>-<effort>"}` mid-session.
    - Family không có variant effort (vd `swe-1-6`, `glm-5-2`, `adaptive`) → `("", false)` để giữ model hiện tại; variant `-fast`/`-priority`/`-1m` giữ nguyên khi đổi effort nếu đang bật.

- `T-6` **Live Provider Registry Registration (`provider_registry.go`)**:
  - In `ProviderRegistryFor`:
    - Check `if devinAgentEnabled()`:
    - Register `ProviderRegistration{Key: ProviderKeyDevin, DisplayName: "Devin", Status: ProviderStatusAvailable, Capabilities: devinAdapter{}.Capabilities(), ...}`.
  - `DefaultProviderRegistry` remains strictly untouched (verified by test).

- `T-7` **Session Store Integration & Real Session ID Checks**:
  - Implement `isDevinRealSessionID(id string) bool`:
    - Format verified (F-18): slug `adjective-noun` (`sore-router`, `generated-iguanodon`) — regex `^[a-z]+-[a-z0-9-]+$` chấp nhận; synthetic `thread-*` vẫn reject.
  - In `interactive_service.go` transcript persistence condition, include `ProviderKeyDevin` (DB-backed provider như OpenCode — sessions nằm trong `~/.local/share/devin/cli/sessions.db`, không có per-session file, F-5).

---

## 5. Touched Areas

- **files (runner, new):**
  - `apps/local-runner/internal/runner/devin_adapter.go`
  - `apps/local-runner/internal/runner/devin_event_mapper.go`
  - `apps/local-runner/internal/runner/devin_reasoning.go`
  - `apps/local-runner/internal/runner/devin_adapter_test.go`
  - `apps/local-runner/internal/runner/devin_event_mapper_test.go`
  - `apps/local-runner/internal/runner/devin_reasoning_test.go`
- **files (runner, extend):**
  - `apps/local-runner/internal/runner/provider_event.go` (add `ProviderKeyDevin`)
  - `apps/local-runner/internal/agentpack/pack.go` (add `devin/` to `ModelProviderKey`)
  - `apps/local-runner/internal/runner/provider_registry.go` (register in `ProviderRegistryFor`)
  - `apps/local-runner/internal/runner/interactive_service.go` (add to transcript condition)
- **modules:** Local runner provider runtime & event pipeline
- **routes:** Existing `/client/workflow-runs/.../turns`
- **tables:** None (reuses `ProviderSessionStore`)

---

## 6. Acceptance Check

### 6.1 Definition of Done (DOD)

- [ ] `DOD-1` `devinAdapter` implements `ProviderRuntimeAdapter` and emits normalized events (`TestDevinAdapterSendTurnStreamsAndCompletes`).
- [ ] `DOD-2` Adapter drives `devin acp` process boundary using ACP `initialize → authenticate`, `session/new`, `session/prompt` (`TestDevinAdapterDrivesProcessLifecycle`).
- [ ] `DOD-3` Capabilities are truthful: `Streaming/Resume/FileEvents/Interrupt/SkillSelection` true; `ApprovalEvents/Mcp/Vision` false in MVP (`TestDevinCapabilitiesMatchProvenSet`).
- [ ] `DOD-4` `ProviderRegistryFor` returns Devin as `Available` when flag is on; `DefaultProviderRegistry` has no Devin entry (`TestDefaultProviderRegistryHasNoDevin`, `TestProviderRegistryForDevinWhenFlagOn`).
- [ ] `DOD-5` `ModelProviderKey` routes `devin/*` to `ProviderKeyDevin`; existing providers unchanged (`TestDevinModelProviderKeyRouting`).
- [ ] `DOD-6` Real session ID captured and persisted via `ProviderSessionStore.UpsertSession`; resume test reuses it (`TestDevinSessionPersistenceAndResume`).
- [ ] `DOD-7` Synthetic `thread-*` ID without existing session map is rejected with typed error (`TestDevinSyntheticIdRejected`).
- [ ] `DOD-8` Post-prompt `_meta.sessionId` adoption updates session store (`TestDevinAdoptsPostPromptSessionID`).
- [ ] `DOD-9` Polymorphic message chunks (string and array) parse without data loss or crash (`TestDevinEventMapperPolymorphicChunks`).
- [ ] `DOD-10` Quota `stopReason` mapped to `EventTurnFailed(Recoverable: false)` without blind timeout (`TestDevinEventMapperQuotaStopReason`).
- [ ] `DOD-11` Deny permission with zero reply text produces synthetic explanatory message (`TestDevinCleanDenyExitSyntheticMessage`).
- [ ] `DOD-12` Base regression: full runner test suite passes 100% (`go test ./internal/runner`).

### 6.2 Test Signatures

```go
// devin_adapter_test.go
func TestDevinAdapterSendTurnStreamsAndCompletes(t *testing.T)
func TestDevinAdapterSendTurnWithToolAndFileEvents(t *testing.T)
func TestDevinAdapterSyntheticIdRejected(t *testing.T)
func TestDevinAdapterAdoptsPostPromptSessionID(t *testing.T)
func TestDevinAdapterSessionPersistenceAndResume(t *testing.T)
func TestDevinAdapterHandlesInterrupt(t *testing.T)
func TestDevinAdapterHandlesProcessFailure(t *testing.T)
func TestDevinCleanDenyExitSyntheticMessage(t *testing.T)
func TestDefaultProviderRegistryHasNoDevin(t *testing.T)
func TestProviderRegistryForDevinWhenFlagOn(t *testing.T)

// devin_event_mapper_test.go
func TestDevinEventMapperTextChunksToDelta(t *testing.T)
func TestDevinEventMapperPolymorphicChunks(t *testing.T)
func TestDevinEventMapperToolCallToToolEvents(t *testing.T)
func TestDevinEventMapperFileChangedFromDiff(t *testing.T)
func TestDevinEventMapperQuotaStopReason(t *testing.T)
func TestDevinEventMapperTokenUsageToEvent(t *testing.T)
func TestDevinCapabilitiesMatchProvenSet(t *testing.T)

// devin_reasoning_test.go
func TestDevinReasoningEffortIDMapping(t *testing.T)
```

### 6.3 Code Signatures

```go
// devin_adapter.go
type devinAdapter struct {
    runner        *Runner
    processHandle *devinProcessHandle
    logger        *log.Logger
    mcpServer     *claudeMCPServer
}

func newDevinAdapter(runner *Runner, handle *devinProcessHandle) *devinAdapter
func (a *devinAdapter) Key() string
func (a *devinAdapter) Capabilities() ProviderCapabilities
func (a *devinAdapter) SendTurn(ctx context.Context, req TurnRequest, bridge TurnBridge) error

func isDevinRealSessionID(id string) bool

// devin_event_mapper.go
type devinEventMapper struct {
    bridge TurnBridge
    logger *log.Logger
}

func (m *devinEventMapper) HandleUpdate(update map[string]any) error
func (m *devinEventMapper) HandleTurnCompleted(result map[string]any) error
func devinIsQuotaStopReason(reason string) bool

// devin_reasoning.go
func devinReasoningEffortID(effort string) (string, bool)
```

---

## 7. Out of Scope

- Desktop Settings UI (CLI detection, installation, catalog probe) — Task-402.
- Interactive Approval Gate UI modal and live YOLO toggle — Task-403.
- External MCP Server execution in chat turns — Task-403.
- Sub-agent UI panel parity — Task-403.

---

## 8. Completion Notes

- result: pending implementation
- implementation notes:
- verification:
- follow-ups: Unblocks Task-402 and Task-403
- upstream docs updated: `CP-70` Work Breakdown `P-3`, `P-4`, `P-5`, `P-9`
