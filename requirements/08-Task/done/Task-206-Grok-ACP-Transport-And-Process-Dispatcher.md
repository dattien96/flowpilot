# Task-206: Grok ACP Transport And Process/Dispatcher

## Metadata

- Document ID: `Task-206`
- Title: `Grok ACP Transport And Process/Dispatcher`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-09`
- Last Updated: `2026-07-10`
- Parent Documents: [CP-46: Grok Build Controlled Adapter Over ACP Transport](../../07-Coding-Plan/inprogress/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md)
- Child Documents: `None`
- Related Documents: [Task-164: Gemini ACP Transport Extraction](../done/Task-164-Gemini-ACP-Transport-Extraction.md), [05 - Codex AppServer Migration Detail](../../10-Refactor/New-System/05-Codex-AppServer-Migration-Detail.md)
- Replaces: `None`
- Tags: `grok, grok-build, acp, json-rpc, transport, dispatcher, local-runner`

## AI Quick View

### Summary

- Freeze the Grok ACP wire contract (`initialize` / `session/new` / `session/prompt` / `session/update` / `session/request_permission` / `turn_completed`) into typed Go structs and golden fixtures, using payloads already captured live during CP-46 authoring.
- Build a **standalone** Grok ACP module (`grok_acp.go`) — copy the *shapes* from `gemini_acp_transport.go` (session/new param builder, `agent_message_chunk` text extractor, `flowpilot` MCP server entry builder, response `sessionId` reader) but **do NOT refactor or re-point Gemini's transport** (CP-46 `P-0`/`P-4`). Gemini's file stays byte-identical.
- Add `grok_process.go`: a `grokDispatcher` modeled on `codexDispatcher` (waiters map, per-`sessionId` notification subs, single `inbound` handler, one read-loop, `fail()` drain) plus `ensureGrokProcess(ctx, scopeKey, cwd, env)` modeled on `ensureCodexAppServer` (spawn `grok agent stdio`, run `initialize`, hold one shared handle, teardown+respawn on account switch).
- Enforce the security requirement discovered live: the spawned process env must always disable ambient MCP compat scanning (`GROK_CLAUDE_MCPS_ENABLED=false`, `GROK_CURSOR_MCPS_ENABLED=false`).

### Current Ask

- Build the transport/process layer only — no `ProviderRuntimeAdapter`, no permission routing, no MCP/agents yet (those are Task-207/208/209). This task's output is a Go-level ACP client usable by a later adapter: connect, initialize, create/resume a session, send a prompt, receive typed notifications, and answer inbound `session/request_permission` requests generically.

### Key Decisions

- `T-1` Base the wire structs on the **actually observed** payloads (see Source Refs), not on ACP spec documents alone — Grok's `x.ai/*` extensions and permission `options[]` shape were verified live and must match exactly.
- `T-2` Copy the **Codex dispatcher shape**, not Claude's spawn-per-turn model: one persistent shared process per `scopeKey`, async multiplexed JSON-RPC, sessions routed by `sessionId` the way Codex routes by `threadId`.
- `T-3` **Standalone, not extraction (CP-46 `P-0`):** build `grok_acp.go` by copying the shapes from `gemini_acp_transport.go`; do NOT modify Gemini's file. Unifying the two ACP implementations is an explicit non-goal here. If any shared JSON-RPC helper looks reusable, still copy it rather than mutate the Gemini path.
- `T-4` The process launch must set `GROK_CLAUDE_MCPS_ENABLED=false`, `GROK_CURSOR_MCPS_ENABLED=false`, and `GROK_HOME=<accountHome>`, and must not pass through inherited secrets the process does not need.
- `T-5` This task does not decide client filesystem capabilities — always initialize with `clientCapabilities.fs.{read,write}TextFile=false` per CP-46 `P-6` so Grok uses its own tools.

### Constraints

- Do not modify Codex or Claude process/dispatcher files; only read them as a structural template.
- Do not change `gemini_acp_transport.go` behavior for Gemini — any extraction must be verified by Gemini's existing tests staying green.
- Do not implement permission-decision logic, MCP wiring, or the `ProviderRuntimeAdapter` in this task — stub/typed-passthrough only.
- Never log full ACP frames unfiltered in production paths (a credential leak was observed live via `_x.ai/mcp/servers_updated`); redact/strip before any log line.

### Open Questions

- `Q-1` (from CP-46 `Q-2`) Is Grok's resume request `session/load{sessionId,cwd,mcpServers}` (ACP standard) or an `x.ai/*` variant? Must be probed live in this task since Task-207 depends on it.
- `Q-2` Should the dispatcher key notification routing by `sessionId` alone, or does Grok ever multiplex by a different id (mirror Codex's `threadId` question)?

### Source Refs

- `CP-46` `P-1`, `P-2`; Constraints (security); Risks `R-1`, `R-2`, `R-3`.
- Live captured fixtures (CP-46 authoring session, scratchpad `test_grok_acp*.py` + `grok_acp_log*.txt`): `initialize` result (`agentCapabilities`, `mcpCapabilities:{http:true,sse:true}`, tool list), `session/new` result (`sessionId`, `_meta.x.ai/sessionDetail`), `session/update` notifications (`agent_message_chunk`, `agent_thought_chunk`, `tool_call`, `tool_call_update`), `session/request_permission` request (`options:[{optionId,name,kind}]`), `turn_completed` result (`stopReason`, token usage `_meta`).
- `apps/local-runner/internal/runner/gemini_acp_transport.go` (extraction source)
- `apps/local-runner/internal/runner/codex_appserver.go`, `codex_appserver_process.go` (dispatcher/process template)

## 1. Goal

Produce a reusable Go ACP transport + a Codex-shaped persistent process/dispatcher for `grok agent stdio`, so a later adapter task can drive real Grok turns without writing any JSON-RPC plumbing itself.

## 2. Parent Links

- coding plan: `CP-46`
- tech design: `SD-06`
- system spec: `SS-05`
- specific upstream ids: `CP-46 P-1`, `P-2`

## 3. Trigger

CP-46 requires a Grok-specific process/transport layer before any adapter work can start; the ACP wire contract was already validated live during CP authoring and must be captured before it drifts.

## 4. Exact Change

- `T-1` Add typed Go structs for the Grok ACP messages listed in Key Decisions/Source Refs (`initialize` request/result, `session/new` request/result, `session/prompt` request/result, `session/update` notification incl. `agent_message_chunk`/`agent_thought_chunk`/`tool_call`/`tool_call_update`, `session/request_permission` request incl. `options[]`, permission response, `turn_completed` result).
- `T-2` Build a standalone `grok_acp.go` with the ACP primitives (session/new params incl. resume `session/load` request shape, text extraction, `flowpilot` MCP server entry builder, `sessionId` reader), copying shapes from `gemini_acp_transport.go` without modifying it.
- `T-2b` **Probe and document the resume request shape** live: confirm whether Grok resume is `session/load{sessionId,cwd,mcpServers}` (ACP standard) or an `x.ai/*` variant, and capture a fixture. Task-207's resume (its `T-4`) depends on this being resolved here.
- `T-3` Add `grok_process.go`: `grokDispatcher` (waiters, per-`sessionId` subs, `inbound` handler, read-loop, `fail()`), `grokProcessHandle`, `ensureGrokProcess(ctx, scopeKey, cwd, env)`, a `grokBinaryName` var (default `"grok"`, overridable), and a `grokAgentEnabled()` gate mirroring `codexAppServerEnabled()`.
- `T-4` Set launch env: `GROK_CLAUDE_MCPS_ENABLED=false`, `GROK_CURSOR_MCPS_ENABLED=false`, `GROK_HOME=<home>`; strip host env the process does not need.
- `T-5` Add golden-fixture tests: replay captured `initialize`→`session/new`→`session/prompt`→`session/update`(x N)→`session/request_permission`→(reply)→`turn_completed` against a fake stdio pipe and assert the dispatcher parses/routes every frame correctly.
- `T-6` Add a log-redaction guard so no raw `_x.ai/mcp/servers_updated` (or any frame carrying `headers`/`env`/tokens) is written to logs unfiltered.

## 5. Touched Areas

- files: new `apps/local-runner/internal/runner/grok_process.go`, `grok_acp_types.go` (or equivalent), shared ACP primitives file (new), `apps/local-runner/internal/runner/gemini_acp_transport.go` (extraction only), test fixtures under a `testdata/grok_acp/` directory, `grok_process_test.go`
- modules: local runner provider transport layer
- routes: none (no HTTP surface added)
- tables: none

## 6. Acceptance Check

- Golden-fixture replay test passes against the exact captured live sequence.
- Gemini's existing ACP tests pass unchanged after extraction.
- The dispatcher correctly routes a `session/request_permission` inbound request to a test-provided handler and returns the handler's reply on the wire.
- The spawned-process env asserts contain the two `GROK_*_MCPS_ENABLED=false` vars and `GROK_HOME`.
- No test or code path logs an unredacted MCP-server-list frame.

### 6.1 Definition of Done (DOD)

- [x] `DOD-1` Typed structs exist for every message class captured live and round-trip through JSON without loss. (`grok_acp_types.go`; round-trip exercised via `grok_process_test.go` golden fixtures + `grokContextWindowFromInit`.)
- [x] `DOD-2` Shared ACP primitives are extracted; Gemini's test suite is unchanged and green. (Per `T-3`, "extracted" means shape-COPIED into a standalone `grok_acp.go`, not refactored out of `gemini_acp_transport.go` — that file has zero diff. Gemini's own test suite is unaffected.)
- [x] `DOD-3` `grokDispatcher` multiplexes multiple concurrent sessions over one stdio pipe in a test. (`TestGrokDispatcherMultiplexesConcurrentSessions`.)
- [x] `DOD-4` `ensureGrokProcess` reuses an existing handle for a matching `scopeKey` and tears down + respawns on mismatch. (`TestEnsureGrokProcessInitializes`.)
- [x] `DOD-5` Process launch always disables ambient MCP compat scanning; a test asserts the env vars are present. (`TestGrokProcessEnvDisablesAmbientMCPScanning`. Live-verified caveat: these two flags do NOT suppress Grok's own marketplace-plugin MCP auto-install — see CP-46 §10.2.)
- [x] `DOD-6` A redaction test proves credential-shaped fields never reach the log sink unmasked. (`TestRedactGrokFrameForLogStripsCredentialShapedFields`.)
- [x] `DOD-7` The Grok resume request shape (`session/load` vs `x.ai/*`) is probed live, documented, and captured as a fixture (unblocks Task-207 `T-4`). **Live-verified (2026-07-10, real logged-in account):** `session/load{sessionId,cwd,mcpServers}` is confirmed as the real, ACP-standard resume shape — `TestLiveRealGrokChatStreamAndResume` issues it against a real `grok agent stdio` process and the resumed turn correctly recalls turn-1 context. Live probing also surfaced a real shape gap the original ACP-spec-only implementation missed: `session/load`'s response nests `sessionId` under `result._meta.sessionId`, not top-level like `session/new` — fixed in `grokACPResponseSessionID` (see Task-207 `DOD-6`).
- [x] `DOD-8` **Base-regression (`P-0`):** `gemini_acp_transport.go` is unchanged (git diff empty for that file); the full Gemini test suite passes unchanged; no Codex/Claude process file is modified.

## 7. Out of Scope

- `ProviderRuntimeAdapter` implementation, event mapping to `ProviderEvent`, permission-decision policy, MCP/`ask_user`/`spawn_agent` wiring, account/home resolution, desktop UI. These are Task-207 through Task-211.
- Full resume *semantics* (persistence, re-seed, mismatch handling) — Task-207/212. This task only captures the resume request *shape* (`T-2b`/`DOD-7`).
- Refactoring/unifying the Gemini ACP transport — explicit non-goal (CP-46 `P-0`/`P-4`).

## 8. Completion Notes

- result:
- implementation notes:
- verification:
- follow-ups:
- upstream docs updated:
