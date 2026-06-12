# 04 - Detailed Coding Plan

## Goal

Implement the new FlowPilot system in slices:

1. update the admin app
2. add cross-platform desktop client (Electron)
3. implement Codex app-server as the first provider adapter
4. keep Claude and Gemini adapter placeholders for later

The implementation must preserve FlowPilot's core value:

- workflow control
- prompt optimization
- artifacts
- summaries
- Supabase RAG
- approval/proxy gates
- MCP proxy enforcement

## Workstream Overview

The coding plan should be executed as parallel-safe workstreams, but merged
through the runner contract.

| Workstream | Primary Output | Depends On |
|---|---|---|
| Runner contracts | provider interfaces, event types, session model | none |
| Persistence | provider session/event/approval tables | runner contracts |
| Codex adapter | app-server lifecycle and event mapping | runner contracts |
| Runner APIs | admin and interactive endpoints | runner contracts, persistence |
| Admin Web | config and audit UI | runner APIs |
| Desktop client (Electron) | coding UX and approvals | runner APIs |
| Claude/Gemini placeholders | registry entries and disabled states | runner contracts |

## Shared Types To Define First

Create shared runtime types before UI work.

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

interface ProviderSession {
  id: string;
  workflowRunId: string;
  workflowStepRunId?: string;
  providerKey: ProviderKey;
  providerSessionId: string;
  providerThreadId?: string;
  status: ProviderSessionStatus;
}

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

interface ProviderSkillSelection {
  name: string;
  path?: string;
  source: "slash_picker" | "text_shortcut" | "workflow_default";
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
```

Provider events should use discriminated unions and must include correlation ids:

```ts
interface ProviderEventBase {
  id: string;
  workflowRunId: string;
  workflowStepRunId?: string;
  providerSessionId: string;
  providerKey: ProviderKey;
  providerTurnId?: string;
  occurredAt: string;
}
```

Every event persisted or streamed to clients should extend this base shape.

### Provider Event Union

Define the event union with enough payload shape for UI rendering and audit.

```ts
type ProviderEvent =
  | (ProviderEventBase & {
      type: "turn_started";
      providerTurnId: string;
    })
  | (ProviderEventBase & {
      type: "message_delta";
      text: string;
    })
  | (ProviderEventBase & {
      type: "message_completed";
      text: string;
    })
  | (ProviderEventBase & {
      type: "tool_started";
      toolName: string;
      input?: unknown;
    })
  | (ProviderEventBase & {
      type: "tool_completed";
      toolName: string;
      output?: unknown;
      status: "success" | "failed" | "cancelled";
    })
  | (ProviderEventBase & {
      type: "file_changed";
      path: string;
      changeType?: "created" | "modified" | "deleted" | "renamed";
    })
  | (ProviderEventBase & {
      type: "permission_required";
      approvalId: string;
      provider: ProviderKey;
      details: unknown;
    })
  | (ProviderEventBase & {
      type: "turn_failed";
      error: string;
      recoverable: boolean;
    })
  | (ProviderEventBase & {
      type: "turn_completed";
      finalMessage: string;
    });
```

### Files To Add For Adapter Contract

Suggested files:

```text
provider-runtime/
  ProviderRuntimeAdapter.ts
  ProviderRuntimeTypes.ts
  ProviderEvents.ts
  ProviderCapabilities.ts
  ProviderRegistry.ts
  UnsupportedProviderAdapter.ts
```

Implementation rule:

- runner core imports only these shared types and registry
- Codex/Claude/Gemini imports stay under their provider folders
- UI receives serialized event DTOs derived from `ProviderEvent`

## 4.1 Update The Admin App

### Intent

Move the React admin app away from being the main coding chat. Keep it as the
configuration, setup, history, and audit surface.

### Scope

Admin Web should keep:

- project configuration
- workflow configuration
- provider configuration
- MCP/proxy configuration
- YOLO and approval policy configuration
- run history
- workflow run detail
- artifact browsing
- provider event audit views

Admin Web should reduce emphasis on:

- primary chat input
- real-time coding interaction
- command approval modals intended for active coding

### Required Changes

1. Add or update pages for provider runtime configuration.
2. Add UI for selecting default provider per project or workflow.
3. Add UI for approval/proxy policy configuration.
4. Add read-only provider event timeline to run detail.
5. Add provider session metadata display.
6. Keep existing workflow run history and artifact views.
7. Add links or setup instructions for connecting the desktop client.

### Suggested Admin Navigation

Admin Web should expose:

- `Settings / Providers`
- `Settings / MCP Proxy`
- `Settings / Approval Policy`
- `Workflows`
- `Workflow Runs`
- `Workflow Run Detail / Events`
- `Workflow Run Detail / Artifacts`
- `Developer Clients / Desktop App`

The goal is to make configuration discoverable without making Admin Web the
primary coding surface.

### Provider Settings UI

Provider settings should include:

- provider key and display name
- enabled/disabled status
- default model
- reasoning effort where supported
- working directory policy
- approval mode
- sandbox mode
- MCP access mode
- provider capability summary
- health/status check

For Claude and Gemini placeholders, show them as unavailable or experimental
instead of hiding them completely.

### Approval Policy UI

Approval policy should separate FlowPilot policy from provider-native behavior.

Fields:

- default approval mode
- YOLO mode visibility
- read-only MCP access
- read-write MCP access
- dangerous command behavior
- allowed command patterns if supported later
- blocked command patterns if supported later
- audit retention

YOLO is the **single source of truth** for approval posture (see §4.5.5 and `05`
"YOLO As SSOT"): the selected YOLO value drives both the FlowPilot runner policy
and the Codex thread/turn sandbox + approval mode. The UI should make this
explicit, and must still not imply that YOLO=true is safe — YOLO=true disables all
gates and must be shown as an explicit, audited posture.

### Run Detail Changes

Run detail should add a provider-runtime timeline:

- provider session started
- turn started
- message deltas summarized or grouped
- tool started/completed
- file changed
- approval requested
- approval decision
- turn completed
- finalizer completed
- errors

This should be read-only audit UI. Live approval should happen in the
interactive client first.

### Admin API Needs

The admin app should call runner/admin APIs for:

- list projects
- list workflows
- list provider configurations
- update provider defaults
- update MCP/proxy configuration
- update approval policy
- list workflow runs
- list provider sessions
- list provider events
- list artifacts and summaries

Suggested endpoint groups:

```text
GET  /admin/providers
PUT  /admin/providers/:providerKey
GET  /admin/provider-capabilities
GET  /admin/mcp-proxy
PUT  /admin/mcp-proxy
GET  /admin/approval-policy
PUT  /admin/approval-policy
GET  /admin/workflow-runs/:runId/provider-sessions
GET  /admin/workflow-runs/:runId/provider-events
GET  /admin/workflow-runs/:runId/provider-approvals
```

### Acceptance Criteria

- Admin Web can configure providers and policies.
- Admin Web can inspect completed and active runs.
- Admin Web can inspect provider events for audit.
- Primary real-time coding UX is not blocked on Admin Web.

### Admin Test Coverage

Add tests for:

- provider settings render enabled and placeholder providers
- approval policy warning text for YOLO/provider boundary
- run detail renders provider event timeline
- provider session metadata appears on run detail
- disabled providers cannot be selected as default

## 4.2 Interactive Client — Cross-Platform Desktop App (Electron)

> **Client decision (final):** the interactive client is a **cross-platform
> Electron desktop app**, not a VS Code extension. One codebase ships Windows +
> macOS (+ Linux) and works alongside any IDE (VS Code, Android Studio, Xcode).
> macOS builds are signed/notarized on a Mac via CI (e.g. GitHub Actions runners);
> mobile is out of scope. Optional VS Code/JetBrains plugins may come later and
> reuse the same React webview. See `05` "Interactive Client Strategy".

### Intent

Build the Interactive Client UX as a desktop app so it is universal across IDEs and
not tied to one editor's extension platform, while still providing fast streaming,
file navigation, and approval UX.

### Stack

Use TypeScript + React, packaged with Electron.

Recommended structure:

```text
apps/desktop-flowpilot/
  package.json
  electron/
    main.ts            # Electron main: window, transport, IDE-CLI file open
    preload.ts         # safe bridge exposed to the renderer
  src/                 # React renderer (IDE-agnostic; talks only to the runner)
    App.tsx
    runnerClient.ts    # HTTP/WebSocket transport to FlowPilot Runner
    components/
```

Use:

- Electron main process for window lifecycle, OS integration, auto-update, and the
  IDE-CLI bridge that opens files in the user's editor (`code -g` / `studio` / `xed`)
- React renderer for rich UI — **no Electron/IDE specifics inside it**, so it stays
  portable to a future VS Code/JetBrains plugin
- HTTP/WebSocket/local transport to talk to FlowPilot Runner

### App Shell Details

Initial actions (renderer → main via the preload bridge):

- `connectRunner`
- `openChat`
- `selectWorkflow`
- `selectStep`
- `startRun`
- `resumeRun`
- `approveAction`
- `openRunHistory`
- `openFileInIde`  (main shells out to the detected IDE CLI)

Initial views (renderer):

- project/workflow/step navigator
- chat panel
- run timeline panel

Configuration (persisted by main):

- `runnerUrl`
- `workspaceId`
- `defaultProvider`
- `autoOpenChat`
- `preferredIde`  (which IDE CLI to use for file open)

### Core UI

The desktop app should provide:

- project selector
- workflow selector
- workflow step selector
- active run and active step display
- chat input
- streaming assistant output
- `/` command menu
- skill picker
- command approval card/modal
- file change list
- run timeline
- tool and MCP activity rows
- clickable file links (open in the user's IDE)

### UX Flow

Typical user flow:

1. user opens the FlowPilot desktop app and selects a workspace
2. desktop app connects to local FlowPilot Runner
3. user selects project/workflow/step
4. user types a prompt or `/` command
5. desktop app sends the turn request to runner
6. runner streams normalized events
7. desktop app renders assistant text, tools, commands, files, and approvals
8. user approves/denies provider requests when needed
9. desktop app opens changed files in the user's IDE via its CLI (`code`/`studio`/`xed`)
10. runner finalizes artifacts, summary, and RAG

### Slash Command And Skill UX

`/` should open a client-side command palette.

Initial commands:

- `/workflow` select workflow
- `/step` select step
- `/skill` select provider skill
- `/resume` resume current run
- `/artifacts` show artifacts
- `/history` show run history

Skill selection flow:

1. desktop app asks runner for provider capabilities and available skills
2. user selects a skill
3. desktop app sends selected skill metadata with the turn
4. runner passes selected skill to the provider adapter when supported
5. provider event log records selected skill for audit

### Development Mode

The desktop app should be usable during development:

1. `npm install` in `apps/desktop-flowpilot/`
2. `npm run dev` to launch Electron with the React renderer (hot reload)
3. connect to local FlowPilot Runner
4. test workflow selection, chat, streaming, approvals, and file opening
5. the renderer hot-reloads; restart Electron after main-process changes

### Runner Client Contract

The desktop app should talk to FlowPilot, not directly to provider runtimes.

Required operations:

- list projects
- list workflows
- list workflow steps
- start workflow run
- resume workflow run
- send turn
- stream normalized provider events
- submit approval decision
- list or open artifacts
- resolve workspace file path (then open it in the user's IDE via its CLI)

Suggested interactive endpoints:

```text
GET  /client/projects
GET  /client/projects/:projectId/workflows
GET  /client/workflows/:workflowId/steps
POST /client/workflow-runs
POST /client/workflow-runs/:runId/resume
POST /client/workflow-runs/:runId/turns
GET  /client/workflow-runs/:runId/events/stream
POST /client/approvals/:approvalId/decision
GET  /client/workflow-runs/:runId/artifacts
GET  /client/provider-capabilities
GET  /client/provider-skills?provider=codex
```

### Rendering Rules

- Show final assistant response separately from logs.
- Collapse command output by default.
- Render `file_changed` as file rows.
- Display short file names by default.
- Keep full path in tooltip/details/copy action.
- Click file rows to open files in the user's IDE (via its CLI).
- Render `permission_required` as an approval card.

### Acceptance Criteria

- Developer can select workflow and step in the desktop app.
- Developer can send a prompt and receive streamed response.
- Developer can approve or deny command requests.
- Developer can open changed files in their IDE from the response/timeline.
- Desktop app uses normalized FlowPilot events only.
- One codebase builds and runs on Windows and macOS.

### Desktop App Test Coverage

Add tests or manual verification for:

- app launches on Windows and macOS
- runner connection failure is shown clearly
- workflow selector loads data from runner
- step selector updates active step
- event stream renders message deltas
- `permission_required` renders approval card
- approval decision posts back to runner
- file row opens in the user's IDE via its CLI
- slash command picker inserts selected action/skill

## 4.3 Code App-Server As Codex Adapter

### Intent

Implement Codex app-server as the first high-control provider adapter behind the
Provider Runtime Gateway.

### Adapter Boundary

Codex-specific JSON-RPC and event names stay inside `CodexAdapter`.

FlowPilot core should only see:

- `ProviderRuntimeAdapter`
- `ProviderSession`
- `ProviderTurnInput`
- `ProviderEvent`

### Adapter Modules

Recommended module split:

```text
provider-runtime/
  ProviderRuntimeAdapter.ts
  ProviderRegistry.ts
  ProviderEvents.ts
  TurnFinalizer.ts
  codex/
    CodexAdapter.ts
    CodexAppServerProcess.ts
    CodexJsonRpcClient.ts
    CodexEventMapper.ts
    CodexApprovalBridge.ts
    CodexSkillMapper.ts
  claude/
    ClaudeAdapter.ts
  gemini/
    GeminiAdapter.ts
```

The exact paths should follow the existing repo structure, but the boundaries
should stay the same.

### Codex Adapter Interface Implementation

`CodexAdapter` should implement every method in `ProviderRuntimeAdapter`.

```ts
class CodexAdapter implements ProviderRuntimeAdapter {
  async startSession(input: ProviderSessionStartInput): Promise<ProviderSession>;
  async resumeSession(input: ProviderSessionResumeInput): Promise<ProviderSession>;
  sendTurn(input: ProviderTurnInput): AsyncIterable<ProviderEvent>;
  async submitApproval(input: ProviderApprovalDecisionInput): Promise<void>;
  async interrupt(input: ProviderInterruptInput): Promise<void>;
  async closeSession(input: ProviderCloseInput): Promise<void>;
  async listSkills(input: ProviderListSkillsInput): Promise<ProviderSkill[]>;
}
```

Method behavior:

- `startSession`: start/connect app-server, initialize, create provider session
- `resumeSession`: reattach to existing provider thread/session
- `sendTurn`: send optimized prompt and stream normalized events
- `submitApproval`: forward approval decision to app-server
- `interrupt`: request provider interrupt/steer when supported
- `closeSession`: close local process/session resources
- `listSkills`: expose Codex skills to runner/client skill picker

### Codex Internal Interfaces

Keep app-server protocol types local to the Codex folder.

```ts
interface CodexJsonRpcClient {
  initialize(): Promise<void>;
  startThread(input: CodexThreadStartInput): Promise<CodexThread>;
  resumeThread(input: CodexThreadResumeInput): Promise<CodexThread>;
  startTurn(input: CodexTurnStartInput): AsyncIterable<CodexAppServerEvent>;
  submitApproval(input: CodexApprovalDecision): Promise<void>;
  interrupt(input: CodexInterruptInput): Promise<void>;
  listSkills(input: CodexListSkillsInput): Promise<CodexSkill[]>;
  close(): Promise<void>;
}

interface CodexEventMapper {
  toProviderEvent(event: CodexAppServerEvent): ProviderEvent | null;
}
```

Only `CodexAdapter` should translate between Codex protocol types and
FlowPilot's provider-neutral types.

### Process Lifecycle

Codex app-server process management should handle:

- start process
- initialize protocol
- detect startup failure
- detect process exit
- restart only when safe
- close on session end when owned by runner
- leave external app-server alone when runner only connects to it
- capture stderr for diagnostics without mixing it into assistant response

### Runtime Flow

1. Start or connect to `codex app-server --listen stdio://`.
2. Send initialize request.
3. Start or resume Codex thread.
4. Send `turn/start` with the optimized prompt.
5. Include selected skill input when the client chooses a skill.
6. Read streamed Codex events.
7. Map Codex events to normalized `ProviderEvent`.
8. Emit `permission_required` when Codex asks for approval.
9. Accept approval decisions from FlowPilot and send them back to Codex.
10. Emit `turn_completed` with final message.
11. Persist provider thread/session ids.

### Approval Bridge

Approval bridge responsibilities:

1. receive provider approval request
2. create FlowPilot approval record
3. emit `permission_required`
4. wait for client decision
5. validate decision against FlowPilot policy
6. send provider-specific decision to Codex app-server
7. persist decision and outcome

Approval requests should survive client reconnects. The pending approval state
belongs in the runner, not only in the client.

### Event Mapping

Initial normalized mapping:

- Codex turn started -> `turn_started`
- Codex assistant text delta -> `message_delta`
- Codex assistant message complete -> `message_completed`
- Codex tool or MCP call started -> `tool_started`
- Codex tool or MCP call completed -> `tool_completed`
- Codex command execution (start/end) -> `tool_started`/`tool_completed` with exit
  status — a distinct mapped event, separate from MCP tool calls and from the
  assistant message (so command output never mixes into the final answer)
- Codex file change item -> `file_changed`
- Codex approval request -> `permission_required`
- Codex turn error -> `turn_failed`
- Codex turn complete -> `turn_completed`

### Event Persistence

Every normalized event should be:

- assigned a FlowPilot event id
- correlated to workflow run and step
- correlated to provider session
- persisted before or while streaming to clients
- replayable for clients that reconnect

The desktop client should be able to recover the current run timeline from
persisted events even if it missed live stream messages.

### Turn Finalizer

After `turn_completed`, run a provider-neutral finalizer:

1. save final response artifact
2. snapshot changed files or git diff
3. generate summary
4. push summary/artifact metadata to Supabase RAG
5. update workflow step status
6. persist provider event audit trail

Finalizer failure should not erase the completed provider turn. Mark finalizer
state separately so it can be retried.

### Tests

Add tests for:

- start session
- resume session
- send turn
- event mapping
- approval request mapping
- approval decision forwarding
- turn finalizer
- failure handling
- provider session persistence

Add contract tests using fake provider events:

- message-only turn
- tool-call turn
- file-change turn
- approval-required turn
- failed turn
- interrupted turn
- reconnect and replay persisted events

### Acceptance Criteria

- Codex app-server can run a controlled workflow turn.
- FlowPilot captures streaming message events.
- FlowPilot captures final response.
- FlowPilot receives dangerous command approval requests.
- FlowPilot can send approval decision back.
- FlowPilot persists provider session and event records.
- Artifacts and RAG finalization run after turn completion.

### Codex Adapter Milestones

1. process starts and initialize succeeds
2. thread start/resume works
3. simple prompt returns final message
4. streaming message deltas render in client
5. file event maps to `file_changed`
6. command approval maps to `permission_required`
7. approval decision resumes turn
8. provider session survives resume
9. finalizer runs after `turn_completed`

## 4.4 Keep Claude And Gemini Adapter Placeholders

### Intent

Do not hard-code Codex into FlowPilot core. Keep Claude and Gemini paths visible
from the beginning, even if implementation comes later.

### Placeholder Interface Implementation

Create adapter classes or modules that implement the same
`ProviderRuntimeAdapter` contract:

```ts
class ClaudeAdapter implements ProviderRuntimeAdapter {
  async startSession(input: ProviderSessionStartInput): Promise<ProviderSession> {
    throw new UnsupportedProviderRuntimeError("claude");
  }

  async resumeSession(input: ProviderSessionResumeInput): Promise<ProviderSession> {
    throw new UnsupportedProviderRuntimeError("claude");
  }

  sendTurn(input: ProviderTurnInput): AsyncIterable<ProviderEvent> {
    throw new UnsupportedProviderRuntimeError("claude");
  }

  async submitApproval(input: ProviderApprovalDecisionInput): Promise<void> {
    throw new UnsupportedProviderRuntimeError("claude");
  }

  async interrupt(input: ProviderInterruptInput): Promise<void> {
    throw new UnsupportedProviderRuntimeError("claude");
  }

  async closeSession(input: ProviderCloseInput): Promise<void> {
    throw new UnsupportedProviderRuntimeError("claude");
  }
}

class GeminiAdapter implements ProviderRuntimeAdapter {
  async startSession(input: ProviderSessionStartInput): Promise<ProviderSession> {
    throw new UnsupportedProviderRuntimeError("gemini");
  }

  async resumeSession(input: ProviderSessionResumeInput): Promise<ProviderSession> {
    throw new UnsupportedProviderRuntimeError("gemini");
  }

  sendTurn(input: ProviderTurnInput): AsyncIterable<ProviderEvent> {
    throw new UnsupportedProviderRuntimeError("gemini");
  }

  async submitApproval(input: ProviderApprovalDecisionInput): Promise<void> {
    throw new UnsupportedProviderRuntimeError("gemini");
  }

  async interrupt(input: ProviderInterruptInput): Promise<void> {
    throw new UnsupportedProviderRuntimeError("gemini");
  }

  async closeSession(input: ProviderCloseInput): Promise<void> {
    throw new UnsupportedProviderRuntimeError("gemini");
  }
}
```

Placeholders should return explicit unsupported-provider errors until implemented.

Placeholder behavior:

- provider appears in registry
- provider exposes capabilities as disabled or unknown
- provider cannot be selected for controlled runs unless enabled
- direct use returns a typed unsupported-provider error
- Admin Web can show planned support
- desktop client can show provider unavailable state

### Unsupported Provider Error

Use a typed error so runner APIs and UI can render this clearly.

```ts
class UnsupportedProviderRuntimeError extends Error {
  constructor(providerKey: ProviderKey) {
    super(`${providerKey} controlled runtime is not implemented yet`);
    this.name = "UnsupportedProviderRuntimeError";
  }
}
```

### Claude Future Direction

Preferred options:

1. Claude Agent SDK for stronger lifecycle and approval control.
2. `claude -p` for simpler non-interactive execution.

Claude adapter must eventually normalize:

- session id
- streamed assistant text
- tool-use events
- permission/approval requests
- final message
- changed files or artifact paths

Claude should not be implemented by pretending it is Codex. It needs its own
adapter and tests.

Future Claude adapter shape:

```text
claude/
  ClaudeAdapter.ts
  ClaudeRuntimeClient.ts
  ClaudeEventMapper.ts
  ClaudeApprovalBridge.ts
  ClaudeSessionMapper.ts
```

Expected implementation behavior:

- `startSession`: create or continue Claude session
- `resumeSession`: attach to stored Claude session id when supported
- `sendTurn`: send optimized prompt and stream normalized events
- `submitApproval`: use SDK/hook approval callback if available
- `interrupt`: use SDK cancellation or process interrupt when available
- `closeSession`: close SDK/process resources

If the first implementation uses `claude -p`, mark capabilities lower:

- `streaming`: only if structured streaming is reliable
- `approvalEvents`: only if permission callbacks are exposed
- `fileEvents`: only if events or reliable diffs are available
- `resume`: only if continuation/session ids are reliable

### Gemini Future Direction

Initial option:

```bash
gemini -p "<optimized prompt>" --output-format stream-json
```

Gemini adapter must verify:

- stream-json schema stability
- session resume behavior
- MCP event visibility
- permission behavior
- non-hanging failure behavior

Gemini should start as a lower-confidence adapter until stream, resume, MCP, and
approval behavior are proven.

Future Gemini adapter shape:

```text
gemini/
  GeminiAdapter.ts
  GeminiCliProcess.ts
  GeminiStreamJsonParser.ts
  GeminiEventMapper.ts
  GeminiSessionMapper.ts
```

Expected implementation behavior:

- `startSession`: configure CLI invocation/session metadata
- `resumeSession`: attach to Gemini session when reliable support exists
- `sendTurn`: run `gemini -p` with stream-json output and map events
- `submitApproval`: only supported if Gemini exposes approval callbacks/events
- `interrupt`: cancel process or use provider-supported interrupt
- `closeSession`: stop process and persist final state

Initial Gemini capabilities should be conservative until proven by tests.

### Provider Registry

Add a provider registry that can expose:

- provider key
- display name
- availability
- supported features
- adapter factory

Example feature flags:

- `streaming`
- `resume`
- `approvalEvents`
- `fileEvents`
- `skillSelection`
- `mcp`

Provider registry example:

```ts
interface ProviderRegistration {
  key: ProviderKey;
  displayName: string;
  status: "available" | "disabled" | "placeholder";
  capabilities: ProviderCapabilities;
  createAdapter: () => ProviderRuntimeAdapter;
}
```

### Acceptance Criteria

- FlowPilot core depends on provider interfaces, not Codex classes.
- Codex adapter is registered as implemented.
- Claude and Gemini are registered as placeholders or disabled providers.
- UI can show unavailable providers clearly.
- Future provider work does not require rewriting runner core.

## 4.5 Codex App-Server Adapter — Detailed Go Implementation (Code-Grounded)

This section grounds `05`'s work items W1–W8 and the **YOLO-as-SSOT approval model**
into concrete changes in the Go runner (`apps/local-runner`). It is implementation
guidance only — no code is written yet. Symbols referenced are real
(`file:line`); new symbols are named so the build is unambiguous.

### 4.5.0 Package layout

Build inside the existing `internal/runner` package first (to reuse the unexported
`LiveSession` helpers and JSON-RPC functions in `sessions.go`); extract to a
`codexappserver` subpackage later if needed. New files:

```text
apps/local-runner/internal/runner/
  provider_event.go          # normalized ProviderEvent types + constants
  codex_appserver.go         # shared process mgmt + Codex JSON-RPC methods
  codex_event_mapper.go      # Codex notification -> ProviderEvent
  codex_approval_bridge.go   # permission_required + decision round-trip
  yolo_resolver.go           # YOLO -> Codex config + runner policy (SSOT)
  provider_session_store.go  # (workspace, threadId) persistence
```

Reuse as-is: `commandContextFn` (`sessions.go:22`), `writeJsonRpcRequest`
(`sessions.go:120`), `readJsonRpcResponseWithHandler` (`sessions.go:157`),
`LiveSession` (`sessions.go:32`), `SessionStreamCallback` (used by
`SendMessageWithCallback`, `sessions.go:653`).

### 4.5.1 Normalized events (W4) — `provider_event.go`

Go has no discriminated unions; use a struct with a `Type` discriminator and a
correlation base:

```text
type ProviderEventType string
const ( TurnStarted ProviderEventType = "turn_started"; MessageDelta = "message_delta";
        MessageCompleted; ToolStarted; ToolCompleted; FileChanged;
        PermissionRequired; TurnFailed; TurnCompleted )

type ProviderEvent struct {
  ID, WorkflowRunID, WorkflowStepRunID, ProviderSessionID, ProviderKey,
  ProviderTurnID string
  OccurredAt time.Time
  Type ProviderEventType
  // type-specific payload fields (Text, ToolName, Path, ApprovalID, Error, FinalMessage, …)
}
```

### 4.5.2 Shared app-server process (W1) — `codex_appserver.go`

- Extend `Runner` (`runner.go:121`): add `codexAppServer *codexAppServerHandle`
  guarded by a mutex. `Runner.workspace` (`runner.go:122`) is demoted to a default
  cwd; per-thread `cwd` is authoritative.
- `ensureCodexAppServer(ctx) (*codexAppServerHandle, error)`: start **one**
  `codex app-server --listen stdio://` via `commandContextFn` if not running;
  reuse otherwise. Wire stdin/stdout like `StartSession` (`sessions.go:448`); keep
  a `bufio.Scanner` read loop.
- The process is workspace-agnostic; the cwd is supplied per `thread/start`.

### 4.5.3 Codex JSON-RPC methods (W2) — `codex_appserver.go`

- Add Codex payload builders next to `geminiACP*Params` (`sessions.go:265`):
  `codexInitializeParams`, `codexThreadStartParams(cwd, sandbox, approvalMode,…)`,
  `codexThreadResumeParams(threadId)`, `codexThreadListParams(cwd)`,
  `codexThreadReadParams(threadId)`, `codexTurnStartParams(threadId, prompt, skill)`,
  `codexApprovalDecisionParams(requestId, decision)`.
- A single **read-loop goroutine** demuxes by JSON-RPC `id` (responses) vs `method`
  (notifications). Notifications fan out to the event mapper and approval bridge.

### 4.5.4 Thread mapping + persistence (W3) — `provider_session_store.go`

- Persist only `(workspace, threadId, providerThreadId, model, status)` per the
  `03` data model (`workflow_provider_sessions`). For the local runner, a small
  JSON/SQLite store under `.flowpilot/` is sufficient; reuse the existing workspace
  file conventions.
- `StartThread(ctx, workspace, yolo, model)`, `ListThreads(ctx, workspace)` →
  `thread/list` filtered by cwd, `ResumeThread(ctx, threadId)` → `thread/resume`.
  Codex owns the thread JSONL log; FlowPilot keeps only the mapping.

### 4.5.5 YOLO resolver (W5, SSOT) — `yolo_resolver.go` ⭐

The key new piece. One value configures both layers:

```text
type YoloPosture struct {
  CodexSandbox      string // "full-access"      | "workspace-write"
  CodexApprovalMode string // "never"            | "on-request"
  RunnerAutoApprove bool   // true               | false
}

func resolveYoloPosture(yolo bool) YoloPosture {
  if yolo { return YoloPosture{"full-access", "never", true} }
  return YoloPosture{"workspace-write", "on-request", false}
}
```

- **Input source:** the per-step YOLO value already flows in request types —
  `PromptExecutionRequest.YoloMode` (`types.go:469`) and the session-start request
  carry it; the session-start request already has `ApprovalMode` and `AllowWrite`
  (`types.go:511-512`) to populate. Resolve YOLO per workflow run/step
  (Task-030/031) and carry it on the turn.
- **Applied at:** `CodexSandbox` → `codexThreadStartParams`; `CodexApprovalMode` →
  `codexThreadStartParams`/`codexTurnStartParams` (confirm which the installed
  Codex accepts); `RunnerAutoApprove` → approval bridge behavior.
- **Retire the hack:** stop writing a fixed `default_tools_approval_mode = "approve"`
  in `ensureCodexGoogleDriveMcpConfig` (`google_drive_mcp_provider_config.go:275`).
  App-server approval mode is now set per thread/turn from `resolveYoloPosture`, not
  pinned in the MCP config file. The proxy MCP gate keeps keying on YOLO (CP-29
  P-5/P-11) — that part is unchanged.

### 4.5.6 Approval bridge (W5) — `codex_approval_bridge.go`

- On a `permission_required` notification from the read loop:
  1. create an approval record (`workflow_provider_approvals`, `03` data model);
  2. emit a `PermissionRequired` `ProviderEvent` via the stream callback;
  3. **block the turn** on a per-`approvalId` decision channel held in a runner map
     (survives client reconnect).
- `SubmitApprovalDecision(approvalId, decision)`: validate against FlowPilot
  policy, send `codexApprovalDecisionParams` back over JSON-RPC, unblock, persist
  outcome. Mirror the existing TS use case `SubmitApprovalDecisionUseCase`
  (`apps/admin-web/src/domain/usecase/approvals/submit-approval-decision-usecase.ts`).
- If `RunnerAutoApprove` (YOLO=true): never reached in practice (Codex `never`
  approval mode means it does not ask); if a request still arrives, auto-approve and
  record it.

### 4.5.7 Event mapper (W4) — `codex_event_mapper.go`

Map Codex turn/item notifications to `ProviderEvent` (the table in §4.3 "Event
Mapping" applies): turn started → `turn_started`, text delta → `message_delta`,
message complete → `message_completed`, tool/MCP call → `tool_started`/
`tool_completed`, file change → `file_changed`, approval → `permission_required`,
error → `turn_failed`, complete → `turn_completed`. Emit through
`SessionStreamCallback`.

### 4.5.8 Chat path migration (W6)

- Add `Runner.ExecuteTurn(ctx, req) (PromptExecutionResult, error)`:
  `ensureCodexAppServer` → ensure thread (start/resume) → `resolveYoloPosture` →
  optimize prompt (existing optimizer, before `turn/start`) → `turn/start` → stream
  events via callback → `TurnFinalizer`.
- Repoint callers of `ExecutePrompt` (`runner.go:852`): the HTTP route at
  `internal/cli/root.go:1239` and the admin-web caller
  `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts` (via the
  local-runner gateway `executePrompt`). Add an `/execute-turn` endpoint or
  feature-flag the existing path.
- Keep `ExecutePrompt` as the fallback adapter (W8) until parity is proven.

### 4.5.9 Lifecycle & failure states (W7)

Formalize `LiveSession.Status` (`sessions.go:53`) to the `03` set
(`starting/ready/running/waiting_for_approval/interrupted/failed/completed/closed`)
and apply the recovery rules from `03` (process death, stream disconnect, finalizer
failure, pending-approval reconnect).

### 4.5.10 Sequences

```text
YOLO=false, dangerous terminal command:
  ExecuteTurn -> thread/start(cwd, sandbox=workspace-write, approval=on-request)
             -> turn/start(optimizedPrompt)
  Codex -> permission_required(exec: "rm -rf …")
  bridge -> persist + emit PermissionRequired + BLOCK
  user  -> deny
  bridge -> codexApprovalDecision(deny) ; command NOT executed
  Codex -> turn_completed (or turn_failed) -> finalize

YOLO=true:
  ExecuteTurn -> thread/start(cwd, sandbox=full-access, approval=never)
             -> turn/start(optimizedPrompt)
  Codex -> runs commands directly, no permission_required
        -> turn_completed -> finalize ; audit record: gating-disabled
```

### 4.5.11 Go tests to add

Follow the `sessions_test.go` pattern (mock `commandContextFn` with a scripted
process, `sessions_test.go:89-129`). Cover the `06` test ids:

- `T-01/T-05/T-07` start, stream deltas, final message separation.
- `T-08/T-09/T-22` `permission_required` mapping; deny blocks; approve runs.
- `T-21/T-23/T-24/T-25` YOLO posture: full-access/never vs workspace-write/on-request;
  per-turn posture; gating-disabled audit; no standalone `="approve"`.
- `T-13/T-15` `thread/list` by cwd; `thread/resume` after restart.
- `T-26` command execution maps to distinct `tool_started`/`tool_completed` + exit status.
- `T-27/T-28` finalizer-failure retry (turn stays completed); recoverable `turn_failed` re-send.
- `T-19/T-20` fallback `ExecutePrompt` and existing flows unchanged.

### 4.5.12 Sub-step order

1. `provider_event.go` types.
2. `codex_appserver.go` process + `initialize` (T-01).
3. `thread/start` + `turn/start` + read loop + event mapper (T-05/T-07).
4. `yolo_resolver.go` + apply to thread/turn config (T-21/T-25).
5. `codex_approval_bridge.go` + decision round-trip (T-08/T-09/T-22).
6. `provider_session_store.go` + `thread/list`/`thread/resume` (T-13/T-15).
7. `ExecuteTurn` + migrate callers, keep fallback (T-19/T-20).
8. Lifecycle states + finalizer wiring + audit record (T-24).

## Implementation Order

1. Define provider runtime interfaces and event types.
2. Add provider session and provider event persistence.
3. Add provider registry with Codex implemented and Claude/Gemini placeholders.
4. Implement Codex app-server adapter.
5. Add provider-neutral turn finalizer.
6. Expose runner APIs and event streaming for interactive clients.
7. Update Admin Web for configuration and audit.
8. Build the Electron desktop client MVP.
9. Add approval UI and approval decision round trip.
10. Add file/path rendering and workflow/step UX polish.

## Suggested Delivery Phases

### Phase 1: Runner Contract And Persistence

- define shared provider types
- add provider registry
- add session/event/approval persistence
- add fake provider adapter for tests
- add event replay API

### Phase 2: Codex Adapter MVP

- start app-server
- initialize JSON-RPC
- start/resume thread
- send simple turn
- stream message deltas
- persist final message

### Phase 3: Approval And Finalizer

- map provider approval request
- add approval records
- add approval decision endpoint
- forward decision to Codex
- add TurnFinalizer
- add retry state for finalizer failures

### Phase 4: Admin Web Update

- provider settings page
- approval policy page
- run detail provider timeline
- provider sessions and approval audit
- desktop client setup page

### Phase 5: Desktop Client MVP (Electron)

- Electron + React scaffold (`apps/desktop-flowpilot/`)
- runner connection
- workflow/step selector
- chat UI
- event stream rendering
- file links via IDE CLI
- approval card

### Phase 6: Provider Placeholders And Hardening

- Claude placeholder
- Gemini placeholder
- provider capability UI
- reconnect/replay behavior
- process crash handling
- documentation and migration notes

## Cross-Cutting Test Plan

End-to-end scenarios:

1. select workflow and step in the desktop app, send turn, receive final answer
2. provider requests command approval, user approves, turn completes
3. provider requests command approval, user denies, turn fails or continues
   according to provider behavior
4. file change event appears and opens in the user's IDE
5. desktop app disconnects and reconnects, timeline replays
6. finalizer fails and can be retried
7. Admin Web shows provider event audit for the same run
8. Claude/Gemini placeholders are visible but unavailable

## Resolved Implementation Decisions

See `05-Codex-AppServer-Migration-Detail.md` for the canonical, code-grounded
versions of these.

- **App-server scoping** → **one shared app-server, multi-workspace runner,
  `cwd`-per-thread**, mirroring the native Codex App Client. Not per workspace and
  not per run. A single app-server process hosts many threads across many `cwd`s.
- **Session model** → a FlowPilot chat session **is a Codex thread**; a FlowPilot
  workspace **is a Codex `cwd`**. Lean on Codex thread APIs (`thread/start`,
  `thread/resume`, `thread/list`, `thread/read`, `thread/archive`); FlowPilot
  persists only the `(workspace, threadId)` mapping. Do not build a custom session
  registry. A provider session therefore maps to a thread, not rigidly to a run or
  step; runs/steps reference the thread(s) they used.

## Open Implementation Questions

- Should runner expose HTTP/WebSocket or local socket first?
- How should multiple desktop app windows attach to the same run/thread?
- What approval decisions are supported by the current Codex app-server version?
- How should desktop client authentication to local runner work?
- How should the multi-workspace runner register/bind workspaces, and how do
  clients select the active `cwd`?
- How much provider event payload should be persisted for audit versus storage
  size?

## Risk Notes

- App-server improves approval UX but is not a complete security model.
- FlowPilot must keep sandbox, provider approval, proxy, and YOLO policy
  explicit.
- The desktop client must not duplicate workflow business logic.
- Admin Web should not be deleted; it remains important for configuration and
  audit.
- Claude and Gemini should not be forced through Codex app-server.
