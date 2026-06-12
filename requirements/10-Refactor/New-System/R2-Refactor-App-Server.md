# R2: Refactor Plan - Provider Runtime Gateway And App-Server Strategy

This document captures the proposed refactor direction for making FlowPilot work as a **headless workflow controller** while still supporting Codex, Claude, and Gemini.

The main decision is:

- Do **not** make Codex app-server the architecture.
- Make Codex app-server one implementation of a provider-neutral runtime gateway.
- Keep FlowPilot's core value in workflow control, prompt optimization, artifact/RAG persistence, and approval/proxy policy.

---

## Source References

- Codex app-server: `https://developers.openai.com/codex/app-server`
- Codex MCP: `https://developers.openai.com/codex/mcp`
- Codex hooks: `https://developers.openai.com/codex/hooks`
- Codex skills: `https://developers.openai.com/codex/skills`
- Claude headless mode: `https://code.claude.com/docs/en/headless`
- Claude hooks: `https://code.claude.com/docs/en/hooks`
- Claude Agent SDK hooks: `https://code.claude.com/docs/en/agent-sdk/hooks`
- Gemini CLI configuration: `https://geminicli.com/docs/reference/configuration/`
- Gemini CLI repository: `https://github.com/google-gemini/gemini-cli`
- FlowPilot YOLO/provider boundary: `requirements/08-Task/done/Task-030-Workflow-Runs-Detail-Page-Yolo-Indicators.md`
- FlowPilot provider approval config note: `requirements/08-Task/todo/Task-032-Document-Proxy-Yolo-And-Provider-Approval-Config.md`

---

## Problem

FlowPilot's web chat UX is currently weaker than the native provider clients:

- Codex App / Codex CLI
- Claude Code
- Gemini CLI

But FlowPilot has product value that native clients do not provide by themselves:

- workflow steps with controlled progression
- optimized prompts generated from markdown process definitions
- persistent artifacts
- summary generation
- Supabase RAG indexing
- project/workflow history
- FlowPilot-owned approval gates
- MCP proxy enforcement, especially Google Drive proxy MCP

The question is whether we can keep the runner as the product core and still use native provider experiences or provider runtimes underneath.

---

## Target Architecture

FlowPilot should own the workflow and memory layer. Providers should be replaceable runtime adapters.

```text
User Surface
  - FlowPilot Web
  - FlowPilot CLI
  - future FlowPilot IDE extension
  - optional native provider client assist mode

        |
        v

FlowPilot Runner Core
  - workflow state machine
  - prompt optimizer
  - artifact writer
  - summary generator
  - Supabase RAG indexer
  - approval/proxy policy
  - run/session persistence

        |
        v

Provider Runtime Gateway
  - CodexAdapter
  - ClaudeAdapter
  - GeminiAdapter
```

Codex app-server belongs inside `CodexAdapter`; it must not leak into workflow engine business logic.

---

## Three-Part Product Split

FlowPilot should be allowed to split into three product/runtime parts instead of
forcing all interaction through the current React web app.

```text
1. Runner Runtime
   - Provider Runtime Gateway
   - Codex app-server adapter
   - Claude/Gemini adapters
   - MCP proxy and approval/proxy policy
   - workflow state machine
   - prompt optimizer
   - artifacts, summaries, Supabase RAG

2. Admin Web
   - workflow configuration
   - provider configuration
   - MCP/proxy configuration
   - project setup
   - run history and audit views
   - non-latency-critical actions

3. Interactive Client UX
   - cross-platform Electron desktop app (first client; works alongside any IDE)
   - optional VS Code/JetBrains plugins and CLI clients later
   - chat input and streaming output
   - slash commands and skill picker
   - command approval prompts
   - file/change navigation
   - low-latency local interaction
```

### Why Split The Client UX Out Of React Web

The current React web app does not have to carry every product responsibility.
It can remain the admin/configuration surface, while the interactive coding UX
moves closer to the developer's working environment.

A VS Code extension is a strong first controlled client because it can provide:

- faster perceived interaction for chat and streaming events
- native file opening and diagnostics navigation
- better path rendering, such as displaying a file name while linking to the
  exact workspace path
- editor-aware context selection
- command approval prompts close to the code being changed
- a natural home for `/` commands, skill selection, and workflow step controls

The runner remains the source of truth. The client is not allowed to own workflow
business logic. It should call the runner APIs, subscribe to normalized provider
events, and render them well.

### Boundary Rules

- Runner Runtime owns workflow state, provider sessions, approval policy,
  prompt optimization, artifacts, summaries, and RAG persistence.
- MCP proxy remains with the runner because it is part of FlowPilot-owned
  policy enforcement.
- Admin Web owns configuration and audit-oriented views, not the primary
  low-latency coding chat.
- VS Code or another interactive client owns the real-time user experience:
  input, streaming, command approvals, file links, and editor navigation.
- Provider-specific protocol details still stay inside provider adapters.
- Interactive clients consume normalized FlowPilot events, not raw Codex
  app-server JSON-RPC.

This means the long-term product shape can be:

```text
Electron Desktop App / CLI / future IDE plugins
        |
        v
FlowPilot Runner Runtime
        |
        +-- MCP Proxy
        +-- Provider Runtime Gateway
              +-- Codex app-server
              +-- Claude adapter
              +-- Gemini adapter

React Admin Web
        |
        v
FlowPilot configuration, history, audit, and setup APIs
```

This keeps FlowPilot's automation guarantees while avoiding the cost of making
the existing web app behave like a native coding client.

---

## VS Code Extension Client Strategy

> **Superseded (final decision):** the interactive client is a **cross-platform
> Electron desktop app**, not a VS Code extension — so one client works alongside
> any IDE (VS Code, Android Studio, Xcode), not just VS Code. The reasoning in this
> section about a thin client over the runner still holds; only the packaging
> changes (Electron desktop instead of a VS Code extension). The React webview is
> kept IDE-agnostic so optional VS Code/JetBrains plugins can reuse it later. See
> `05` "Interactive Client Strategy" and `04 §4.2` for the authoritative version.

The first Interactive Client UX should be a VS Code extension. This gives
FlowPilot a low-latency coding surface without forcing the React Admin Web to
become a full native-coding client.

### Implementation Stack

Use TypeScript for the VS Code extension.

Recommended shape:

```text
extensions/vscode-flowpilot/
  package.json
  src/
    extension.ts
    runnerClient.ts
    webview/
      App.tsx
      components/
```

The extension shell should use the VS Code Extension API for:

- activation
- command registration
- sidebar or panel registration
- file opening
- workspace path resolution
- notifications and quick picks
- secure local configuration

The rich UI should use a VS Code Webview with React and TypeScript because
FlowPilot needs more than simple command palettes:

- chat input and streaming output
- workflow selector
- workflow step selector
- run timeline
- approval cards/modals
- file change list
- tool and MCP activity rows
- slash command and skill picker

### Development Workflow

The extension can be used inside VS Code while it is in development.

Typical loop:

1. open `extensions/vscode-flowpilot/` in VS Code
2. run the extension in an Extension Development Host
3. connect it to a local FlowPilot Runner Runtime
4. test workflow selection, chat, event streaming, approvals, and file links
5. reload the development host after extension or webview changes

This allows FlowPilot to validate the real coding UX before packaging or
publishing the extension.

### Runner Communication

The extension must not talk directly to Codex app-server. It should talk only to
FlowPilot Runner APIs and event streams.

Expected communication:

```text
VS Code Extension
        |
        | HTTP / WebSocket / local transport
        v
FlowPilot Runner Runtime
        |
        +-- workflow state
        +-- provider runtime gateway
        +-- MCP proxy
        +-- approval/proxy policy
        +-- artifacts, summaries, RAG
```

The runner should expose client-facing operations such as:

- list projects
- list workflows
- list workflow steps
- start workflow run
- resume workflow run
- select or advance workflow step
- send provider turn
- stream normalized `ProviderEvent`
- answer `permission_required`
- open or resolve artifact metadata

The extension should render normalized FlowPilot events. It should not depend on
raw Codex app-server JSON-RPC names.

### Workflow And Step UI

Because FlowPilot has many workflows and workflow steps, the extension should
provide first-class selection UI.

Expected UI:

- project selector
- workflow selector
- step selector
- current run status
- active step indicator
- previous/next step navigation
- run history for the current workspace
- compact workflow progress view

This UI replaces the need to drive controlled workflow execution through a web
admin page while coding.

The React Admin Web can still show the same data for configuration and audit,
but the extension should be the primary place where a developer selects and runs
workflow steps during coding.

### Approval UI

The extension should own the real-time approval experience.

When the runner emits:

```ts
{ type: "permission_required", provider: "codex", details: unknown }
```

the extension should display an approval card or modal close to the chat/run
timeline.

Approval UI should show:

- provider
- command or tool name
- working directory
- reason
- risk label when available
- available approval decisions
- affected workflow run and step

The user's decision should be sent back to the runner. The runner then forwards
the provider-specific decision to the relevant adapter and persists the decision
in the run log.

This keeps the security boundary in the runner while allowing the extension to
provide a fast native-feeling approval experience.

### File And Response Rendering

The extension can provide better rendering than the current web chat because it
has direct access to the user's editor workspace.

Examples:

- display only `workflow-start-runtime.ts`
- keep the full path in tooltip/details
- click to open the file in VS Code
- reveal changed files in Explorer
- show command output collapsed by default
- separate assistant final response from logs and tool output

This is one of the main reasons to move the coding UX out of the React Admin
Web.

### Responsibility Boundary

- VS Code extension owns interaction and rendering.
- Runner owns workflow business logic and provider control.
- Admin Web owns configuration, setup, history, and audit.
- Provider adapters own provider-specific protocol details.

---

## Provider Runtime Gateway Contract

The runner should define a provider-neutral contract that every provider adapter implements.

```ts
interface ProviderRuntimeAdapter {
  startSession(input: ProviderSessionStartInput): Promise<ProviderSession>;
  resumeSession(input: ProviderSessionResumeInput): Promise<ProviderSession>;
  sendTurn(input: ProviderTurnInput): AsyncIterable<ProviderEvent>;
  interrupt(input: ProviderInterruptInput): Promise<void>;
  closeSession(input: ProviderCloseInput): Promise<void>;
}

interface ProviderTurnInput {
  sessionId: string;
  workflowRunId: string;
  workflowStepRunId: string;
  optimizedPrompt: string;
  workingDirectory: string;
  modelName?: string;
  reasoningEffort?: string;
  requiredMcps?: string[];
  mcpAccessMode?: "read_only" | "read_write";
  yoloMode: boolean;
}

type ProviderEvent =
  | { type: "turn_started"; providerTurnId: string }
  | { type: "message_delta"; text: string }
  | { type: "message_completed"; text: string }
  | { type: "tool_started"; toolName: string; input?: unknown }
  | { type: "tool_completed"; toolName: string; output?: unknown }
  | { type: "file_changed"; path: string }
  | { type: "permission_required"; provider: string; details: unknown }
  | { type: "turn_failed"; error: string }
  | { type: "turn_completed"; finalMessage: string };
```

FlowPilot core consumes only this normalized event stream.

Provider-specific details stay inside adapters:

- Codex JSON-RPC app-server event names
- Claude Agent SDK stream objects or `claude -p` output
- Gemini CLI `stream-json` output
- provider-specific approval settings
- provider-specific session ids

---

## Codex Adapter

### Runtime Strategy

Use `codex app-server` for the Codex adapter when FlowPilot needs strong control.

FlowPilot runner starts or connects to:

```bash
codex app-server --listen stdio://
```

or a local socket/WebSocket transport when appropriate.

The adapter then:

1. sends `initialize`
2. sends `thread/start` or `thread/resume`
3. sends `turn/start` with the optimized prompt
4. reads streamed notifications
5. maps Codex item/turn notifications into `ProviderEvent`
6. emits `turn_completed`

### What This Gives FlowPilot

- FlowPilot controls the actual prompt sent to Codex.
- Prompt `A` can be transformed into optimized prompt `ABC` before Codex sees it.
- FlowPilot can stream Codex events into the run log.
- FlowPilot can capture final response, tool events, and file-change signals.
- FlowPilot can persist artifacts and RAG summaries after every turn.
- FlowPilot can store Codex thread id against the workflow run for resume.

### Boundary

This mode means the user is not chatting in the native Codex App UI. The user chats through FlowPilot Web, FlowPilot CLI, or a future FlowPilot IDE extension, and FlowPilot acts as the Codex app-server client.

---

## Claude Adapter

### Runtime Strategy

Claude must be handled through its own adapter, not through Codex app-server.

Preferred options:

1. Claude Agent SDK for deeper programmatic control.
2. `claude -p` for simpler non-interactive execution.

Claude's documented headless mode supports:

- `claude -p`
- structured output
- streaming
- continuing conversations
- `--allowedTools`
- MCP config
- hook integration

Claude's Agent SDK/hook system is stronger than a plain CLI wrapper because it can provide lifecycle control, tool approval callbacks, input/output transformation, and hook decisions.

### What The Adapter Should Normalize

- Claude session id
- streamed assistant text
- tool-use events
- permission/approval requests
- final message
- changed files or artifact paths

### Known Difference From Codex

Claude does not use Codex app-server. FlowPilot needs a separate Claude runtime implementation and test coverage.

---

## Gemini Adapter

### Runtime Strategy

Gemini should be handled through a Gemini adapter.

Initial implementation can use Gemini CLI:

```bash
gemini -p "<optimized prompt>" --output-format stream-json
```

Gemini CLI documentation describes:

- non-interactive prompt mode with `-p`
- `--output-format json`
- `--output-format stream-json`
- MCP configuration
- sandbox mode
- `--yolo`
- resume/session flags

### What The Adapter Should Normalize

- Gemini session id when available
- stream-json events
- assistant message chunks
- tool calls
- final response
- permission or failed-tool events when exposed

### Known Gap

Gemini likely has weaker programmatic lifecycle and approval-control surfaces than Codex app-server or Claude Agent SDK. The first Gemini adapter should be treated as a lower-control adapter until we prove:

- stable stream-json schema
- reliable resume behavior
- reliable MCP tool event capture
- non-hanging permission behavior

---

## Two Product Modes

FlowPilot should support two modes rather than forcing one UX.

### Mode A: Native Assist Mode

Native provider client remains the chat UI.

```text
Codex App / Claude Code / Gemini CLI
        |
        | MCP tools + hooks where supported
        v
FlowPilot Runner Proxy
```

FlowPilot capabilities in this mode:

- expose FlowPilot tools through MCP
- allow the model to fetch optimized workflow context
- allow the model to save artifacts
- use hooks where supported to notify FlowPilot at lifecycle events
- run post-turn artifact/RAG sync when hooks are available

Limitations:

- FlowPilot cannot reliably force the model to call a tool before every final answer.
- FlowPilot may not see the exact prompt before the provider sees it.
- FlowPilot may not see the exact final response unless the provider exposes it through hooks/logs.
- Workflow step compliance is weaker because the native provider client owns the turn.
- Provider-native dangerous command prompts are not FlowPilot-owned gates.

This mode is best for convenience and keeping the native UX.

### Mode B: Controlled Workflow Mode

FlowPilot owns the client surface and calls provider adapters.

```text
FlowPilot Web / CLI / IDE Extension
        |
        v
FlowPilot Runner Core
        |
        v
Provider Runtime Gateway
        |
        +-- Codex app-server
        +-- Claude Agent SDK / claude -p
        +-- Gemini CLI stream-json
```

FlowPilot capabilities in this mode:

- guaranteed prompt optimization before provider sees the prompt
- guaranteed run/step state tracking
- guaranteed artifact save after provider turn
- guaranteed summary and Supabase RAG indexing
- normalized provider event stream
- stronger resume/retry semantics
- stronger workflow step boundaries
- stronger FlowPilot-owned approval gates

Limitations:

- FlowPilot must provide the chat/command UX.
- Native Codex App / Claude Code / Gemini CLI UX is not the primary interface.
- Provider parity depends on adapter quality.

This mode is best for real workflow automation.

---

## Comparison: Current Codex Command Call vs Codex App-Server

FlowPilot may already call Codex through a command today. `codex app-server`
does not remove command execution entirely. The difference is the integration
boundary.

Current command-based execution treats Codex like a one-shot subprocess:

```bash
codex ... "<optimized prompt>"
```

FlowPilot starts the process, passes input, then interprets stdout, stderr,
exit codes, generated files, or logs.

Codex app-server starts Codex as a structured runtime:

```bash
codex app-server --listen stdio://
```

FlowPilot still starts or connects to a process, but after startup it talks to
Codex through a protocol:

1. initialize runtime
2. start or resume thread
3. start turn with the optimized prompt
4. stream turn/item/tool notifications
5. interrupt, steer, or close when needed
6. persist provider thread/session metadata

| Capability | Current Codex Command Call | Codex App-Server Adapter |
|---|---|---|
| Process model | One-shot subprocess per prompt or run | Long-lived runtime process or managed session |
| Integration boundary | Text output, exit code, generated files, logs | Structured protocol events |
| Prompt optimization | Possible, but only at process invocation time | Strong; prompt is sent through `turn/start` |
| Streaming | Requires parsing provider output | Structured streamed notifications |
| Final response capture | Fragile if mixed with logs/tool output | Stronger; final message is part of turn lifecycle |
| Tool and file events | Best-effort parsing or post-run diff | Direct event mapping into `ProviderEvent` |
| Resume semantics | Depends on CLI flags and external state | Store Codex thread id and resume explicitly |
| Interrupt behavior | Usually kill the process | Protocol-level interrupt or steer where supported |
| Failure diagnosis | Split across stdout, stderr, exit code, and files | Centralized provider event timeline |
| Artifact/RAG finalization | Runs after process exit; may lack exact turn context | Runs after `turn_completed` with normalized context |
| Workflow step enforcement | Medium; command execution is coarse-grained | Stronger; FlowPilot owns turn dispatch and completion |

The practical upgrade is not "no command." The upgrade is moving from
subprocess text I/O to a stateful runtime protocol that FlowPilot can observe,
resume, finalize, and audit.

The current command path can remain useful as a compatibility adapter or
fallback. Codex app-server should be the preferred Codex implementation for
Controlled Workflow Mode because it gives FlowPilot stronger lifecycle control.

---

## Pain Points App-Server Should Address

The practical motivation for Codex app-server is not only architectural purity.
It directly addresses current FlowPilot UX and control pain points that make the
native Codex App feel tempting.

### 1. Slash Input And Skill Selection

FlowPilot can implement its own `/` input behavior in Controlled Workflow Mode.
For example:

1. user types `/`
2. FlowPilot opens a command or skill picker
3. Codex adapter requests the available Codex skills
4. user selects a skill
5. FlowPilot sends the optimized prompt plus the selected skill through the
   Codex app-server turn protocol

This means FlowPilot does not need to copy the native Codex App UI exactly.
It needs to build a first-class controlled input surface that can invoke the
same underlying Codex skill concept.

Design implication:

- `/` is a FlowPilot UI command trigger.
- `$skill-name` can still be supported as a text shortcut if useful.
- selected skills should be stored on the provider turn record for auditability.
- skills should be shown as workflow/runtime capabilities, not only free-text
  prompt hints.

### 2. Dangerous Command Approval

Codex app-server is a better fit than raw command execution when the model wants
to run a dangerous command because the adapter can receive structured approval
requests from Codex and surface them through FlowPilot UI.

The desired FlowPilot behavior:

1. Codex requests command execution approval.
2. Codex adapter maps the provider request to `permission_required`.
3. FlowPilot shows the command, working directory, reason, and available
   decisions.
4. user approves, denies, or chooses a scoped approval option when supported.
5. adapter sends the decision back to Codex app-server.
6. the decision and resulting command event are saved in the run log.

This is stronger than parsing terminal output because FlowPilot receives an
intentional approval event instead of guessing from process text.

Important boundary:

- app-server improves approval UX and event capture.
- app-server alone is not a complete security model.
- FlowPilot still needs explicit sandbox, provider approval, proxy, and YOLO
  policy before claiming safe handling of dangerous host commands.

### 3. Clean, Product-Quality Response Rendering

Codex app-server gives FlowPilot structured events, which makes clean rendering
possible without relying on raw stdout formatting.

Examples:

- render `agentMessage` as assistant text
- render `commandExecution` as a command block with status
- render `fileChange` as a changed-file item
- render `mcpToolCall` as a tool activity row
- render final response separately from logs and tool output

This lets FlowPilot hide noisy implementation details in the UI. For example, a
full path like:

```text
apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts
```

can be displayed as:

```text
workflow-start-runtime.ts
```

with the full path available in a tooltip, detail panel, copy action, or link.

This is difficult to do reliably with command-output parsing because the final
answer, logs, paths, command output, and tool status can be mixed together.
With app-server, FlowPilot can map structured provider items into stable UI
components.

### Product Conclusion

The earlier instinct to return to the native Codex App is understandable because
the native client has better interaction quality than the current FlowPilot
command-based integration.

However, Codex app-server can address the core pain directly:

- interactive controlled input
- skill selection
- structured dangerous-command approvals
- structured event capture
- cleaner run logs
- product-quality response rendering

Therefore the product direction should not be "go back to native Codex App" as
the reliable automation path. It should be:

- use Codex app-server for FlowPilot Controlled Workflow Mode
- keep Native Assist Mode as a convenience path for users who prefer native
  provider clients

---

## Comparison: Codex Client App + FlowPilot Proxy vs FlowPilot Controlled App-Server

| Capability | Native Codex App + FlowPilot Proxy | FlowPilot Controlled Mode With Codex App-Server |
|---|---|---|
| Chat UX | Best native Codex UX | FlowPilot must build/own UX |
| Prompt optimization before model sees prompt | Weak; model must call MCP or follow instruction | Strong; FlowPilot sends optimized prompt through `turn/start` |
| Guaranteed post-turn sync | Medium with hooks, not pure MCP | Strong; FlowPilot consumes turn events |
| Exact final response capture | Not guaranteed unless hook/transcript access exposes it | Strong; app-server streams turn/item events |
| Artifact/RAG persistence | Best-effort unless hooks enforce post-turn action | Guaranteed by runner after `turn_completed` |
| Workflow step enforcement | Medium; native client owns conversation | Strong; FlowPilot owns workflow state and turn dispatch |
| Provider dangerous command approval | Provider-owned; FlowPilot cannot fully gate | Still provider-owned unless sandbox/proxy is added |
| Google Drive proxy MCP gate | Strong only for FlowPilot-owned proxy tool calls | Strong for FlowPilot-owned proxy tool calls |
| Multi-provider support | Needs per-client MCP/hooks setup | Needs provider adapters |
| Resume semantics | Native client resume plus FlowPilot sync is fragile | FlowPilot stores provider session/thread ids |
| Failure diagnosis | Split across native client and FlowPilot | Centralized in FlowPilot run log |
| Product direction | Assistant sidecar | Workflow orchestration runtime |

Conclusion:

- Native Codex App + FlowPilot proxy is good for low-friction adoption.
- FlowPilot Controlled Mode is required for strong automation guarantees.
- Codex app-server is the best Codex implementation for Controlled Mode, but it does not solve Claude/Gemini by itself.

---

## Comparison Across Providers

| Feature | Codex Adapter | Claude Adapter | Gemini Adapter |
|---|---|---|---|
| Primary integration | `codex app-server` | Claude Agent SDK or `claude -p` | Gemini CLI |
| Prompt rewrite before provider sees it | Yes | Yes | Yes |
| Streaming events | Yes | Yes | Likely via `stream-json` |
| Exact final response capture | Yes | Yes | Yes, if JSON output is stable |
| Session resume | Yes via thread APIs | Yes via Claude continuation/session support | Depends on Gemini session support |
| MCP support | Yes | Yes | Yes |
| Hook support | Codex hooks for native assist mode | Strong Claude hooks/SDK hooks | Needs investigation |
| Approval callback/control | App-server plus Codex approval model | Stronger via Agent SDK/hooks | Needs investigation |
| Current confidence | High | Medium-high | Medium-low |

---

## Runner Refactor Slices

### Slice 1: Define Provider Runtime Gateway

- Add provider-neutral interfaces in the runner.
- Define normalized event types.
- Define provider session persistence shape.
- Keep existing `ExecutePrompt` behavior behind a compatibility adapter.

### Slice 2: Codex App-Server Adapter

- Start `codex app-server`.
- Implement JSON-RPC client.
- Implement `thread/start`, `thread/resume`, `turn/start`, `turn/steer`, and event reading.
- Persist Codex thread id against workflow run/session.
- Map Codex streamed events to normalized `ProviderEvent`.

### Slice 3: Artifact And RAG Turn Finalizer

- Add a provider-neutral `TurnFinalizer`.
- On `turn_completed`:
  - save final response artifact
  - snapshot changed files or git diff
  - generate summary
  - push summary/artifact metadata to Supabase RAG
  - update workflow step status

### Slice 4: Claude Adapter

- Decide Claude Agent SDK vs `claude -p` implementation.
- Prefer SDK if it gives reliable streaming, permission callbacks, and structured message objects.
- Map Claude events to normalized `ProviderEvent`.
- Add tests for final response capture and artifact finalization.

### Slice 5: Gemini Adapter

- Implement Gemini CLI `--output-format stream-json` adapter.
- Verify schema stability.
- Verify session resume behavior.
- Verify MCP event visibility.
- Document lower-control limitations if any behavior cannot be enforced.

### Slice 6: Native Assist Mode

- Expose FlowPilot runner as MCP server.
- Add provider-specific setup helpers:
  - Codex MCP config + hooks
  - Claude MCP config + hooks
  - Gemini MCP config
- Treat this mode as best-effort convenience, not full workflow enforcement.

### Slice 7: Provider Permission Policy

- Define what FlowPilot can and cannot approve.
- Keep current rule:
  - FlowPilot safe gates are enforceable only where FlowPilot owns the gate.
  - Provider-native dangerous host commands require sandboxing, provider callback integration, or a separate command approval bridge.
- Do not claim provider dangerous-command safety from YOLO alone.

---

## Required Data Model Additions

Add or normalize provider session metadata:

```text
workflow_provider_sessions
  id
  workflow_run_id
  workflow_step_run_id nullable
  provider_key
  provider_session_id
  provider_thread_id nullable
  transport
  working_directory
  model_name
  reasoning_effort
  status
  created_at
  updated_at
```

Add normalized event storage:

```text
workflow_provider_events
  id
  workflow_run_id
  workflow_step_run_id nullable
  provider_session_id
  provider_key
  event_type
  payload_json
  occurred_at
```

These tables should not replace existing workflow run/step state. They are provider-runtime telemetry.

---

## Design Rules

- FlowPilot core must not import Codex-specific app-server types.
- Provider adapters must emit normalized events.
- Prompt optimization must happen before adapter `sendTurn`.
- Artifact/RAG finalization must be provider-neutral.
- Native Assist Mode must be documented as best-effort.
- Controlled Workflow Mode must be documented as the reliable automation path.
- Codex app-server is a Codex adapter, not the product architecture.
- Claude and Gemini support must be adapter-driven from the beginning.

---

## Resolved Decisions

See `05-Codex-AppServer-Migration-Detail.md` for the canonical, code-grounded
versions.

- **Codex app-server lifecycle** → **one shared, long-lived app-server child
  process per runner**, with a **multi-workspace runner** routing `cwd` per thread.
  This mirrors the native Codex App Client (one app-server hosting many threads
  across many working directories). Not per workflow run, not per workspace.
- **Session granularity** → a Codex **thread** is the session primitive (bound to a
  `cwd`). FlowPilot does not pin one session rigidly to a run or step; workflow
  runs/steps reference the thread(s) they used, and FlowPilot persists only the
  `(workspace, threadId)` mapping. Lean on Codex thread APIs rather than a custom
  session registry.

## Open Questions

- Should FlowPilot Web remain the primary Controlled Mode UI, or should the first controlled surface be CLI-only?
- Which Claude path is best for FlowPilot: Agent SDK or `claude -p`?
- How stable is Gemini CLI `stream-json` across versions?
- Should Native Assist Mode be configured automatically by the runner or only documented for power users?
- What sandbox policy is required before FlowPilot can advertise safe handling for dangerous host commands?
- How should the multi-workspace runner register/bind workspaces, and how do clients select the active `cwd`?

---

## Recommendation

Implement **Controlled Workflow Mode** as the reliable path and **Native Assist Mode** as the convenience path.

Order:

1. Provider Runtime Gateway contract.
2. Codex app-server adapter.
3. Provider-neutral artifact/RAG finalizer.
4. Claude adapter.
5. Gemini adapter.
6. Native Assist Mode MCP/hooks setup.
7. Provider permission policy.

This lets FlowPilot keep its real purpose:

- process control
- optimized prompts
- artifacts
- summaries
- Supabase RAG
- approval/proxy gates

without becoming locked to Codex.
