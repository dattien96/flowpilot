# 07 — Claude Provider Adapter Plan (Architecture · Design · Coding)

> Companion to `05-Codex-AppServer-Migration-Detail.md`. Where `05` grounds the
> **Codex** app-server adapter in the real Go runner, this doc does the same for the
> **Claude** adapter: it answers *"does Claude have the same support as Codex
> app-server?"*, then gives the architecture, design, and a code-grounded coding plan
> to move Claude from a disabled placeholder to a working controlled-mode adapter
> behind the **same** `ProviderRuntimeAdapter` contract.
>
> Research basis: official Anthropic docs (Claude Agent SDK reference, Claude Code
> headless/CLI reference, permission-modes, sessions, MCP) cross-checked by an
> adversarially-verified research pass (June 2026, Claude Code CLI ~2.1.x). Claims
> below are tagged **[high]/[med]/[low]** confidence; every **[med]/[low]** item is a
> Phase-0 spike target (see "Validate First").

---

## TL;DR

- **Does Claude have a Codex-`app-server` equivalent? No — not a literal one.** There is
  **no** `claude app-server --listen stdio://` daemon and **no official Go Agent SDK**.
  The closest embeddable surface is the **Claude Code CLI in headless stream-json mode**
  (`claude -p --input-format stream-json --output-format stream-json --verbose`), a
  long-lived, bidirectional, newline-delimited-JSON process over stdio — which is exactly
  what the TS/Python Agent SDK wraps internally. **[high]**
- **At the single-session level, Claude is at parity** with Codex app-server: streaming,
  turn start, resume/fork, interrupt, server→client approval, MCP, and skills all have
  working equivalents. **[high]**
- **The one defining property that is MISSING:** a *single shared process multiplexing
  many cwd-scoped threads*. In Claude, **cwd is fixed per process** and there is no
  `thread/start`/`thread/list` RPC on a live shared process. **FlowPilot's Go runner must
  become the multiplexer:** run **one `claude` process per (account, cwd, session)**, and
  back `thread/list`/`thread/read` with on-disk transcript JSONL, not live RPC. **[high]**
- **Recommendation: Option A** — a pure-Go Claude adapter that drives the `claude` CLI in
  stream-json mode, reusing the existing `codex_appserver_process.go` / `codex_appserver.go`
  / `codex_event_mapper.go` patterns. No Node/Python sidecar. The provider-neutral
  `ProviderRuntimeAdapter` + `TurnBridge` + `ProviderEvent` contract is unchanged.
- **Single biggest risk to validate before any build:** the **permission-interception
  wire contract** (how a host answers a per-tool approval at runtime in headless mode).
  Prefer the **documented** `--permission-prompt-tool <mcp>` route over the **undocumented,
  version-fragile** `control_request(can_use_tool)` stdio frames.

---

## Final Decision (Canonical)

- **Provider runtime:** Claude is embedded via the **Claude Code CLI headless stream-json
  transport**, driven from Go — the same shape as the Codex app-server path, minus the
  shared-process multiplexing.
- **Process model:** **one `claude` process per (provider account, cwd, session)**. The Go
  runner owns the process pool + thread registry (the multiplexer Codex's app-server gave
  us for free). A turn either reuses the live session process (feed a new `user` line on
  stdin) or re-spawns with `--resume <session_id>` — MVP starts with the simpler
  spawn-per-turn-with-resume and may keep the process warm later.
- **Contract:** **no change.** Claude implements the existing `ProviderRuntimeAdapter`
  (`provider_registry.go:49`) and emits the existing normalized `ProviderEvent` union
  (`provider_event.go:74`). The runner core, `TurnBridge`, finalizer, retry, and client
  stream are untouched.
- **Approval:** reuse the existing `TurnBridge.RequestApproval` (`interactive_service.go:379`)
  and the YOLO SSOT. The **documented** `--permission-prompt-tool` MCP route is the
  interception mechanism; the undocumented stdio control frames are a fallback only.
- **`ask_user`:** served as a **real external MCP server** the Go runner hosts (stdio/http)
  and passes via `--mcp-config` — *not* an in-process SDK tool (those are TS/Python-only).
- **Foundation:** build on the existing live-process infra (`codex_appserver_process.go`,
  the `codexDispatcher` newline-JSON read loop) as a sibling Claude transport.
- **Enablement:** gated behind `FLOWPILOT_CLAUDE_ADAPTER` (mirrors
  `FLOWPILOT_CODEX_APPSERVER`); Claude stays a placeholder in the default registry so
  demos/tests stay green without a real `claude` binary.
- **Auth/account:** reuse the existing Claude account/auth machinery already in the runner
  (`runner.go:791`, `:1311`, `:1708`, `:1795`): `ANTHROPIC_API_KEY` / `~/.claude/auth.json`
  / `CLAUDE_CONFIG_DIR`. **Billing caveat:** for embedded/server use, authenticate with an
  **API key (or Bedrock/Vertex)**, not a claude.ai subscription login; from 2026-06-15
  `claude -p` on subscription plans draws a separate Agent-SDK credit pool. **[med]**

---

## How Claude Models It vs Codex (Capability Comparison)

> The runner's contract is provider-neutral on purpose. The table maps each
> `ProviderCapabilities` flag and each Codex **app-server primitive** to the concrete
> Claude mechanism, with confidence.

| Capability / primitive | Codex app-server | Claude mechanism | Parity | Conf |
|---|---|---|---|---|
| `streaming` | thread notifications stream deltas | stream-json stdout; token deltas via `--include-partial-messages` → `stream_event`(`content_block_delta`/`text_delta`) | **equal** | high |
| `resume` | `thread/resume` on live shared process | `--resume <id>` / `--continue` / `--session-id` / `--fork-session`; every `result` carries `session_id` | **equal (cwd-bound)** | high |
| `approvalEvents` | inbound JSON-RPC request → reply `{decision}` | `--permission-mode default` + `--permission-prompt-tool <mcp>` (documented); or SDK `canUseTool` / `control_request(can_use_tool)` (undoc.) | **equal cap / med wire** | med |
| `fileEvents` | `file.changed` / `fileChange` items | **derived** — synthesize from `Edit`/`Write`/`MultiEdit`/`NotebookEdit` `tool_use` inputs (or PostToolUse hook). No native per-edit event | **equal (derived)** | high |
| `skillSelection` | skill on `turn/start` | Agent Skills (`.claude/skills/<n>/SKILL.md`) work in `-p`; invoke by prefixing the prompt with `/skill-name` and/or injecting content (mirror `injectSkillContent`) | **equal** | high |
| `mcp` | `mcpServers` on `thread/start` (incl. in-adapter `ask_user`) | external MCP via `--mcp-config` (stdio/http/sse), `--strict-mcp-config`; tool names `mcp__<srv>__<tool>` | **equal** | high |
| `interrupt` | `turn/interrupt` notification | SDK `interrupt()` (streaming); CLI: ctx-cancel / close stdin / signal (matches the runner's existing ctx-cancel interrupt) | **equal SDK / partial CLI** | med |
| **shared-process multiplexing** | one process hosts many cwd threads | **MISSING** — cwd fixed per process; run N processes, Go host multiplexes | **missing** | high |
| `thread/start` (cwd-per-thread) | RPC on shared process | **process spawn** with that cwd (`--add-dir`/launch dir) + `--session-id` | **partial** | high |
| `thread/resume` | RPC on live process | new process with `--resume`; reads on-disk JSONL; **requires same cwd** | **partial** | high |
| `thread/list` | `thread/list {cwd}` | SDK `listSessions()` **or** enumerate `~/.claude/projects/<encoded-cwd>/*.jsonl`; not a live registry, no CLI flag | **partial (disk)** | high |
| `thread/read` | `thread/read {threadId}` | SDK `getSessionMessages()` **or** parse the `<id>.jsonl` directly | **partial (disk)** | high |
| server→client approval | inbound request id → reply | `canUseTool` / permission-prompt tool blocks until host answers — same blocking semantics as `RequestApproval` | **equal cap / med wire** | med |

**Bottom line:** Claude can satisfy **all seven** `ProviderCapabilities` flags. The
architectural cost is concentrated in **one place** — the runner, not the model, must own
multi-session multiplexing and treat list/read as disk reads.

---

## Embedding Options (and why Option A)

### Option A — Go drives the `claude` CLI in stream-json (RECOMMENDED)

Spawn `claude -p --input-format stream-json --output-format stream-json --verbose`
(`--include-partial-messages` for deltas, `--include-hook-events` for tool/file lifecycle).
**One process per (account, cwd, session)**; the Go host is the multiplexer. Feed turns as
`{"type":"user","message":{"role":"user","content":...},"parent_tool_use_id":null}` lines on
stdin; parse `system/init`, `assistant`, `user`(tool_result), `stream_event`, `result` lines
on stdout. Approvals via `--permission-prompt-tool` → a Go-served MCP tool → `RequestApproval`.
`ask_user` as an external MCP server.

- **Why:** best fit for the Go runner — no non-Go runtime; native process control matches
  `codex_appserver_process.go`; reuses the `codexDispatcher` newline-JSON read-loop shape;
  `--permission-prompt-tool` is documented and fits the existing MCP `ask_user` precedent.
- **Cost:** the runner owns the process pool + thread registry (the multiplexing Codex got
  for free); file/exit-code events are derived; stream-json is loosely versioned (pin a CLI
  version, centralize parsing).

### Option B — Node/Python Agent-SDK sidecar

A small sidecar uses `@anthropic-ai/claude-agent-sdk` (or Python) for in-process `canUseTool`,
`interrupt()`, the typed `SDKMessage` stream, and `createSdkMcpServer()`+`tool()` for
`ask_user`, re-exported to Go over a protocol you define. One sidecar can host many concurrent
`query()` sessions → re-creates shared multiplexing and gives the cleanest, fully-documented
approval primitive. **Cost:** adds a Node/Python runtime + IPC hop + a second version-pin
(SDK *and* its bundled CLI), and inverts the runner-drives-subprocess architecture. Keep as a
**future upgrade** if the CLI permission contract proves too fragile.

### Option C — Raw Messages API via `anthropic-sdk-go` + hand-rolled loop

Pure Go and official, but you re-implement the agent loop, built-in tools, skills, sandboxing,
session persistence, and the entire permission/ask_user UX. **Not recommended** — defeats the
point of provider parity. Listed for completeness.

**Decision: Option A for the MVP**, with Option B documented as the escape hatch.

---

## Claude Code Agent Engine via stream-json (How It Works)

> **Mental-model correction.** The transport in this plan is **Claude Code's agent
> engine** — the same core behind the interactive `claude` terminal client, the
> VS Code/JetBrains extensions, and the Agent SDK — **not** the consumer "Claude"
> desktop chat app. Those are all **front-ends over one engine**; stream-json is simply
> its *programmatic* front-end. So a headless host can do **everything a native client
> does** (stream a clean final answer, catch every tool call, gate dangerous commands,
> edit files, use MCP/skills) — it just renders the UI itself instead of using the
> engine's built-in terminal UI. That UI ownership is exactly FlowPilot's controlled
> mode.

### The engine loop

A `claude -p --input-format stream-json --output-format stream-json` process **is** the
agent engine. Per turn it:

1. reads a **user-turn JSON line** on stdin (`{"type":"user","message":{...}}`);
2. runs the agentic loop — think → call a tool → observe → repeat;
3. **before any gated tool runs**, asks the host for permission (this is the
   dangerous-command gate) and **blocks** until the host answers allow/deny;
4. streams JSON event lines on stdout throughout (`system/init`, `assistant`,
   `user`/tool_result, `stream_event` deltas);
5. ends the turn with a single `result` line carrying the **clean final answer** +
   `session_id` + usage.

FlowPilot owns both ends: it writes the user turn in, maps every stdout line to a
normalized `ProviderEvent`, and answers the permission/question callbacks. The engine
never renders UI — FlowPilot's desktop client does.

### Where it sits in the system (layer diagram)

```text
┌──────────────────────────────────────────────────────────────────────────┐
│ Desktop Client (Electron)            Admin Web (config / audit)            │  render only
│  chat • approval card • ask_user card • file links                         │
└───────────────▲────────────────────────────────▲──────────────────────────┘
                │ normalized event stream          │ HTTP
                │ (ProviderEvent + monotonic seq)   │
┌───────────────┴────────────────────────────────┴───────────────────────────┐
│ GO RUNNER  (single backend — owns workflow logic, calls Supabase)           │
│                                                                             │
│  InteractiveService.startTurn → sendTurnWithRetry → adapter.SendTurn        │
│  TurnBridge { Emit · RequestApproval · AskQuestion }   ← provider-neutral    │
│  ApprovalPolicyEngine + YOLO SSOT      TurnFinalizer → artifacts/RAG/Supabase│
│        │                     ▲                                               │
│        │ SendTurn            │ RequestApproval / AskQuestion  (BLOCKS)        │
│        ▼                     │                                               │
│  ┌────────────────────────────────────────────────┐  ┌─────────────────────┐│
│  │ claudeAdapter                                    │  │ FlowPilot MCP server ││
│  │  • build args + YOLO→permission-mode             │  │  mcp__flowpilot__    ││
│  │  • claude_event_mapper: stdout line→ProviderEvent│  │    approve           ││
│  │  • claudeProcessPool (1 proc / account,cwd,sess) │  │    ask_user          ││
│  └────────┬─────────────────────────▲───────────────┘  └──────────▲──────────┘│
└───────────┼─────────────────────────┼────────────────────────────┼───────────┘
            │ stdin: user-turn JSON     │ stdout: stream-json lines  │ MCP call
            ▼                           │                            │ (permission / question)
   ┌────────────────────────────────────────────────────────────────┴─────────┐
   │ claude -p  (Claude Code agent engine) — one process per (account,cwd,sess) │
   │   think → tool_use → [gated? → MCP permission call] → observe → … → result  │
   └────────────────────────────────────────────────────────────────────────────┘
```

Relationships: clients **render only** (they consume the normalized event stream and
submit decisions); the **Go runner** owns turn lifecycle, policy/YOLO, the FlowPilot MCP
server, and the finalizer; the **claudeAdapter** is the only layer that speaks
stream-json; the **engine** is an opaque subprocess that never sees FlowPilot's UI or
Supabase. This is the same dependency rule as the Codex path (`03` Layer Dependency
Rule) — only the adapter's transport differs.

### Turn + approval interaction (sequence)

```text
Client → Runner:    send turn (stepId, prompt, yolo)
Runner(adapter) → claude:   write {"type":"user", ...} on stdin
claude → Runner:    system/init (session_id) ............. → Emit turn_started
claude → Runner:    stream_event text deltas ............. → Emit message_delta (live)
claude wants Bash "rm -rf …" (a gated/dangerous command):
   claude → FlowPilot MCP (mcp__flowpilot__approve){tool,input}   [engine BLOCKS]
   MCP handler → TurnBridge.RequestApproval(details)
        YOLO=true  → auto-approve (audited: gating-disabled)
        YOLO=false → Emit permission_required → client shows card → user decides
   decision → MCP handler returns allow/deny → claude continues
              (DENY blocks the command; the reason is fed back to the model)
claude → Runner:    Edit tool_use (file_path) ............ → derive Emit file_changed
claude → Runner:    result {subtype:success, result, session_id, usage} → Emit turn_completed
Runner:             TurnFinalizer → artifacts / summary / Supabase RAG
```

**ask_user** follows the same shape: the model calls `mcp__flowpilot__ask_user`, the
handler routes to `TurnBridge.AskQuestion`, the client shows the options card, and the
answer returns as the tool result — so structured questions work for Claude exactly as
for Codex.

---

## Architecture

### Where it plugs in (unchanged seams)

The provider abstraction already exists and Claude is already a registered placeholder. The
adapter is the *only* new moving part; the orchestration is reused verbatim:

```text
Desktop / Admin Web ──HTTP/WS──> Go Runner (InteractiveService)
                                   │  startTurn → sendTurnWithRetry → adapter.SendTurn  (interactive_service.go:630)
                                   │  TurnBridge{ Emit / RequestApproval / AskQuestion } (interactive_service.go:363)  ← reused as-is
                                   │  finishTurn → TurnFinalizer                          (interactive_service.go:661)  ← reused
                                   ▼
                         ProviderRuntimeGateway (registry)  (provider_registry.go)
                                   ├── codex  → codexAdapter        (shared app-server, 1 process, N threads)
                                   ├── claude → claudeAdapter  ★NEW (process pool, 1 process per session/cwd)
                                   └── gemini → placeholder
```

### The one real architectural delta: a process-per-thread pool

Codex: **1 shared `codex app-server`** + `codexDispatcher` routes by `threadId`.
Claude: **N `claude` processes**, one per `(account, cwd, session)`. Introduce a
`claudeProcessPool` owned by the `Runner` (sibling to `r.codexAppServer`,
`runner.go:121`) that:

- keys live processes by `(scopeKey=accountID, cwd, sessionID)`;
- spawns `claude -p …` on demand, wires a **per-process** newline-JSON reader (the
  `codexDispatcher` read-loop shape, simplified: one process = one logical thread, so routing
  is trivial — no `threadSubs` map needed);
- enforces lifecycle/backpressure/idle-TTL and a max-process cap;
- on process death, drains the turn with a recoverable error (same guarantee as
  `codexDispatcher.fail`).

`thread/list` / `thread/read` become **disk reads** of `~/.claude/projects/<encoded-cwd>/*.jsonl`
keyed by `(cwd, session_id)` — not RPCs. (Encoded-cwd path scheme is a Phase-0 confirm.)

### Layering (Go-only, no sidecar)

```text
claude_adapter.go        implements ProviderRuntimeAdapter (Key/Capabilities/SendTurn)
claude_process.go        spawn/reuse claude -p; stdio pipes; per-process reader; pool + registry
claude_stream.go         newline-JSON read loop (codexDispatcher-shaped); stdin writer mutex
claude_event_mapper.go   stream-json line → normalized ProviderEvent (mirror codex_event_mapper.go)
claude_permission_mcp.go FlowPilot MCP server: approve + ask_user tools → TurnBridge
yolo_resolver.go (edit)  add ClaudePermissionMode to YoloPosture (SSOT stays one function)
provider_registry.go (edit) ProviderRegistryFor: add FLOWPILOT_CLAUDE_ADAPTER-gated claude branch
```

---

## Design

### 1. Adapter contract mapping (no contract change)

```go
type claudeAdapter struct {
    pool       *claudeProcessPool
    cwd        string            // default; req.Cwd is authoritative (04-06)
    mcpConfig  string            // path/JSON for --mcp-config (approve + ask_user)
    promptPrep func(TurnRequest) string // injectSkillContent + ask_user reinforcement (mirror codexAdapter.promptPrep)
}

func (a *claudeAdapter) Key() ProviderKey { return ProviderKeyClaude }
func (a *claudeAdapter) Capabilities() ProviderCapabilities {
    return ProviderCapabilities{ Streaming:true, Resume:true, ApprovalEvents:true,
        FileEvents:true, SkillSelection:true, Mcp:true, Interrupt:true }
}
func (a *claudeAdapter) SendTurn(ctx, req TurnRequest, bridge TurnBridge) error
```

`SendTurn` flow (mirrors `codexAdapter.SendTurn`, `codex_adapter.go:90`):
1. resolve cwd (`req.Cwd` wins), resolve session: first turn → mint/record `session_id`;
   subsequent → `--resume <session_id>` (same cwd).
2. resolve YOLO → CLI flags (below); build args incl. `--mcp-config`,
   `--permission-prompt-tool mcp__flowpilot__approve`, skill flags.
3. `pool.acquire(scopeKey, cwd, sessionID)` → process + reader.
4. write the `user` turn line (prompt = `a.promptPrep(req)`); `parent_tool_use_id:null`.
5. pump reader lines through `mapClaudeLine` → `bridge.Emit`; the permission MCP tool
   handler (separate goroutine) calls `bridge.RequestApproval` / `bridge.AskQuestion`.
6. terminate on the `result` line (`turn_completed`/`turn_failed`); honor `ctx.Done()` by
   stopping input / signalling the process (interrupt).

### 2. YOLO SSOT → Claude permission flags

Extend `YoloPosture` (`yolo_resolver.go:20`) with a Claude field; keep `resolveYoloPosture`
the single mapping (`RunnerAutoApprove` already neutral):

```go
type YoloPosture struct {
    CodexSandbox      string
    CodexApprovalMode string
    ClaudePermissionMode string // NEW: "bypassPermissions" (yolo) | "default" (gated)
    RunnerAutoApprove bool
}
// yolo=true  → ClaudePermissionMode "bypassPermissions" (+ optional allowedTools floor), RunnerAutoApprove=true
// yolo=false → ClaudePermissionMode "default" + --permission-prompt-tool, RunnerAutoApprove=false
```

| YOLO | Claude CLI | Runner bridge |
|---|---|---|
| **true** | `--permission-mode bypassPermissions` (no prompts); audit gating-disabled | auto-approve any stray request (existing `RequestApproval` YOLO branch, `interactive_service.go:385`) |
| **false** | `--permission-mode default` + `--permission-prompt-tool mcp__flowpilot__approve` | surface `permission_required` card; **deny blocks** the tool + reason fed back to model |

A **hard-deny floor** (deny rules / `--disallowedTools`) survives even bypass — use it for an
always-blocked set regardless of YOLO. **[med]** (confirm rule precedence in Phase 0.)

### 3. Event mapping (`claude_event_mapper.go`)

Centralize all wire shapes here (one-file change on schema drift), exactly like
`codex_event_mapper.go`:

| `ProviderEvent` | Claude stream-json source |
|---|---|
| `turn_started` | first line `{type:"system",subtype:"init"}` (capture `session_id`, model, cwd, tools); for warm reuse, synthesize on stdin-write / first `assistant` |
| `message_delta` | `stream_event` → `event.content_block_delta` + `delta.text_delta` → `Text` (needs `--include-partial-messages`) |
| `message_completed` | `assistant` message; concat `text` content blocks → `Text` |
| `tool_started` | `tool_use` content block (`name`→`ToolName`, `input`→`Input`); Bash = command exec |
| `tool_completed` | matching `tool_result` on the following `user` message (correlate by `tool_use` id); **exit code** via PostToolUse(Bash) hook `{stdout,stderr,exit_code}` (`--include-hook-events`) → `exit_code!=0`→`failed` (same convention as Codex) |
| `file_changed` | **derived** from `Edit`/`Write`/`MultiEdit`/`NotebookEdit` `tool_use` (`input.file_path`→`Path`, tool→`ChangeType`) |
| `permission_required` | the `--permission-prompt-tool` MCP call (or `canUseTool`) → `ApprovalDetails` → `bridge.RequestApproval` |
| `user_question_required` | model calls `mcp__flowpilot__ask_user` → handler → `bridge.AskQuestion`; (Claude's built-in `AskUserQuestion` would arrive via `canUseTool`) |
| `turn_failed` | `result` with `is_error:true` / subtype in `{error_max_turns,error_during_execution,error_max_budget_usd,…}`; `system/api_retry` = transient, not a failure |
| `turn_completed` | `result` `subtype:"success"` → `FinalMessage = result.result` (clean final text; **no transcript parsing**); carries `session_id`, `total_cost_usd`, `usage`, `num_turns` for the finalizer/audit |

Correlation/`seq`: as with Codex, the mapper leaves `ProviderTurnID` empty and runner core
stamps the canonical turn id + monotonic `seq` at `emitLocked` (`interactive_service.go:376`).

### 4. `ask_user` + permission as a Go-served MCP server (`claude_permission_mcp.go`)

In-process SDK tools are TS/Python-only, so FlowPilot serves a **real external MCP server**
(stdio or loopback http) exposing two tools, passed via `--mcp-config`:
- `mcp__flowpilot__approve` — the `--permission-prompt-tool`; receives `{tool_name, input}`,
  calls `bridge.RequestApproval`, returns the documented allow/deny payload (exact return
  shape is a **Phase-0 confirm** — historically a stringified content array).
- `mcp__flowpilot__ask_user` — receives `{prompt, options[], multiSelect?}`, calls
  `bridge.AskQuestion`, blocks, returns the choice. This mirrors `codexAskUserMcpServer()`
  (`codex_adapter.go:60`) but as an external server. Reinforce usage in the prompt
  (`askUserReinforcement`, `codex_adapter.go:47`).

> **Bridge note:** `codexAdapter.handleInbound` (`codex_adapter.go:194`) currently wires only
> **approvals**, not questions. The `ask_user`→`AskQuestion` MCP route is the analogous
> elicitation work and benefits **both** providers — factor it into a shared helper.

### 5. Sessions, resume, multi-workspace, accounts

- **Persist `(cwd, session_id)` together** in `workflow_provider_sessions` (`04-02`) — the
  table already has `working_directory`, `provider_session_id`, `provider_thread_id`,
  `provider_account_id`. For Claude: `provider_session_id` = Claude `session_id` (UUID);
  `provider_thread_id` mirrors it; **no schema change**. Resume **requires the same cwd** or a
  fresh session is silently created — store and re-supply both.
- **Account scoping:** reuse `ResolveProviderAccount("claude", …)` (used in
  `provider_registry.go:198`) for `extraEnv` (`ANTHROPIC_API_KEY` / `CLAUDE_CONFIG_DIR` /
  home). `scopeKey` = account id; switching the active account drops that account's pooled
  processes (mirrors the Codex account-switch recreate, `04-06`).
- **Skills:** map `req.SelectedSkills` → `/skill-name` prompt prefix and/or injected content
  via the existing `injectSkillContent` (`runner.go:1012`).

### 6. Registry wiring (`provider_registry.go`)

Add a `FLOWPILOT_CLAUDE_ADAPTER`-gated branch in `ProviderRegistryFor` (mirror the Codex
branch, `provider_registry.go:187`): resolve the Claude account → build `claudeAdapter` backed
by `r.claudePool`; on resolve failure return `errorAdapter{key: ProviderKeyClaude, err}` so a
turn fails typed instead of hanging. Default registry keeps Claude a `placeholderAdapter`
(`placeholder_adapters.go`). `Selectable`/`DefaultProviderKey`/`createRun` enforcement already
gate on `Status==Available` — no change.

---

## Coding Plan

### Phase 0 — Validate First (spike, gates everything)

The permission/event wire contract is the load-bearing, least-documented surface. Before any
adapter code, run a **pinned** `claude` CLI and capture real frames:

- C0-a **Permission interception:** `claude -p --input-format stream-json --output-format
  stream-json --permission-mode default --permission-prompt-tool mcp__flowpilot__approve
  --mcp-config <flowpilot.json>`. Capture: (1) exact JSON the approve tool receives,
  (2) exact return shape to allow vs deny, (3) that **deny actually blocks** the tool and the
  reason returns to the model. Decide documented-MCP vs control-frame route. **[P0]**
- C0-b **Stream schema:** capture `system/init`, `assistant`(text+tool_use), `user`(tool_result),
  `stream_event`, `result` lines verbatim; confirm `--include-partial-messages` deltas and
  whether `--include-hook-events` PostToolUse(Bash) yields `{exit_code}` on the pure-CLI stream.
- C0-c **Session/resume:** confirm `session_id` on `result`; `--resume` continuation; the
  `~/.claude/projects/<encoded-cwd>/` path scheme and the cwd-match requirement.
- C0-d **Streaming input:** confirm a second `user` line on stdin starts a new turn in the same
  process (warm reuse) vs requiring respawn+`--resume`. Record the interrupt behavior on
  ctx-cancel/close-stdin/signal.

Deliverable: a short findings note pinning the CLI version and the chosen approval route. **If
C0-a fails on the documented route, fall back to Option B (sidecar) before proceeding.**

### Phase 1 — Transport + process pool

`claude_process.go` + `claude_stream.go`: spawn `claude -p …` with stdio pipes + env
(`commandContextFn`, reuse from `codex_appserver_process.go`); per-process newline-JSON reader
(simplified `codexDispatcher`); `claudeProcessPool` keyed by `(account, cwd, session)` with
idle-TTL, max-cap, and a `fail()`-style drain on death. Add `r.claudePool` to `Runner`.

### Phase 2 — Adapter + event mapper

`claude_adapter.go` (`SendTurn` per flow above) + `claude_event_mapper.go` (table above,
centralized). `promptPrep` = `injectSkillContent` + `askUserReinforcement`. Leave
`ProviderTurnID` empty (runner stamps it).

### Phase 3 — Permission/ask_user MCP + YOLO

`claude_permission_mcp.go` (approve + ask_user tools → `TurnBridge`); extend `YoloPosture`
with `ClaudePermissionMode`; build `--mcp-config` + `--permission-prompt-tool` + permission-mode
args from the resolved posture. Factor the shared `AskQuestion` MCP route used by both providers.

### Phase 4 — Registry, capabilities, enablement

`FLOWPILOT_CLAUDE_ADAPTER`-gated Claude branch in `ProviderRegistryFor`; flip capabilities to
all-true when enabled; `errorAdapter` on resolve failure. Admin Web `/admin/providers` already
renders status+capabilities — Claude shows Available when enabled.

### Phase 5 — Sessions/resume + list/read on disk

Persist `(cwd, session_id, account)`; resume path; `thread/list`/`thread/read` as encoded-cwd
JSONL enumeration/parse. Account-switch drops pooled processes.

### Phase 6 — Hardening & docs

Failure/interrupt parity (`finishTurn` maps ctx-cancel; verify process death = recoverable
turn fail); version-pin doc; operator notes (API-key auth requirement + billing caveat; YOLO
posture). Update `04-07` placeholder status: Claude → implemented-but-gated.

---

## Tests (Go; scripted fake `claude` over in-memory pipes, mirroring `sessions_test.go:89`)

- CL-01 spawn + `system/init` parsed → `turn_started` + `session_id` captured.
- CL-05 `stream_event` deltas → `message_delta`; CL-07 final `result` → `turn_completed` with
  clean `FinalMessage` (separated from logs).
- CL-10 `tool_use`(Bash) → `tool_started`; matching `tool_result` + PostToolUse hook →
  `tool_completed` with `exit_code!=0`→`failed`.
- CL-12 `Edit`/`Write` `tool_use` → derived `file_changed` with `Path`.
- CL-20 YOLO=false: permission MCP tool call → `permission_required` → approve resumes / **deny
  blocks** + reason returned; idempotent + first-write-wins (reuses existing bridge tests).
- CL-21 YOLO=true: `bypassPermissions`, no `permission_required`, audited gating-disabled.
- CL-25 `ask_user` MCP tool → `user_question_required` → answer resumes.
- CL-30 resume: second turn with stored `(cwd, session_id)` continues; mismatched cwd is
  detected (no silent fork).
- CL-31 process death mid-turn → recoverable `turn_failed`, no hung turn (pool drain).
- CL-35 two sessions in different cwds run as two pooled processes concurrently.
- CL-40 registry: gated off → placeholder/`UnsupportedProviderRuntimeError`; gated on →
  Available + capabilities; resolve failure → typed `errorAdapter`.
- Regression: Codex tests + one-shot `ExecutePrompt` (`runner.go:791`) still pass.

---

## Definition of Done

- [ ] Phase-0 findings note: pinned CLI version + chosen approval route, with captured frames.
- [ ] `claude_process.go`/`claude_stream.go`/`claude_adapter.go`/`claude_event_mapper.go`/
      `claude_permission_mcp.go` added; `YoloPosture.ClaudePermissionMode` + registry branch wired.
- [ ] Claude implements `ProviderRuntimeAdapter`, all 7 capabilities true when enabled; no
      contract/orchestration changes (`TurnBridge`, `sendTurnWithRetry`, `finishTurn`, finalizer reused).
- [ ] YOLO SSOT drives Claude permission mode + runner bridge from one value; deny blocks; YOLO=true audited.
- [ ] `ask_user` + approve served as a Go MCP server via `--mcp-config`/`--permission-prompt-tool`.
- [ ] Sessions persist `(cwd, session_id, account)`; resume + list/read (disk) work; account-switch drops pooled processes.
- [ ] All CL-* tests pass over a scripted fake `claude`; `go vet` clean; no Codex/`ExecutePrompt` regressions.
- [ ] Live-acceptance (external, like Codex 06 Part D): one real `claude` binary run — stream,
      approve/deny round-trip, resume — behind `FLOWPILOT_CLAUDE_ADAPTER`.
- [ ] **Review gate:** AI review + human sign-off.

---

## Risks & Open Questions (verify against the pinned CLI)

- **[P0] Approval wire contract** — undocumented `control_request(can_use_tool)` regressed on
  CLI 2.1.6 (issue #469). Use the documented `--permission-prompt-tool`; confirm its exact
  request/return JSON and that **deny blocks**. *Load-bearing — validate first.*
- **[P0] No shared multiplexing** — runner must own a process pool + registry + backpressure
  (real adapter work, not config).
- **[P1] No official Go Agent SDK** (issue #498 open) — Option A hand-implements the driver;
  Option B accepts a Node/Python runtime.
- **[P1] Loosely-versioned stream-json** — pin a CLI version; centralize parsing in
  `claude_event_mapper.go`.
- **[P1] cwd-bound resume foot-gun** — mismatched cwd silently forks; always persist+re-supply
  `(session_id, cwd)`.
- **[P2] Derived file/exit events** — confirm PostToolUse hooks appear on the pure-CLI stream
  (`--include-hook-events`); the headless docs only enumerate system/assistant/user/result/stream_event.
- **[P2] `ask_user` must be external MCP** — not in-process SDK (TS/Python-only).
- **[P2] Interrupt** — no documented stdin verb; confirm ctx-cancel/close-stdin/signal cleanly
  ends the in-flight turn and matches `finishTurn`.
- **[med] Auth/billing** — embedded use needs API key (or Bedrock/Vertex), not claude.ai login;
  Agent-SDK credit pool from 2026-06-15. Confirm against the account model in `provider_accounts.go`.

---

## Appendix A — Phase-0 Spike (runnable checklist)

> Goal: pin a `claude` CLI version and capture the real wire frames the adapter + event
> mapper depend on, **before** writing adapter code. Run on the same OS the runner ships
> on. Record everything in a findings note. Materialize this as real commands/files when
> you start the spike.

### Pin the version

```bash
claude --version            # record exact version; the whole adapter is pinned to it
```

### Step 1 — Stream schema, session id, deltas (no MCP needed)

```bash
# bash
printf '%s\n' '{"type":"user","message":{"role":"user","content":"List the files here, then stop."}}' \
 | claude -p --input-format stream-json --output-format stream-json --verbose \
     --include-partial-messages --include-hook-events \
 | tee step1.jsonl
```

```powershell
# PowerShell
'{"type":"user","message":{"role":"user","content":"List the files here, then stop."}}' |
 claude -p --input-format stream-json --output-format stream-json --verbose `
   --include-partial-messages --include-hook-events |
 Tee-Object step1.jsonl
```

Capture from `step1.jsonl`:

- [ ] first line is `system`/`subtype:init`; record `session_id`, `cwd`, `model`, `tools`, `mcp_servers` keys → `turn_started` source
- [ ] `stream_event` lines with `content_block_delta`/`text_delta` → `message_delta` source
- [ ] `assistant` message shape (text vs `tool_use` blocks); a `tool_use` for a Read/Bash call → `tool_started`
- [ ] `user`/`tool_result` shape and how it correlates to the `tool_use` id → `tool_completed`
- [ ] terminal `result` line: `subtype`, `result` (clean final text), `session_id`, `usage`, `total_cost_usd` → `turn_completed`
- [ ] whether `--include-hook-events` emits a PostToolUse(Bash) frame with `{exit_code}` (the exec-status source)

### Step 2 — Permission interception (the P0 gate)

Minimal FlowPilot MCP config (`flowpilot-mcp.json`) exposing `approve` + `ask_user` over a
~30-line stdio stub MCP server that logs the request and returns a fixed decision:

```json
{ "mcpServers": { "flowpilot": { "command": "node", "args": ["./mcp-stub.js"] } } }
```

```bash
printf '%s\n' '{"type":"user","message":{"role":"user","content":"Run: rm -rf ./tmpdir"}}' \
 | claude -p --input-format stream-json --output-format stream-json --verbose \
     --permission-mode default \
     --permission-prompt-tool mcp__flowpilot__approve \
     --mcp-config ./flowpilot-mcp.json \
 | tee step2.jsonl
```

Capture / decide:

- [ ] **exact JSON the `approve` tool receives** (tool name, command, cwd, reason fields)
- [ ] **exact return shape** to ALLOW vs DENY (record verbatim — historically a stringified content array)
- [ ] DENY actually **blocks** the command and the reason is fed back to the model
- [ ] ALLOW lets it run; note which tools are gated (does Read prompt? does Bash?)
- [ ] `--permission-mode bypassPermissions` → no permission call at all (the YOLO=true path)
- [ ] a deny-rule floor (`--disallowedTools`) still blocks under bypass

### Step 3 — Resume + cwd binding

- [ ] re-run with `--resume <session_id>` (same cwd) → continues context
- [ ] `--resume <session_id>` from a **different** cwd → confirm it silently forks (the foot-gun)
- [ ] locate the transcript: `~/.claude/projects/<encoded-cwd>/<session_id>.jsonl`; record the cwd-encoding scheme (backs `thread/list`/`thread/read`)

### Gate

- [ ] Step 2 succeeds on the documented `--permission-prompt-tool` route → proceed with **Option A**.
- [ ] Step 2 does not → **switch to Option B (sidecar)** before writing adapter code.

Deliverable: `07a-Claude-Spike-Findings.md` (version, captured frames, chosen approval
route, encoded-cwd scheme).

---

## Appendix B — File Scaffolding (stubs + tests)

> Reference skeletons for the new `apps/local-runner/internal/runner/` files. Create these
> **after** the Phase-0 gate passes. Names mirror the Codex set so reviewers can diff the two.

```go
// claude_process.go — process pool + spawn (mirror codex_appserver_process.go)
const claudeAdapterEnvFlag = "FLOWPILOT_CLAUDE_ADAPTER"
func claudeAdapterEnabled() bool        { /* env 1/true/yes, mirror codexAppServerEnabled */ }
var claudeBinaryName = func() string    { /* FLOWPILOT_CLAUDE_BIN or "claude" */ }

type claudeProcKey struct{ account, cwd, session string }
type claudeProcess struct { stream *claudeStream; kill func(); sessionID string }
type claudeProcessPool struct { /* mu; procs map[claudeProcKey]*claudeProcess; idleTTL; max */ }
func (p *claudeProcessPool) acquire(ctx context.Context, key claudeProcKey, args []string, env map[string]string) (*claudeProcess, error)
func (p *claudeProcessPool) drop(key claudeProcKey) // account-switch / process death
```

```go
// claude_stream.go — per-process newline-JSON read loop (a simplified codexDispatcher:
// one process == one logical thread, so no threadSubs routing map is needed)
type claudeLine struct { Type, Subtype string; Raw map[string]any }
type claudeStream struct { /* w io.Writer; writeMu; lines chan claudeLine; done; closeErr */ }
func newClaudeStream(w io.Writer) *claudeStream
func (s *claudeStream) start(r io.Reader)               // read loop; fail() drains on EOF/err
func (s *claudeStream) writeUserTurn(text string) error // {"type":"user",...} + write mutex
func (s *claudeStream) fail(err error)                  // drain lines chan with error (no hung turn)
```

```go
// claude_adapter.go — implements ProviderRuntimeAdapter (mirror codex_adapter.go)
type claudeAdapter struct { pool *claudeProcessPool; cwd, mcpConfig string; promptPrep func(TurnRequest) string }
func (a *claudeAdapter) Key() ProviderKey                   { return ProviderKeyClaude }
func (a *claudeAdapter) Capabilities() ProviderCapabilities { /* all 7 true */ }
func (a *claudeAdapter) SendTurn(ctx context.Context, req TurnRequest, bridge TurnBridge) error // spawn/reuse → write turn → pump mapper→Emit → result
func claudeArgs(posture YoloPosture, sessionID, mcpConfig string, skills []SkillSelection) []string
```

```go
// claude_event_mapper.go — stream-json line → ProviderEvent (mirror codex_event_mapper.go)
func mapClaudeLine(l claudeLine) (ProviderEvent, bool)            // system/init, stream_event, assistant, user(tool_result), result
func deriveFileChanged(toolUse map[string]any) (ProviderEvent, bool) // Edit/Write/MultiEdit/NotebookEdit → file_changed
// leave ProviderTurnID empty: runner core stamps the canonical turn id + seq (emitLocked)
```

```go
// claude_permission_mcp.go — Go-served MCP server: approve + ask_user → TurnBridge
func (s *InteractiveService) serveFlowpilotMcp(bridge TurnBridge) (configPath string, stop func(), err error)
//   tool mcp__flowpilot__approve  → bridge.RequestApproval(details) → allow/deny
//   tool mcp__flowpilot__ask_user → bridge.AskQuestion(prompt,options,multi) → choice
```

```go
// yolo_resolver.go (EDIT) — extend YoloPosture; keep resolveYoloPosture the single SSOT
type YoloPosture struct { CodexSandbox, CodexApprovalMode, ClaudePermissionMode string; RunnerAutoApprove bool }
// yolo=true → ClaudePermissionMode "bypassPermissions"; yolo=false → "default"
```

```go
// provider_registry.go (EDIT) — ProviderRegistryFor: add a FLOWPILOT_CLAUDE_ADAPTER-gated claude branch
// if claudeAdapterEnabled() { register Available claude backed by claudeAdapter(r.claudePool, account env) }
// else keep the placeholderAdapter (default registry unchanged)
```

Test skeletons (`claude_*_test.go`, scripted fake `claude` over in-memory pipes, per `sessions_test.go:89`):

```go
func TestClaudeStreamInitParsed(t *testing.T)          {} // CL-01 init → turn_started + session_id
func TestClaudeAdapterStreamsDeltas(t *testing.T)      {} // CL-05 stream_event → message_delta
func TestClaudeFinalResultSeparated(t *testing.T)      {} // CL-07 result → clean turn_completed
func TestClaudeCommandExecExitStatus(t *testing.T)     {} // CL-10 Bash tool_result + hook exit_code
func TestClaudeDerivesFileChanged(t *testing.T)        {} // CL-12 Edit/Write → file_changed
func TestClaudeYoloFalseApprovalRoundTrip(t *testing.T){} // CL-20 approve resumes / deny blocks
func TestClaudeYoloTrueBypassNoPrompt(t *testing.T)    {} // CL-21 bypassPermissions, audited
func TestClaudeAskUserQuestionRoundTrip(t *testing.T)  {} // CL-25 ask_user → user_question_required
func TestClaudeResumeKeepsCwd(t *testing.T)            {} // CL-30 resume same cwd / mismatch detected
func TestClaudeProcessDeathRecoverable(t *testing.T)   {} // CL-31 pool drain, no hung turn
func TestClaudeRegistryGating(t *testing.T)            {} // CL-40 gated off/on + errorAdapter
```
