# SD-12: Refactor Workflow With Session

## 1. Goal

Translate `SS-11-Workflow-With_Session` into a concrete technical design.

The current system already has:

- workflow definitions
- workflow runs
- workflow steps
- artifact persistence
- provider/model routing

The refactor adds:

- session-aware execution policy
- long-lived provider adapters
- session persistence
- follow-up execution that can continue the same provider session

This refactor must preserve the existing workflow engine as the top-level orchestrator.

---

## 2. Current Problem

Current execution shape:

1. `workflow-start-runtime` resolves step
2. it assembles prompt
3. it calls `localRunnerGateway.executePrompt(...)`
4. local runner starts provider CLI process
5. process exits after one response

Observed current behavior:

- initial step execution uses a fresh provider command
- follow-up prompt also uses a fresh provider command
- `flowId` exists in the request contract but is not used as real provider session reuse

This means the current architecture is:

- workflow-aware
- artifact-aware
- not session-aware

---

## 3. New Runtime Model

The workflow engine must manage both:

- workflow state
- AI session state

### 3.1 New Entities

Add a runtime entity:

- `workflow_run_session`

Suggested shape:

```text
workflow_run_sessions
- id
- workflow_run_id
- provider
- model
- transport_type
- provider_session_id
- process_key
- status
- metadata_json
- started_at
- completed_at
```

Notes:

- `provider_session_id` is the remote or provider-owned session/thread id
- `process_key` is the local process identity used by the runner when the transport is long-lived
- `metadata_json` stores adapter-specific fields

Durability rule:

- `workflow_run_id` is the stable FlowPilot identity
- `provider_session_id` is optional runtime state
- a new provider session may be created later for the same workflow run if the old provider session no longer exists

### 3.2 Step Execution Policy

Session behavior is derived from the existing `subagent` field.

Rules:

- `subagent = null` -> shared workflow session
- `subagent != null` -> isolated step session

Notes:

- do not add a separate `workflow_steps.execution_mode`
- the shared path is always the implicit workflow run `main` session

For single-step launches, runtime must still resolve these fields from the selected step definition.

### 3.3 Cross-Provider Session Rule

Main-session steps and subagent steps may use different providers because provider is still resolved per step from the configured model.

Example:

- main workflow session starts on Codex because step model is `gpt-5.x`
- coding step has `subagent` and uses Gemini because step model is `gemini-*`
- runner starts an isolated Gemini session for the coding step
- coding step artifact is persisted normally
- next non-subagent step resumes the original Codex main session and consumes the Gemini artifact through prompt assembly

Design rule:

- never try to migrate one live provider session across providers
- cross-provider handoff happens through persisted artifacts and prompt/context assembly

---

## 4. Adapter Contract

The old runner contract is too narrow:

```ts
executePrompt(request) -> result
```

We need a session-aware adapter contract:

```ts
type AiSessionStartRequest = {
  providerKey: string;
  modelName: string;
  reasoningEffort: string | null;
  workingDirectory: string;
  approvalMode: string | null;
  allowWrite: boolean;
};

type AiSessionHandle = {
  transportType: string;
  providerSessionId: string;
  processKey: string | null;
};

type AiSessionMessageRequest = {
  session: AiSessionHandle;
  prompt: string;
  skillIds: string[];
  contextSourceIds: string[];
};

startSession(request) -> AiSessionHandle
sendMessage(request) -> PromptExecutionResult
closeSession(session) -> void
```

Compatibility rule:

- `executePrompt(...)` remains as fallback for providers that do not support long-lived sessions

---

## 5. Provider Transport Design

### 5.1 Codex Transport

Transport type:

- `codex_mcp`

Implementation:

1. runner starts one `codex mcp-server` process
2. runner initializes MCP JSON-RPC
3. first message uses `tools/call` with tool `codex`
4. response returns `threadId`
5. later messages use tool `codex-reply` with the same `threadId`

Store:

- `provider_session_id = threadId`
- `process_key = local runner process id or internal handle`

Important:

- do not use raw interactive `codex` stdin piping for this feature
- it fails in non-terminal runner conditions

### 5.2 Claude Transport

Transport type:

- `claude_stream_json`

Implementation:

1. runner starts one long-lived Claude process:
   `claude -p --verbose --input-format stream-json --output-format stream-json`
2. runner writes JSONL user messages to stdin
3. runner parses JSONL events from stdout
4. final result event contains `session_id`
5. later prompts are written into the same process stdin

Store:

- `provider_session_id = session_id`
- `process_key = local runner process id or internal handle`

Important:

- the current verified machine behavior shows same process + same `session_id`
- `claude mcp serve` is not the recommended path for FlowPilot session execution

### 5.3 Gemini Transport

Transport type:

- `gemini_acp`

Implementation:

1. runner starts one `gemini --acp` process
2. runner sends `initialize`
3. runner sends `session/new`
4. result returns `sessionId`
5. runner sends `session/prompt` for the first and later prompts
6. runner consumes `session/update` message chunks plus final result

Store:

- `provider_session_id = sessionId`
- `process_key = local runner process id or internal handle`

Fallback:

- `gemini --resume` is valid for cross-process continuation
- ACP remains the preferred transport because it avoids respawning the provider process

---

## 6. Workflow Runtime Resolution

### 6.1 Session Resolution Algorithm

Before executing a step:

1. resolve provider and model using existing workflow logic
2. resolve `subagent`
3. if `subagent` exists, create a new session record unless a live step-owned session already exists
4. if `subagent` is null, load or create the shared main session record for `workflow_run_id`
5. send step prompt through that session adapter

Pseudocode:

```text
subagent = step.subagent
sessionKey =
  subagent == null
    ? workflowRunId + ":main"
    : workflowRunStepId

session = findLiveSession(sessionKey)
if not found:
  session = startProviderSession(provider, model, workingDir, permissions)

result = sendMessage(session, assembledPrompt)
persistOutput(result)
```

### 6.2 Cross-Provider Handoff

If the current step provider differs from the main session provider:

1. execute the current step in its own provider session according to its step model
2. persist outputs as normal artifacts
3. when the next non-subagent step runs, reopen or continue the workflow run main session for that step's provider
4. inject the prior subagent artifact output into the assembled prompt

Example:

- Step 1 main session: Codex
- Step 2 subagent coding: Gemini
- Step 3 non-subagent review: Codex main session resumes and reads Gemini artifact output

### 6.2 Follow-Up Resolution

Follow-up prompt handling must:

1. target one concrete step
2. resolve that step's `subagent`
3. load the corresponding live session
4. send the follow-up prompt through the same session if available
5. if live session is unavailable:
   - use provider-native resume when supported
   - otherwise fall back to one-shot prompt execution with reconstructed context

If the step belongs to the main session, the follow-up must return to that main-session provider even if earlier subagent steps used another provider.

---

## 7. Runner Process Model

The runner must introduce a process/session registry in memory.

Suggested in-memory structure:

```go
type LiveSession struct {
    SessionID         string
    WorkflowRunID     string
    Provider          string
    Model             string
    TransportType     string
    ProviderSessionID string
    Process           *exec.Cmd
    Status            string
}
```

The registry is used to:

- route later prompts to the same process
- close processes when run completes or is canceled
- avoid duplicate live sessions for the same session key

### 7.1 Concurrency Rule

Only one prompt at a time may be sent into one live session.

Need per-session lock:

- non-subagent main-session steps must serialize on the same session
- subagent-isolated sessions may run independently

---

## 8. Persistence And Recovery

There are two different persistence concerns:

1. provider conversation identity
2. local live process identity

### 8.1 Across Same App Lifetime

When the local runner process stays alive:

- reuse the live provider process directly
- reuse the provider session id directly

### 8.2 After Runner Restart

After local runner restart:

- live local process is gone
- provider session id may or may not be reusable depending on provider
- FlowPilot must still be able to continue the same workflow run even if the old provider session is gone

Recovery rule:

1. reload workflow run state, step state, prompts, and artifacts from FlowPilot persistence
2. try provider-native resume only if it is simple and reliable
3. otherwise start a fresh provider session with a new provider thread/session id
4. inject the persisted workflow context into that new provider session
5. continue the same `workflow_run_id`

Cross-machine example:

- run starts on PC A
- later user resumes work on PC B
- old Codex/Claude/Gemini session is not available on PC B
- FlowPilot starts a brand-new provider session on PC B
- FlowPilot continues the same workflow run using stored prompts, outputs, and artifacts

Design consequence:

- provider session reuse is an optimization, not a hard dependency
- workflow state and artifact state are the durable continuation source

---

## 9. UI/UX Impact

The workflow run UI remains workflow-oriented.

Required visible behavior changes:

- step detail must explain that setting `subagent` makes the step isolated automatically
- run detail page should show session continuity where relevant
- follow-up prompt should continue the step's live session when policy allows

Recommended run detail metadata:

- provider
- model
- session kind (`main` or `subagent`)
- session status

Do not turn the product into a generic chat client. The UI still centers around workflow steps and artifacts.

---

## 10. Verified Provider Test Summary

These results are part of the design baseline, not only exploratory notes.

### 10.1 Codex

Verified locally:

- plain interactive `codex` piped over stdin/stdout failed because stdin is not a terminal
- `codex mcp-server` worked
- one process handled:
  - first request -> `PONG_ONE`
  - second request -> `PONG_TWO`
  - same `threadId`

Design consequence:

- use `codex mcp-server`
- do not build main-session continuity around raw `codex exec`

### 10.2 Claude

Verified locally:

- one `claude` process in stream JSON mode handled:
  - first request -> `CLAUDE_STREAM_ONE`
  - second request -> `CLAUDE_STREAM_TWO`
  - same `session_id`

Design consequence:

- use long-lived `claude -p --input-format stream-json --output-format stream-json --verbose`

### 10.3 Gemini

Verified locally:

- one `gemini --acp` process handled:
  - `session/new`
  - `session/prompt` -> `GEMINI_ACP_ONE`
  - `session/prompt` -> `GEMINI_ACP_TWO`
  - same `sessionId`

Design consequence:

- use ACP as the main session transport
- keep `--resume` only as an optional optimization for recovery scenarios, not as a required continuation path

---

## 11. Compatibility With Existing Engine

This refactor must not break:

- step ordering
- artifact creation
- approval gates
- prompt cache generation
- model routing rules

The session layer sits between:

- prompt assembly
- provider execution

Current flow:

```text
assemble prompt -> executePrompt -> persist result
```

New flow:

```text
assemble prompt -> resolve session -> sendMessage(session) -> persist result
```

---

## 12. Acceptance Criteria

The refactor is technically complete when:

1. the workflow engine can run two or more non-subagent steps without spawning a new provider process per step
2. a follow-up prompt on a non-subagent step continues the same provider session
3. a subagent step starts its own provider session
4. Codex session transport uses `codex mcp-server`
5. Claude session transport uses stream JSON mode
6. Gemini session transport uses ACP mode
7. session metadata is persisted on the workflow run and isolated subagent executions
8. existing artifact and workflow-run history behavior still works
9. workflow run can continue after runner restart or moving to another machine by creating a fresh provider session from persisted workflow context
