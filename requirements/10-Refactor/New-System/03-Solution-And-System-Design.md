# 03 - Solution And System Design

## Chosen Direction

FlowPilot should move to a three-part system:

1. Runner Runtime
2. Admin Web
3. Interactive Client UX

Codex app-server is not the whole architecture. It is the Codex implementation
inside a provider-neutral runtime gateway.

## Architecture Goals

The new system must satisfy these goals:

- preserve FlowPilot-owned workflow control
- make provider runtimes replaceable
- keep Codex-specific details out of core workflow logic
- support a fast IDE-based client UX
- keep Admin Web focused on configuration and audit
- make provider events durable and inspectable
- support approval decisions as first-class workflow events
- keep MCP proxy policy centralized in the runner
- allow Claude and Gemini adapters later without redesigning the runner

## Architecture Non-Goals

The new system should not:

- turn Codex app-server into the whole product architecture
- make desktop client responsible for workflow business rules
- make Admin Web responsible for low-latency coding interaction
- expose raw provider protocol details to clients
- force every provider to have the same feature depth on day one
- claim dangerous-command safety without sandbox and approval policy

## Why Choose App-Server

App-server resolves the current pain without giving up FlowPilot's workflow
control.

It gives FlowPilot:

- structured provider events
- provider session/thread lifecycle
- command and permission approval events
- final response capture
- file and tool activity capture
- stronger resume semantics
- reliable turn completion signal

These are the missing pieces needed to build a better client UX without falling
back to native Codex App as the main product surface.

## System Components

### Runner Runtime

The runner should be a long-lived local or server-side process that exposes APIs
to Admin Web and interactive clients.

Internal modules:

- `WorkflowRuntime`: owns run and step lifecycle
- `PromptOptimizer`: creates optimized prompts from workflow definitions
- `ProviderRuntimeGateway`: selects and calls provider adapters
- `McpProxyRuntime`: owns FlowPilot proxy tools and policy
- `ApprovalPolicyEngine`: decides what can be auto-approved, blocked, or shown
  to the user
- `ProviderEventStore`: persists normalized provider events
- `ProviderSessionStore`: persists provider session/thread metadata
- `TurnFinalizer`: saves final response, artifacts, summary, and RAG metadata
- `ClientEventHub`: streams normalized events to the desktop client and Admin Web

### Admin Web

Admin Web should be a client of the runner/admin APIs.

Main modules:

- provider settings
- MCP/proxy settings
- approval policy settings
- workflow configuration
- workflow run history
- provider session/event audit
- artifact and summary browser
- desktop client connection/setup page

### Interactive Desktop Client (Electron)

The interactive client is a **cross-platform Electron desktop app** (final
decision — see `04 §4.2` and `05` "Interactive Client Strategy"). One codebase runs
on Windows + macOS (+ Linux) and works alongside any IDE (VS Code, Android Studio,
Xcode). It is a client of the runner interactive APIs.

Main modules:

- `RunnerClient`: HTTP/WebSocket/local transport wrapper (React renderer)
- `WorkflowNavigator`: project/workflow/step navigation
- `ChatPanel`: chat input, streaming output, run timeline
- `ApprovalView`: command/tool approval card
- `FileEventRenderer`: changed-file rows; opens files in the user's IDE via its CLI
- `SkillPicker`: `/` command and skill selection
- `IdeBridge` (Electron main): detect + invoke IDE CLI (`code -g` / `studio` / `xed`)
- `LocalConfig`: runner URL, auth token, workspace binding

The renderer (React webview) stays IDE-agnostic so it can be reused inside an
optional VS Code/JetBrains plugin later. The client renders state; it must not
compute workflow progression.

## How It Resolves Current Pain Points

| Pain Point | App-Server Resolution |
|---|---|
| weak web chat UX | move coding UX to desktop client while runner controls turns |
| noisy full file paths | render structured file events as file-name links |
| mixed logs and final answer | separate event types in UI |
| dangerous command approval | surface structured `permission_required` events |
| weak workflow step enforcement | runner owns turn dispatch and step state |
| unreliable artifact/RAG sync | finalizer runs after normalized `turn_completed` |
| fragile resume | persist provider session/thread ids |
| hard `/` and skill UX | client provides picker, adapter sends selected skill |
| hard failure diagnosis | centralized provider event log |

## Client-To-Runner API Shape

The exact transport can be HTTP plus WebSocket, local socket, or another local
transport. The contract should be stable regardless of transport.

Required commands:

```text
GET  /projects
GET  /projects/:projectId/workflows
GET  /workflows/:workflowId/steps
POST /workflow-runs
POST /workflow-runs/:runId/resume
POST /workflow-runs/:runId/steps/:stepRunId/turns
POST /workflow-runs/:runId/approvals/:approvalId/decision
GET  /workflow-runs/:runId/events
GET  /workflow-runs/:runId/artifacts
GET  /provider-capabilities
```

Required event stream:

```text
GET /workflow-runs/:runId/events/stream
```

The stream should emit normalized FlowPilot events, not raw provider events.

## State Ownership

| State | Owner | Notes |
|---|---|---|
| project config | Runner/Admin APIs | rendered and edited by Admin Web |
| workflow definitions | Runner/Admin APIs | source for prompt optimization |
| active workflow run | Runner | clients can request state changes |
| active step | Runner | desktop client can select/request step actions |
| provider session id | Runner | stored for resume |
| provider events | Runner | persisted for audit and replay |
| approval decision | Runner | client submits, runner persists |
| chat rendering state | Client | derived from event stream |
| editor file navigation | desktop client | opens files in the user's IDE via its CLI |

## Event Flow

Normal turn:

```text
Desktop Client
  -> send turn request
Runner
  -> validate workflow run and step
  -> optimize prompt
  -> call ProviderRuntimeGateway
CodexAdapter
  -> start/resume app-server thread
  -> send turn/start
  -> map provider stream to ProviderEvent
Runner
  -> persist events
  -> stream events to clients
  -> run TurnFinalizer after turn_completed
Desktop Client
  -> render final answer, files, tools, artifacts
```

Approval turn:

```text
CodexAdapter
  -> receives provider approval request
  -> emits permission_required
Runner
  -> persists approval request
  -> streams permission_required to clients
Desktop Client
  -> shows approval card
  -> submits decision
Runner
  -> validates decision against policy
  -> persists decision
  -> forwards provider-specific decision to CodexAdapter
CodexAdapter
  -> resumes provider turn
```

## Layer Responsibilities Summary

### 1. Runner Runtime

The runner is the source of truth.

Responsibilities:

- workflow state machine
- workflow run and step state
- prompt optimization
- Provider Runtime Gateway
- Codex app-server adapter
- Claude and Gemini adapter placeholders
- MCP proxy
- approval/proxy policy
- provider session persistence
- normalized provider event persistence
- artifact writer
- summary generator
- Supabase RAG indexer

The runner owns business logic. No client should duplicate workflow rules.

### 2. Admin Web

The React web app should become the admin/configuration surface.

Responsibilities:

- project setup
- workflow configuration
- provider configuration
- MCP/proxy configuration
- YOLO and approval policy configuration
- run history
- audit views
- artifact browsing
- non-latency-critical actions

The admin web can still show workflow run details, but it should not be the main
real-time coding chat.

### 3. Interactive Client UX

The interactive client is a cross-platform Electron desktop app (works alongside
any IDE; optional VS Code/JetBrains plugins may reuse the same webview later).

Responsibilities:

- chat input and streaming response
- `/` commands and skill picker
- workflow selector
- workflow step selector
- approval cards or modals
- file links and changed-file navigation (open in the user's IDE via its CLI)
- tool/MCP activity timeline
- compact run status
- editor-aware context selection

The desktop app consumes FlowPilot runner APIs and normalized event streams. It
must not speak raw Codex app-server JSON-RPC directly.

### Layer Dependency Rule

```text
Admin Web -----------+
                     |
Desktop Client ---+--> Runner Runtime --> Provider Runtime Gateway
                                             |
                                             +--> Codex app-server
                                             +--> Claude adapter
                                             +--> Gemini adapter
```

Allowed dependencies:

- Admin Web depends on runner/admin APIs.
- desktop client depends on runner/interactive APIs.
- Runner depends on provider adapter interfaces.
- Provider adapters depend on provider-specific runtimes.

Disallowed dependencies:

- Admin Web directly calls Codex app-server.
- desktop client directly calls Codex app-server.
- Workflow core imports Codex app-server JSON-RPC types.
- Provider adapters update workflow state directly.

## Provider Runtime Gateway

The runner should define a provider-neutral adapter contract.

```ts
type ProviderKey = "codex" | "claude" | "gemini";

interface ProviderRuntimeAdapter {
  startSession(input: ProviderSessionStartInput): Promise<ProviderSession>;
  resumeSession(input: ProviderSessionResumeInput): Promise<ProviderSession>;
  sendTurn(input: ProviderTurnInput): AsyncIterable<ProviderEvent>;
  submitApproval(input: ProviderApprovalDecisionInput): Promise<void>;
  interrupt(input: ProviderInterruptInput): Promise<void>;
  closeSession(input: ProviderCloseInput): Promise<void>;
  listSkills?(input: ProviderListSkillsInput): Promise<ProviderSkill[]>;
}
```

Normalized event examples:

```ts
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

Provider-specific details stay inside provider adapters.

## Adapter Contract Details

The adapter contract should be concrete enough that Codex, Claude, and Gemini can
share runner logic while still hiding provider-specific protocol differences.

```ts
interface ProviderSessionStartInput {
  workflowRunId: string;
  workflowStepRunId?: string;
  providerKey: ProviderKey;
  workingDirectory: string;
  modelName?: string;
  reasoningEffort?: string;
  requiredMcps?: string[];
  mcpAccessMode?: "read_only" | "read_write";
  yoloMode: boolean;
}

interface ProviderSessionResumeInput {
  workflowRunId: string;
  workflowStepRunId?: string;
  providerSessionId: string;
  providerThreadId?: string;
  workingDirectory: string;
}

interface ProviderTurnInput {
  sessionId: string;
  workflowRunId: string;
  workflowStepRunId: string;
  optimizedPrompt: string;
  workingDirectory: string;
  selectedSkill?: ProviderSkillSelection;
  modelName?: string;
  reasoningEffort?: string;
  requiredMcps?: string[];
  mcpAccessMode?: "read_only" | "read_write";
  yoloMode: boolean;
}

interface ProviderApprovalDecisionInput {
  workflowRunId: string;
  workflowStepRunId?: string;
  providerSessionId: string;
  providerTurnId?: string;
  approvalId: string;
  decision: string;
}

interface ProviderInterruptInput {
  providerSessionId: string;
  providerTurnId?: string;
  reason: string;
}

interface ProviderCloseInput {
  providerSessionId: string;
  reason: "completed" | "cancelled" | "failed" | "shutdown";
}

interface ProviderListSkillsInput {
  providerSessionId?: string;
  workingDirectory: string;
}

interface ProviderSkill {
  name: string;
  path?: string;
  description?: string;
  source: "provider" | "flowpilot" | "workspace";
}

interface ProviderSkillSelection {
  name: string;
  path?: string;
  source: "slash_picker" | "text_shortcut" | "workflow_default";
}
```

## Provider Adapter Responsibilities

| Responsibility | Runner Core | Provider Adapter |
|---|---|---|
| workflow state | owns | receives ids only |
| prompt optimization | owns | receives optimized prompt |
| provider protocol | no provider-specific imports | owns |
| event normalization | defines event schema | maps provider events |
| approval policy | owns | forwards provider approval requests/decisions |
| artifacts/RAG | owns finalizer | emits enough event data |
| skills UI | client/runner owns selection | lists or invokes provider skill when supported |
| session persistence | owns database records | returns provider session/thread ids |

Adapters must not update workflow run or step state directly. They emit events
and return results; the runner decides how workflow state changes.

## Provider-Specific Adapter Shape

### Codex Adapter

Codex is the first implemented adapter.

Runtime:

```text
CodexAdapter
  -> CodexAppServerProcess   (one shared, long-lived process per runner)
  -> CodexJsonRpcClient
  -> codex app-server        (hosts many threads across many cwds)
```

**Scoping and thread model (final decision — see
`05-Codex-AppServer-Migration-Detail.md`):** one shared app-server, a
multi-workspace runner, and `cwd`-per-thread, mirroring the native Codex App
Client. A FlowPilot workspace is a Codex `cwd`; a FlowPilot chat session is a Codex
**thread** (own id + JSONL log on disk). Lean on Codex thread APIs
(`thread/start`, `thread/resume`, `thread/list`, `thread/read`, `thread/archive`)
rather than a custom session registry; FlowPilot persists only the
`(workspace, threadId)` mapping.

Responsibilities:

- start one shared `codex app-server --listen stdio://` per runner
- initialize JSON-RPC
- start/resume/list/read Codex threads, each bound to a `cwd`
- send `turn/start`
- include selected skill input when supported
- map Codex streamed events to `ProviderEvent`
- map Codex approval request to `permission_required`
- submit approval decisions back to Codex
- expose Codex skills through `listSkills`
- persist the `(workspace, threadId)` mapping through the runner session store

### Claude Adapter

Claude must be a separate adapter, not a Codex wrapper.

Initial status:

- registered as placeholder
- disabled for controlled runs until implemented
- exposes capabilities as unavailable or unknown
- returns typed unsupported-provider errors

Future runtime choices:

- Claude Agent SDK for stronger lifecycle, tool, and approval control
- `claude -p` for simpler non-interactive execution if SDK is not used

The future Claude adapter must map Claude session, text, tool-use, permission,
file-change, final-message, and failure events into the same `ProviderEvent`
schema.

### Gemini Adapter

Gemini must also be a separate adapter.

Initial status:

- registered as placeholder
- disabled for controlled runs until implemented
- exposes capabilities as unavailable or lower-confidence
- returns typed unsupported-provider errors

Future runtime choice:

```bash
gemini -p "<optimized prompt>" --output-format stream-json
```

The future Gemini adapter must prove stream-json stability, resume behavior, MCP
event visibility, permission behavior, and non-hanging failure behavior before
it is treated as equivalent to Codex.

## Provider Capability Model

Each provider should advertise capabilities so UI and runner policy do not assume
Codex-level support for every provider.

```ts
interface ProviderCapabilities {
  streaming: boolean;
  resume: boolean;
  approvalEvents: boolean;
  fileEvents: boolean;
  skillSelection: boolean;
  mcp: boolean;
  interrupt: boolean;
}
```

Initial expected values:

| Provider | Status | Notes |
|---|---|---|
| Codex | implemented first | app-server adapter, highest confidence |
| Claude | placeholder | Agent SDK preferred later |
| Gemini | placeholder | CLI stream-json investigation later |

The UI should show disabled or lower-confidence providers clearly.

## Target Flow

```text
Desktop Client
        |
        | start/resume run, send turn, approve permission
        v
FlowPilot Runner Runtime
        |
        +-- prompt optimizer
        +-- workflow state machine
        +-- MCP proxy
        +-- artifact/RAG finalizer
        |
        v
Provider Runtime Gateway
        |
        +-- Codex app-server adapter
        +-- Claude adapter placeholder
        +-- Gemini adapter placeholder
```

## Data Model Additions

Provider session metadata:

```text
workflow_provider_sessions
  id
  workflow_run_id
  workflow_step_run_id nullable
  provider_key
  provider_session_id
  provider_thread_id nullable
  provider_turn_id nullable
  transport
  working_directory
  model_name
  reasoning_effort
  capabilities_json
  status
  last_error nullable
  created_at
  updated_at
```

Provider event telemetry:

```text
workflow_provider_events
  id
  workflow_run_id
  workflow_step_run_id nullable
  provider_session_id
  provider_key
  provider_turn_id nullable
  event_type
  payload_json
  occurred_at
```

Approval records:

```text
workflow_provider_approvals
  id
  workflow_run_id
  workflow_step_run_id nullable
  provider_session_id
  provider_key
  provider_turn_id nullable
  request_payload_json
  available_decisions_json
  selected_decision nullable
  decided_by nullable
  status
  requested_at
  decided_at nullable
```

These tables are provider-runtime telemetry. They should not replace existing
workflow run and step state.

## Error And Recovery States

The runner should model provider runtime failures explicitly.

Provider session status:

- `starting`
- `ready`
- `running`
- `waiting_for_approval`
- `interrupted`
- `failed`
- `completed`
- `closed`

Turn status:

- `queued`
- `running`
- `waiting_for_approval`
- `finalizing`
- `completed`
- `failed`
- `cancelled`

Recovery rules:

- if provider process dies before turn starts, mark turn failed and allow retry
- if stream disconnects during turn, mark session failed and keep persisted
  events
- if finalizer fails after `turn_completed`, keep provider turn completed but
  mark finalization failed for retry
- if approval is pending, clients may reconnect and reload pending approval from
  runner state
- if multiple clients are connected, the runner remains the single decision
  authority

## Design Rules

- Runner owns workflow business logic.
- Admin Web owns configuration and audit views.
- desktop client owns interaction and rendering.
- MCP proxy stays in the runner.
- Codex app-server is inside `CodexAdapter`.
- FlowPilot core consumes normalized events only.
- Provider adapters hide provider-specific protocols.
- Native provider clients remain convenience mode, not the reliable automation
  path.

## MVP Definition

MVP should prove the architecture with Codex first.

MVP includes:

- provider runtime interfaces
- Codex app-server adapter
- provider session/event persistence
- runner event stream
- desktop client workflow/step selector
- desktop client chat and streaming response
- command approval round trip
- file-change rendering with clickable paths
- Admin Web provider/policy configuration and event audit

MVP excludes:

- full Claude implementation
- full Gemini implementation
- signed installers / auto-update distribution for the desktop client
- advanced approval scopes beyond provider-supported basics
- full native provider client parity
