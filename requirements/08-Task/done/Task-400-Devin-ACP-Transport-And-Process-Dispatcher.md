# Task-400: Devin ACP Transport And Process/Dispatcher

## Metadata

- Document ID: `Task-400`
- Title: `Devin ACP Transport And Process/Dispatcher`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-20`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-70: Devin Provider Integration](../../07-Coding-Plan/done/CP-70-Devin-Provider-Integration.md)
- Child Documents: `None`
- Related Documents: [Task-300: Opencode ACP Transport And Process/Dispatcher](../../08-Task/done/Task-300-Opencode-ACP-Transport-And-Process-Dispatcher.md), [CP-70-note.md](../../07-Coding-Plan/note/CP-70-note.md), [CP-70-Test-Steps.md](../../07-Coding-Plan/done/CP-70-Test-Steps.md)
- Replaces: `None`
- Tags: `devin, acp, json-rpc, transport, dispatcher, local-runner, process-isolation`

## AI Quick View

### Summary

- Freeze the Devin **ACP** wire contract (`initialize` / **`authenticate`** / `session/new` / `session/prompt` / `session/update` / `session/request_permission` / `session/cancel` / `turn_completed` / `session/load` / `session/list`) into typed Go structs and golden fixtures using live `devin acp` capture.
- Build standalone `devin_acp.go` + `devin_acp_types.go` + `devin_process.go` — copy proven patterns from `opencode_acp.go` / `opencode_process.go` without touching or refactoring `gemini_acp_transport.go`, `grok_acp.go`, or `opencode_*.go` (CP-70 `P-0` Zero Base Regression).
- Add `devinDispatcher` (waiters map, per-`sessionId` subs, single inbound handler, one read-loop, `fail()` drain, **tolerate unknown `_cognition.ai/*` notifications**) + `ensureDevinProcess(ctx, scopeKey, cwd, extraEnv, model, permissionMode)` (spawn `devin acp`, boot sequence `initialize → authenticate`, hold one shared handle per `scopeKey`, teardown on account switch).
- Implement **Child Process Scope Isolation** (`account|child:<runId>` and `account|probe:<runId>`) with dedicated process cleanup `CloseProcessesForChildRun(runID)` to permanently prevent BUG-334 MCP client reset and process leaks.
- Enforce strict environment isolation: strip host secrets (`WINDSURF_API_KEY`, `DEVIN_*` tokens), redirect `HOME` + `XDG_CONFIG_HOME` + `XDG_DATA_HOME` về managed account home (config `~/.config/devin`, data `~/.local/share/devin`), và add credential redaction via `redactDevinFrameForLog`. **Không có `DEVIN_HOME`/`DEVIN_CONFIG_CONTENT`** (live-verified).
- **ACP `authenticate` handshake (live-verified 3000.10.31)**: `devin acp` bỏ qua credentials local (`WINDSURF_API_KEY`, `credentials.toml`) — *"ACP host is the sole source of credentials"*. Process phải gọi `authenticate` sau `initialize`, trước `session/new`; auth failure → typed `auth_required`.

### Current Ask

- Build the transport and process layer only — no `ProviderRuntimeAdapter` implementation yet, no desktop UI changes, no permission decision policy (generic passthrough only).
- Output is a Go-level ACP client usable by Task-401: connect, initialize, create/load session, send prompt, receive typed notifications, route inbound permission requests, and handle cancellation.

### Key Decisions

- `T-1` Standalone implementation: create new `devin_acp.go`, `devin_acp_types.go`, `devin_process.go`; do NOT extract shared ACP code or touch existing provider files (P-0 guard).
- `T-2` Child Process Scope Isolation: child agents and variants probe run on isolated processes with keys `scopeKey+"|child:"+runID` and `scopeKey+"|probe"`, preventing parent MCP disconnection (BUG-334).
- `T-3` Process Key Model Exclusion: do NOT include `model` in `processKey` if Devin models are configured per-session via `session/new` or `session/set_config_option`, avoiding BUG-329 session loss during mid-chat model switches.
- `T-4` Environment & Secret Isolation: set `HOME`/`XDG_CONFIG_HOME`/`XDG_DATA_HOME` per managed account (Devin đọc `~/.config/devin` + `~/.local/share/devin`; Windows `%APPDATA%\devin`), strip host secrets (`WINDSURF_API_KEY`, `DEVIN_*`), ensure config paths point to files not directories (CA-679), redact credentials from log output. Không tồn tại `DEVIN_HOME`.
- `T-5` **Authenticate-first boot**: `devin acp` ignores all local credentials and waits for host `authenticate` (F-3). Process handle tracks `authed bool`; `session/new` chỉ được gọi sau khi authenticate thành công; lỗi map typed `auth_required`.
- `T-6` **Extension tolerance**: dispatcher bỏ qua êm `_cognition.ai/*` notifications (`output`, `mcp/serversChanged`) và mọi method không nhận diện được (F-4).

### Constraints

- `opencode_*.go`, `grok_*.go`, `gemini_*.go`, `codex_*.go`, `claude_*.go` must remain byte-identical (verified by empty git diff).
- Do not implement `ProviderRuntimeAdapter` or UI code in this task (reserved for Task-401 and Task-403).
- Never log raw ACP frames containing authentication tokens or credentials without redaction — `authenticate` params và `_meta` auth fields phải qua `redactDevinFrameForLog`.

### Open Questions (probe trong live capture; hầu hết đã trả lời — xem CP-70 §3.0 F-17..F-26)

- `Q-1` ✅ sessionId = **slug `adjective-noun`** (`sore-router`, `generated-iguanodon`); `session/list` keyed canonical cwd (`/tmp`→`/private/tmp`); `session/load` verified.
- `Q-2` ✅ Model catalog ~380 entries từ `session/new.result.configOptions.model.options`; default `swe-2-high`. Nguồn này tốt hơn `devin models list` (REPL auth tách biệt, đang fail trên máy dev).
- `Q-3` ✅ `authenticate{methodId:"devin-browser"}` → `{}`; PKCE ~3s nhờ browser session; **không có token method** trong `authMethods` (chỉ `devin-browser`) — managed account phải có browser login hoặc REPL `devin auth login` trước.
- `Q-4` ✅ `session/new` chấp nhận `{cwd,mcpServers[]}` (stdio-only); `session/set_config_option` tồn tại và switch được `model` lẫn `mode` mid-session (verified).
- `Q-5` ⏳ `session/request_permission` chưa trigger trong probe (`accept-edits` auto-approved `echo`) — **việc còn lại duy nhất của Task-400**: capture bằng mode `smart`/`auto` + dangerous command để freeze optionIds. `prompt.result.usage` = `{totalTokens,inputTokens,outputTokens,cachedReadTokens}` (verified).
- `Q-6` ⏳ `devin -p` output format chưa verify (REPL auth đang fail — cần `devin auth login` thật hoặc chỉ dùng `acp --agent-type summarizer`).

### Source Refs

- `CP-70` §3 Key Decisions `P-1`, `P-2`, `P-3`; §4 Work Breakdown `P-1`, `P-2`; §5.1 Base Regression Guard; §5.2 Rows 1, 2, 10.
- `CP-70-note.md` Section 1 (ACP Feasibility), Section 2 (Issues 1, 2, 3, 10, 11).
- Reference code: `apps/local-runner/internal/runner/opencode_acp.go`, `opencode_process.go`, `grok_process.go`.

---

## 1. Goal

Produce a reusable Go ACP JSON-RPC 2.0 stdio transport and a scoped persistent process dispatcher for `devin acp`, allowing Task-401 to drive Devin turns through clean Go interfaces without raw process or JSON plumbing.

---

## 2. Parent Links

- Coding Plan: [CP-70: Devin Provider Integration](../../07-Coding-Plan/done/CP-70-Devin-Provider-Integration.md) (Work Breakdown `P-1`, `P-2`)
- Tech Design: `SD-06`, `SD-11`
- System Spec: `SS-05`
- Upstream Task Reference: [Task-300: Opencode ACP Transport](../../08-Task/done/Task-300-Opencode-ACP-Transport-And-Process-Dispatcher.md)

---

## 3. Trigger

CP-70 requires a frozen wire contract, typed message classes, and a reliable process dispatcher before the adapter, settings, and UI layers can be implemented.

---

## 4. Exact Change

- `T-1` **Typed ACP Message Structures (`devin_acp_types.go`)**:
  - Define Go structs for JSON-RPC 2.0 frames (`devinRPCRequest`, `devinRPCResponse`, `devinRPCNotification`).
  - Define request/response shapes:
    - `devinInitializeParams`, `devinInitializeResult` (`agentCapabilities`, `promptCapabilities`, `authMethods`).
    - `devinSessionNewParams`, `devinSessionNewResult` (`sessionId`, `_meta`).
    - `devinSessionLoadParams`, `devinSessionLoadResult`.
    - `devinSessionPromptParams`, `devinSessionPromptResult`.
    - `devinSessionCancelParams`.
    - `devinSessionUpdateNotification` (`type`, `content`, `toolCall`, `path`, `diff`).
    - `devinRequestPermissionParams` (`permission`, `pattern`, `options[]`).
    - `devinPermissionResponse` (`outcome: {optionId: string}`).
    - `devinTurnCompletedNotification` (`sessionId`, `stopReason`, `cost`, `_meta: {tokens, cost}`).
  - Provide helper `devinACPResponseSessionID(result map[string]any) string` checking `result["sessionId"]` and fallback `result["_meta"]["sessionId"]`.

- `T-2` **ACP Param Builders & Extraction Primitives (`devin_acp.go`)**:
  - `devinACPInitializeParams() map[string]any`: Build client initialization payload (clientCapabilities fs/terminal).
  - `devinACPAuthenticateParams(methodId string) map[string]any`: Build `authenticate` payload (F-3 — bắt buộc sau initialize).
  - `devinACPSessionNewParams(cwd string, mcpServers map[string]any, model string) map[string]any`: Build `session/new` payload — `mcpServers` chỉ stdio (F-2 `mcpCapabilities http/sse:false`).
  - `devinACPSessionLoadParams(sessionId, cwd string, mcpServers map[string]any) map[string]any`: Build `session/load` payload (`loadSession:true` confirmed).
  - `devinACPSessionListParams(cwd string) map[string]any`: Build `session/list` payload (sessionCapabilities.list — session discovery).
  - `devinACPPromptParams(sessionId, prompt string) map[string]any`: Build `session/prompt` payload.
  - `devinACPExtractText(update map[string]any) string`: Polymorphic extractor supporting string text and array `[{type:"text", text:"..."}]` (CA-709).
  - `devinACPIsPermissionRequest(method string) bool`: Detect `session/request_permission`.
  - `devinACPIsExtensionNotification(method string) bool`: Detect `_cognition.ai/*` và unknown `_*` methods → bỏ qua êm (F-4).
  - `devinACPFlowPilotMCPServerEntry(baseURL, token string) map[string]any`: Loopback MCP configuration (stdio).

- `T-3` **Scoped Process Dispatcher (`devin_process.go`)**:
  - `devinDispatcher`: Mutex-guarded async JSON-RPC 2.0 dispatcher with `waiters map[int64]chan devinResponse`, `sessionSubs map[string]chan devinNotification`, single stdout read-loop goroutine, `call(req)` method, and `fail(err)` drain.
  - `devinProcessHandle`: Wraps `*exec.Cmd`, stdin writer, dispatcher, `scopeKey`, `cwd`, and status.
  - `Runner` extensions:
    - `devinProcessMu sync.Mutex`
    - `devinProcesses map[string]*devinProcessHandle`
  - `ensureDevinProcess(ctx context.Context, scopeKey, cwd string, extraEnv map[string]string, model string, permissionMode string) (*devinProcessHandle, error)`: Spawns or reuses `devin acp` subprocess; boot sequence `initialize → authenticate`; `permissionMode` map sang `DEVIN_PERMISSION_MODE` env (`auto`/`accept-edits`/`smart`/`dangerous`) — **không** phải flag `--auto` (F-7).
  - `CloseProcessesForChildRun(runID string)`: Scans `devinProcesses` and shuts down processes matching `child:<runID>` to prevent leaks (BUG-334).
  - `devinBinaryName() string`: Returns `"devin"` or `FLOWPILOT_DEVIN_BIN` override.
  - `devinAgentEnabled() bool`: Checks `FLOWPILOT_DEVIN_AGENT != "0"`.

- `T-4` **Environment & Secret Isolation**:
  - Configure `HOME`, `XDG_CONFIG_HOME`, `XDG_DATA_HOME` (Devin: config `~/.config/devin`, data `~/.local/share/devin`; Windows `%APPDATA%\devin` + `USERPROFILE`).
  - Strip host secrets (`WINDSURF_API_KEY`, `DEVIN_API_KEY`-like vars) from execution environment.
  - Ensure config file paths point to files, not directories (CA-679) — áp dụng cho `--config <PATH>` nếu dùng.
  - Implement `redactDevinFrameForLog(frame map[string]any) string` to sanitize credentials before logging (gồm `authenticate` params).

- `T-5` **Unit Tests & Golden Fixtures**:
  - `devin_acp_test.go`: Test param builders, JSON round-tripping, session ID extraction, polymorphic text extraction.
  - `devin_process_test.go`: Test dispatcher multiplexing, error draining, process reuse/respawn, child process cleanup.
  - Golden fixtures in `testdata/devin_acp/`: Captured wire traces for `initialize`, `session/new`, `session/prompt`, `session/update`, `turn_completed`.

---

## 5. Touched Areas

- **files (runner, new):**
  - `apps/local-runner/internal/runner/devin_acp_types.go`
  - `apps/local-runner/internal/runner/devin_acp.go`
  - `apps/local-runner/internal/runner/devin_process.go`
  - `apps/local-runner/internal/runner/devin_acp_test.go`
  - `apps/local-runner/internal/runner/devin_process_test.go`
  - `apps/local-runner/internal/runner/testdata/devin_acp/*.json`
- **files (runner, extend):**
  - `apps/local-runner/internal/runner/runner.go` (add `devinProcessMu`, `devinProcesses` fields and cleanup hooks)
- **modules:** Local runner provider transport layer
- **routes:** None
- **tables:** None

---

## 6. Acceptance Check

### 6.1 Definition of Done (DOD)

- [x] `DOD-1` All ACP message types serialize and deserialize through JSON without data loss (`TestDevinACPTypesRoundTrip`).
- [x] `DOD-2` Standalone ACP primitives exist; `opencode_*.go`, `grok_*.go`, `gemini_*.go` have zero git diff.
- [x] `DOD-3` `devinDispatcher` multiplexes concurrent sessions over a single stdio pipe correctly (`TestDevinDispatcherMultiplexesConcurrentSessions`).
- [x] `DOD-4` `ensureDevinProcess` reuses process handle for matching scopeKey and creates isolated handle for `child:<runId>` (verified: `TestDevinProcessKeyAndScopeReuse`).
- [x] `DOD-5` `CloseDevinProcessesForChildRun` terminates and deletes child processes cleanly without touching parent process (impl `devin_process.go`; live-verified R7 child `run-314`/`blushing-raver` reclaimed).
- [x] `DOD-6` Process launch isolates environment: host secrets stripped, `HOME`/`XDG_CONFIG_HOME`/`XDG_DATA_HOME` trỏ managed home (`TestDevinProcessEnvIsolates`).
- [x] `DOD-7` Credential redaction replaces authorization headers and token fields with `[redacted]` (verified: `TestDevinFrameRedaction`).
- [x] `DOD-8` Base-regression guard: `go test ./internal/runner -run 'TestOpencode|TestGrok|TestGemini|TestCodex|TestClaude'` passes 100%.
- [x] `DOD-9` Boot sequence gọi `initialize → authenticate` trước `session/new`; authenticate lỗi → typed `auth_required` (verified: `TestEnsureDevinProcessBootOrderIncludesAuthenticate`, `TestEnsureDevinProcessAuthenticateFailureKillsProcess`; live PKCE ~3s).
- [x] `DOD-10` Dispatcher bỏ qua êm `_cognition.ai/*` và unknown methods (`TestDevinDispatcherToleratesExtensionNotifications`).

### 6.2 Test Signatures

```go
// devin_acp_test.go
func TestDevinACPTypesRoundTrip(t *testing.T)
func TestDevinACPResponseSessionIDHandlesMetaFallback(t *testing.T)
func TestDevinACPSessionNewParamsBuildsCorrectJSON(t *testing.T)
func TestDevinACPSessionLoadParamsBuildsCorrectJSON(t *testing.T)
func TestDevinACPPromptParamsBuildsCorrectJSON(t *testing.T)
func TestDevinACPExtractTextPolymorphic(t *testing.T)
func TestDevinACPIsPermissionRequest(t *testing.T)

// devin_process_test.go
func TestDevinDispatcherMultiplexesConcurrentSessions(t *testing.T)
func TestDevinDispatcherRoutesPermissionRequestToInboundHandler(t *testing.T)
func TestDevinDispatcherFailDrainsWaitersAndSubs(t *testing.T)
func TestEnsureDevinProcessReusesOrRespawns(t *testing.T)
func TestEnsureDevinProcessChildScopeIsolation(t *testing.T)
func TestCloseProcessesForChildRun(t *testing.T)
func TestDevinProcessEnvIsolates(t *testing.T)
func TestEnsureDevinProcessHonorsAgentEnabledFlag(t *testing.T)
func TestDevinProcessAuthenticateBeforeSessionNew(t *testing.T)
func TestDevinAuthenticateFailureTypedError(t *testing.T)
func TestDevinDispatcherToleratesExtensionNotifications(t *testing.T)
func TestRedactDevinFrameForLogStripsCredentials(t *testing.T)
```

### 6.3 Code Signatures

```go
// devin_acp.go
func devinACPInitializeParams() map[string]any
func devinACPAuthenticateParams(methodId string) map[string]any
func devinACPSessionNewParams(cwd string, mcpServers map[string]any, model string) map[string]any
func devinACPSessionLoadParams(sessionId, cwd string, mcpServers map[string]any) map[string]any
func devinACPSessionListParams(cwd string) map[string]any
func devinACPPromptParams(sessionId, prompt string) map[string]any
func devinACPExtractText(update map[string]any) string
func devinACPResponseSessionID(result map[string]any) string
func devinACPIsPermissionRequest(method string) bool
func devinACPIsExtensionNotification(method string) bool
func devinACPFlowPilotMCPServerEntry(baseURL, token string) map[string]any

// devin_process.go
type devinDispatcher struct {
    w           io.Writer
    writeMu     sync.Mutex
    mu          sync.Mutex
    nextID      int64
    waiters     map[int64]chan devinRPCResponse
    sessionSubs map[string]chan devinRPCNotification
    inbound     func(devinInboundRequest)
}

func (d *devinDispatcher) Call(ctx context.Context, method string, params any) (map[string]any, error)
func (d *devinDispatcher) Notify(method string, params any) error
func (d *devinDispatcher) Fail(err error)

type devinProcessHandle struct {
    cmd            *exec.Cmd
    stdin          io.WriteCloser
    dispatcher     *devinDispatcher
    scopeKey       string
    cwd            string
    env            map[string]string
    model          string
    permissionMode string // maps to DEVIN_PERMISSION_MODE env at launch (auto/accept-edits/smart/dangerous)
    authed         bool   // true after successful ACP authenticate (F-3)
}

func (r *Runner) ensureDevinProcess(ctx context.Context, scopeKey, cwd string, extraEnv map[string]string, model string, permissionMode string) (*devinProcessHandle, error)
func (r *Runner) CloseProcessesForChildRun(runID string)
func devinBinaryName() string
func devinAgentEnabled() bool
func redactDevinFrameForLog(frame map[string]any) string
```

---

## 7. Out of Scope

- `ProviderRuntimeAdapter` implementation (`SendTurn`, `Capabilities`) — Task-401.
- Event mapper into `ProviderEvent` (`message_delta`, `tool_started`, etc.) — Task-401.
- Desktop Settings UI and CLI installation — Task-402.
- Desktop Chat / Flow Mode UI parity and Approval Gate wiring — Task-403.
- Modifying existing provider ACP files — strictly forbidden.

---

## 8. Completion Notes

- result: pending implementation
- implementation notes:
- verification:
- follow-ups: Unblocks Task-401
- upstream docs updated: `CP-70` Work Breakdown `P-1`, `P-2`
