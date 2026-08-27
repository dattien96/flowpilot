# Task-300: Opencode ACP Transport And Process/Dispatcher

## Metadata

- Document ID: `Task-300`
- Title: `Opencode ACP Transport And Process/Dispatcher`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-27`
- Last Updated: `2026-08-27`
- Parent Documents: [CP-57: Opencode Provider Integration](../../07-Coding-Plan/todo/CP-57-Opencode-Provider-Integration.md)
- Child Documents: `None`
- Related Documents: [Task-206: Grok ACP Transport And Process/Dispatcher](../done/Task-206-Grok-ACP-Transport-And-Process-Dispatcher.md), [Task-164: Gemini ACP Transport Extraction](../done/Task-164-Gemini-ACP-Transport-Extraction.md)
- Replaces: `None`
- Tags: `opencode, acp, json-rpc, transport, dispatcher, local-runner`

## AI Quick View

### Summary

- Freeze the Opencode **ACP** wire contract (`initialize` / `session/new` / `session/prompt` / `session/update` / permission `question`/`plan_*` / `turn_completed`) into typed Go structs and golden fixtures using a **live `opencode acp` capture** on `1.18.23` (`opencode acp --help`, `opencode --help` `acp`/`mcp`/`models`, `~/.config/opencode/opencode.json` `permission` array). `opencode run --format json --print-logs` (`ses_*` + `ProviderModelNotFoundError` + `step_finish.tokens`) is **one-shot only** for contrast/summarizer, **not** ACP — do not freeze its `step_finish` as an ACP message.
- Build a **standalone** `opencode_acp.go` + `opencode_process.go` — copy shapes from `grok_acp.go`/`codex_appserver.go` but do **NOT** refactor `gemini_acp_transport.go` / `grok_acp.go` (CP-57 `P-0`/`P-4`).
- Add `opencodeDispatcher` (waiters map, per-`sessionId` subs, single inbound handler, one read-loop, `fail()` drain) + `ensureOpencodeProcess(ctx, scopeKey, cwd, env, model, variant, auto)` (spawn `opencode acp`, `initialize`, hold one shared handle per `scopeKey`, teardown on account switch), with env `GROK_CLAUDE_MCPS_ENABLED` analog `OPENCODE_*` isolation and log redaction. **Probe** whether `model/variant/auto` are ACP launch flags vs per-session params to decide `newAdapter` vs `newAdapterForTurn` for Task-301.

### Current Ask

- Build the transport/process layer only — no `ProviderRuntimeAdapter`, no permission decision policy, no MCP/agents yet. Output is a Go-level ACP client usable by Task-301: connect, initialize, create/resume session, send prompt, receive typed notifications, answer inbound permission requests generically.

### Key Decisions

- `T-1` Base structs on the **actually observed `opencode acp` `1.18.23` payloads** (see Source Refs), not ACP spec alone and **not** `opencode run --format json` — Opencode's `permission: [{permission:"question",pattern:"*",action:"deny"}]` was verified from `opencode.json`; `step_finish.tokens{total,input,output,reasoning,cache}` is **one-shot** `run --format json` evidence for contrast only, not an ACP `session/update`.
- `T-2` Copy the **Codex/Grok dispatcher shape** (one persistent shared process per `scopeKey`, async multiplexed JSON-RPC, sessions routed by `sessionId` like Codex `threadId`), not Claude's spawn-per-turn model.
- `T-3` Standalone, not extraction: build `opencode_acp.go` by copying shapes from `grok_acp.go`; do NOT touch `gemini_acp_transport.go` / `grok_acp.go` (CP-57 `P-0` / Work Breakdown `P-2`).
- `T-4` Always launch with `OPENCODE_*` isolation (strip inherited secrets, set `HOME`/`OPENCODE_CONFIG` per managed account analogous to `GROK_HOME`).

### Constraints

- Do not modify `gemini_acp_transport.go`, `grok_acp.go`, `codex_appserver.go`, `claude_process.go`; read as template only.
- Do not implement permission-decision logic, MCP wiring, or `ProviderRuntimeAdapter` in this task — stub/typed-passthrough only.
- Never log full ACP frames unfiltered (credential leak observed for Grok `_x.ai/mcp/servers_updated`; Opencode may echo `mcpServers` headers) — redact before log.
- `gemini_acp_transport.go` must stay byte-identical — verified by `git diff` empty + Gemini test suite green.

### Open Questions

- `Q-1` Is Opencode resume `session/load{sessionId,cwd,mcpServers}` (ACP standard) or an `x.ai/*`-like extension? Must be probed live in this task (Task-301 depends on it).
- `Q-2` Does Opencode multiplex notifications by `sessionId` only, or by another id?

### Source Refs

- `CP-57` Work Breakdown `P-1`, `P-2`; Key Decisions `P-1`, `P-3`, `P-4`; Constraints; Risks `R-1`, `R-6`.
- Live capture `2026-08-27`: `opencode acp --help` (ACP server, **must be captured live in this task** for `initialize`/`session/new`/`session/prompt`/`session/update`/`session/request_permission`), `opencode --help` (`acp`, `mcp`, `run --format json --variant`, `models`, `providers`, `stats`), `opencode run --format json --print-logs --model opencode/muse-spark-1.2-contributor-free` as **one-shot contrast** (`{"type":"step_start"}→{"type":"text","text":"ok"}→{"type":"step_finish","reason":"stop","tokens":{...}}`, `ProviderModelNotFoundError: opencode/deepseek-v4-flash-free Did you mean: ...`), `~/.config/opencode/opencode.json` (`model`, `agent.build.model`, `permission` array).
- `apps/local-runner/internal/runner/grok_acp.go`, `grok_process.go`, `codex_appserver.go` (dispatcher template), `gemini_acp_transport.go` (shape source)

## 1. Goal

Produce a reusable Go ACP transport + a Codex/Grok-shaped persistent process/dispatcher for `opencode acp`, so Task-301 can drive real Opencode turns without writing JSON-RPC plumbing.

## 2. Parent Links

- coding plan: `CP-57` Work Breakdown `P-1`, `P-2` (Key Decisions `P-1`, `P-3`, `P-4`)
- tech design: `SD-06`, `SD-11`
- system spec: `SS-05`
- specific upstream ids: `CP-57 Work Breakdown P-1` (freeze ACP contract), `P-2` (transport/process)

## 3. Trigger

CP-57 requires a frozen wire contract + process layer before any adapter work can start; the `1.18.23` live capture must be captured before it drifts.

## 4. Exact Change

- `T-1` `apps/local-runner/internal/runner/opencode_acp_types.go` (new) — **copy `grok_acp_types.go` / `grok_acp.go` type shapes** (Task-206 template). Current: no opencode types; `grok_acp_types.go` defines `initialize` request/result (`agentCapabilities`, `mcpCapabilities:{http,sse}`, tool list, `reasoningEfforts`, `totalContextTokens`), `session/new` result `sessionId` + `_meta.x.ai/sessionDetail`, `session/update` notification `agent_message_chunk`/`agent_thought_chunk`/`tool_call`/`tool_call_update`, `session/request_permission` `options[]`, `turn_completed` `_meta` tokens. Delta: add opencode-named structs for every **ACP** class observed live in `T-2b/T-2c`: `initialize` request/result (`agentCapabilities`, `mcpCapabilities`, tool list, `promptCapabilities.image`), `session/new` request/result (`cwd`, `mcpServers:[flowpilot,google-drive,jira]`, `permission:[{permission,pattern,action}]`, `model:opencode/<id>`, `variant`), `session/prompt` request/result (`sessionId`, `prompt:promptPrep(req)`, `attachments` if proven), `session/update` notification (`text`/`tool_call`/`tool_call_update` with `path`/`newText` diff, `plan` analog if emitted), inbound `question`/`permission` request (`permission`, `pattern`, `options[]`, `questionId`), permission response (`outcome`/`optionId` — must echo exact optionIds Opencode offers), `turn_completed` result (`sessionId`, `stopReason`, `cost`, fallback `_meta.sessionId` → Task-207 DOD-6 bug: `session/load` returns `sessionId` under `result._meta.sessionId` not top-level). **Do not** freeze `step_finish` as ACP — `step_finish.tokens{total,input,output,reasoning,cache}` is one-shot `run --format json` contrast only (summarizer, CP-57 P-1).
- `T-2` `apps/local-runner/internal/runner/opencode_acp.go` (new) — **copy `grok_acp.go` primitive shapes, do NOT refactor `gemini_acp_transport.go` (CP-57 P-0)**. Current: `gemini_acp_transport.go` has `geminiACPSessionNewParams`, `geminiACPFlowPilotMCPServerEntry`, `geminiACPResponseSessionID` (`_meta` fallback already), `geminiACPExtractText` (`agent_message_chunk`). Delta: implement `opencodeACPSessionNewParams(cwd string, mcpServers map[string]any, permission []any, model, variant string) map[string]any`, `opencodeACPSessionLoadParams(sessionId, cwd string, mcpServers map[string]any) map[string]any`, `opencodeACPPromptParams(sessionId, prompt string) map[string]any`, `opencodeACPExtractText(update map[string]any) string` (handles `text`/`tool_call` shapes Opencode actually emits), `opencodeACPFlowPilotMCPServerEntry(baseURL, token string) map[string]any` (reuse `flowpilotClaudeExtraMCPServers` provider-neutral helper if `mcpServers` HTTP), `opencodeACPResponseSessionID(result map[string]any) string` (check `result["sessionId"]` then `result["_meta"].(map[string]any)["sessionId"]` like `grokACPResponseSessionID`), `opencodeACPIsPermissionRequest(method string) bool` (match `session/request_permission` / `question` / `permission` as observed in T-2b). Unit: `TestOpencodeACPSessionNewParamsBuildsCorrectJSON` asserts `cwd`/`mcpServers`/`permission`/`model`/`variant` keys exactly.
- `T-2b` **Probe resume shape live** on `1.18.23` — Current: Q-1 open. Delta: run `opencode acp` live against logged-in zen account (`opencode auth`); issue `initialize` → `session/new{cwd,mcpServers}` → `session/prompt` → `session/load{sessionId,cwd,mcpServers}` (ACP standard) and capture request/response bytes; if Opencode uses `x.ai/*` variant, capture that shape instead. Save fixture `apps/local-runner/internal/runner/testdata/opencode_acp/resume_*.json` (request + response) and document `session/load vs variant` + `_meta.sessionId` nesting in code comment above `opencodeACPSessionLoadParams`. Unblocks Task-301 `T-4/T-5` resume.
- `T-2c` **Probe launch-flag vs per-session param** live on `1.18.23` — Current: CP-57 §5.2 row 2 deferred. Delta: capture `opencode acp --help` (check for `--model`/`--variant`/`--auto`/`--permission` launch flags vs none) and `initialize` result (`agentCapabilities`, `promptCapabilities`, `reasoningEfforts`, `totalContextTokens`). If flags exist, Task-301 must use `newAdapterForTurn(model,variant)` + `opencodeProcesses map[string]*opencodeProcessHandle` keyed `scopeKey+model+variant+auto` like `runner.go:155-175 grokProcesses map[string]*grokProcessHandle`; else `newAdapter` suffices. Document decision in `opencode_process.go` header + Task-301 wiring.
- `T-3` `apps/local-runner/internal/runner/opencode_process.go` (new) — **copy `grok_process.go:83 grokDispatcher` + `grok_process.go:475 ensureGrokProcess` / `codex_appserver.go` dispatcher** (CP-46 template). Current: `runner.go:155-175` has `grokProcessMu sync.Mutex`, `grokProcesses map[string]*grokProcessHandle` (coexist, keyed `scope+model+effort+alwaysApprove`), `grokDesiredAlwaysApprove bool`, `grokRunSessions *grokRunSessionIndex`; `grok_process.go:33-41` has `FLOWPILOT_GROK_AGENT` + `grokAgentEnabled()` + `grokBinaryName` (`GROK_BIN`). Delta: add `opencodeDispatcher struct { w io.Writer; writeMu sync.Mutex; mu sync.Mutex; nextID int64; waiters map[int64]chan grokResponse; sessionSubs map[string]chan grokNotification; inbound func(grokInboundRequest) }` with `call(req) ([]byte,error)`, `notify` routing by `sessionId`, single `readLoop` (one goroutine, `bufio.Scanner` on stdout, JSON-RPC 2.0 framing), `fail(err)` drain (close waiters + subs). Add `opencodeProcessHandle struct { cmd *exec.Cmd; stdin io.WriteCloser; dispatcher *opencodeDispatcher; scopeKey, cwd string; env map[string]string; model, variant string; auto bool }` + `opencodeProcesses map[string]*opencodeProcessHandle` on `Runner` (copy `runner.go:155-175` with `opencodeProcessMu sync.Mutex` + `opencodeProcesses` + `opencodeDesiredAuto` if needed). Implement `ensureOpencodeProcess(ctx context.Context, scopeKey, cwd string, extraEnv map[string]string, model, variant string, auto bool) (*opencodeProcessHandle,error)` — mirror `ensureGrokProcess` signature at `grok_process.go:475 func (r *Runner) ensureGrokProcess(ctx, scopeKey, cwd, extraEnv, model, reasoningEffort, alwaysApprove)`: `opencodeProcessMu.Lock/Unlock`, key=`scopeKey+"|"+model+"|"+variant+"|"+fmt.Sprint(auto)` (if launch-flags, else just scopeKey), reuse if `handle.model==model && handle.variant==variant && handle.auto==auto`, else `handle.close()` + `exec.CommandContext(ctx, opencodeBinaryName(), "acp")` with `HOME`/`OPENCODE_CONFIG` env, run `initialize` with `grokInitTimeout` analog `30s`, store in map. Add `opencodeBinaryName var` (default `"opencode"`, `FLOWPILOT_OPENCODE_BIN` override) + `opencodeAgentEnabled() bool` (`FLOWPILOT_OPENCODE_AGENT=0/false/no` opts out, on by default) copying `grok_process.go:33-55`. Do NOT touch `gemini_acp_transport.go`/`grok_acp.go` (P-0); byte-identical verified by `git diff` empty.
- `T-4` `apps/local-runner/internal/runner/opencode_process.go` launch env — Current: Grok hard-sets `GROK_CLAUDE_MCPS_ENABLED=false`, `GROK_CURSOR_MCPS_ENABLED=false`, `GROK_HOME`, `HOME`, `XDG_CONFIG_HOME`, plus Windows `USERPROFILE/APPDATA/...` via `provider_registry.go:270/322/395/460`, `runner.go:1155 getEnvForExecution`. Delta: set `HOME=<accountHome>`, `XDG_CONFIG_HOME=Join(accountHome,".config")`, `OPENCODE_CONFIG=Join(accountHome,".config","opencode")`, strip host `OPENCODE_API_KEY`/`OPENCODE_HOME` beyond scope, add `OPENCODE_CLAUDE_MCPS_ENABLED=false`-like isolation if ambient discovery observed (probe in T-2c; at minimum `OPENCODE_DISABLE_AMBIENT_MCP=true` stub and document why). On Windows, mirror `windowsHomeDriveAndPath` for `USERPROFILE/APPDATA/LOCALAPPDATA/HOMEDRIVE/HOMEPATH` like Grok `provider_registry.go:460` block. Test: `TestOpencodeProcessEnvIsolates` asserts `HOME`/`OPENCODE_CONFIG` + `GROK_*` analog absent + no `OPENCODE_API_KEY` leak.
- `T-5` `apps/local-runner/internal/runner/opencode_acp_test.go` + `opencode_process_test.go` (golden-fixture) — Current: none. Delta: add `opencode_acp_fixture_test.go` helper loading `testdata/opencode_acp/*.json` (captured `initialize→session/new→session/prompt→session/update*→session/request_permission→reply→turn_completed`). Add `TestOpencodeACPTypesRoundTrip` (typed structs JSON round-trip), `TestOpencodeDispatcherMultiplexesConcurrentSessions` (2 sessions on one fake `io.Pipe` stdio, route by `sessionId`), `TestOpencodeDispatcherRoutesPermissionRequestToInboundHandler` (fake server→client `question`, handler replies, wire returns `outcome`/`optionId`), `TestOpencodeDispatcherFailDrainsWaitersAndSubs` (close), `TestEnsureOpencodeProcessReusesOrRespawns` (same scopeKey reuse, diff model respawn), `TestOpencodeProcessEnvIsolates`, `TestEnsureOpencodeProcessHonorsAgentEnabledFlag` (`FLOWPILOT_OPENCODE_AGENT=0` returns placeholder error). Keep `step_finish` fixture separate for one-shot contrast only (do NOT assert dispatcher parses it as ACP).
- `T-6` `apps/local-runner/internal/runner/opencode_process.go:368 redactGrokFrameForLog` analog — Current: `grok_process.go:368 redactedPlaceholder`, `grokProcessTest:439 placeholder`. Delta: implement `redactOpencodeFrameForLog(frame map[string]any) string` copying `redactGrokFrameForLog`: strip `headers`/`token`/`env`/`OPENCODE_API_KEY`/`mcpServers[].headers` before `log.Printf`; malformed → fixed `"[redacted]"` placeholder. Test: `TestRedactOpencodeFrameForLogStripsCredentialShapedFields` feeds frame with `headers:{Authorization:"Bearer sk-..."}` and asserts log output contains `"[redacted]"` not token (mirror `grok_process_test.go:439`).

## 5. Touched Areas

- files: new `apps/local-runner/internal/runner/opencode_acp.go`, `opencode_acp_types.go`, `opencode_process.go`, `opencode_process_test.go`, `opencode_acp_test.go`, golden fixtures `apps/local-runner/internal/runner/testdata/opencode_acp/*.json`, helper `opencode_acp_fixture_test.go`; edits: `apps/local-runner/internal/runner/runner.go` (add `opencodeProcessMu`+`opencodeProcess` fields + `opencodeDesiredAuto` if needed), `apps/local-runner/internal/runner/compat.go` (add `CompatTestedOpencodeVersion` stub only — full probe in Task-302), no edits to `gemini_acp_transport.go`/`grok_acp.go`
- modules: local runner provider transport layer
- routes: none
- tables: none

## 6. Acceptance Check

- Golden-fixture replay test passes against the exact captured `1.18.23` live sequence.
- Gemini and Grok test suites stay green after standalone `opencode_acp.go` addition (no diff in theirs).
- Dispatcher correctly routes a `question`/`permission` inbound request to a test handler and returns handler's reply on the wire.
- Spawned-process env asserts contain isolation vars and `HOME`/`OPENCODE_CONFIG`; no credential leak in logs.
- Resume shape probed live, documented, fixture captured (blocks Task-301).

### 6.1 Definition of Done (DOD)

- [ ] `DOD-1` Typed structs for every message class round-trip through JSON without loss. (`opencode_acp_types.go` + `TestOpencodeACPTypesRoundTrip`.)
- [ ] `DOD-2` Standalone ACP primitives exist; Gemini/Grok files zero diff. (`git diff -- apps/local-runner/internal/runner/gemini_acp_transport.go` empty; `go test ./internal/runner -run TestGemini` green.)
- [ ] `DOD-3` `opencodeDispatcher` multiplexes multiple concurrent sessions over one stdio pipe in a test. (`TestOpencodeDispatcherMultiplexesConcurrentSessions`.)
- [ ] `DOD-4` `ensureOpencodeProcess` reuses handle for matching `scopeKey` and respawns on mismatch. (`TestEnsureOpencodeProcessReusesOrRespawns`.)
- [ ] `DOD-5` Process launch isolates ambient discovery; test asserts env vars. (`TestOpencodeProcessEnvIsolates`.)
- [ ] `DOD-6` Redaction test proves credential-shaped fields never reach log sink unmasked. (`TestRedactOpencodeFrameForLogStripsCredentialShapedFields`.)
- [ ] `DOD-7` Resume request shape probed live, documented, fixture captured. (Live run against `opencode acp 1.18.23` with `session/load`; fixture in `testdata/opencode_acp/resume_*.json`.)
- [ ] `DOD-8` Base-regression: `gemini_acp_transport.go` unchanged, `grok_acp.go` unchanged, no Codex/Claude process file modified; `go test ./...` in `apps/local-runner` unchanged vs baseline.

### 6.2 Test Signatures

```go
// opencode_acp_types_test.go
func TestOpencodeACPTypesRoundTrip(t *testing.T)
func TestOpencodeACPResponseSessionIDHandlesMetaFallback(t *testing.T)

// opencode_acp_test.go
func TestOpencodeACPSessionNewParamsBuildsCorrectJSON(t *testing.T)
func TestOpencodeACPSessionLoadParamsBuildsCorrectJSON(t *testing.T)
func TestOpencodeACPPromptParamsBuildsCorrectJSON(t *testing.T)
func TestOpencodeACPExtractTextHandlesAllUpdateShapes(t *testing.T)
func TestRedactOpencodeFrameForLogStripsCredentialShapedFields(t *testing.T)

// opencode_process_test.go
func TestOpencodeDispatcherMultiplexesConcurrentSessions(t *testing.T)
func TestOpencodeDispatcherRoutesPermissionRequestToInboundHandler(t *testing.T)
func TestOpencodeDispatcherFailDrainsWaitersAndSubs(t *testing.T)
func TestEnsureOpencodeProcessReusesOrRespawns(t *testing.T)
func TestOpencodeProcessEnvIsolates(t *testing.T)
func TestEnsureOpencodeProcessHonorsAgentEnabledFlag(t *testing.T)
```

## 7. Out of Scope

- `ProviderRuntimeAdapter`, event mapping to `ProviderEvent`, permission-decision policy, MCP/`ask_user`/`spawn_agent`, account/home resolution, desktop UI — Task-301..303.
- Full resume semantics (persistence, re-seed, mismatch handling) — Task-301. This task only captures the request shape.
- Refactoring shared ACP transport — explicit non-goal (CP-57 `P-0`/`P-4`).

## 8. Completion Notes

- result:
- implementation notes:
- verification:
- follow-ups:
- upstream docs updated: `CP-57` Work Breakdown `P-1`, `P-2`
