# CP-18: Refactor Workflow With Session

## 1. Goal

Implement the session-aware workflow runtime defined by:

- `SS-11-Workflow-With_Session`
- `SD-12-Refactor-Workflow-With_Session`

This plan turns the verified provider experiments into production code.

The key product outcome is:

- keep workflow as the main concept
- stop spawning a fresh provider process for every non-subagent step and follow-up prompt

---

## 2. Scope

This refactor includes:

- step-level session policy
- workflow-run session persistence
- runner live session registry
- Codex session adapter
- Claude session adapter
- Gemini session adapter
- follow-up reuse of existing session
- run detail metadata for session-aware execution
- cross-provider handoff between main session and subagent session

This refactor does not require:

- replacing the workflow engine with a chat UI
- removing artifacts or approvals
- removing the existing one-shot execution path

The existing one-shot execution path remains as fallback.

---

## 3. Verified Baseline

These provider behaviors were verified locally and should be treated as implementation baseline:

### 3.1 Codex

- raw interactive `codex` over stdin/stdout is not suitable
- `codex mcp-server` works as a long-lived process
- same `threadId` can handle multiple prompts

### 3.2 Claude

- one Claude process in stream JSON mode accepted two prompts
- both prompts used the same `session_id`

### 3.3 Gemini

- Gemini ACP mode accepted `session/new` and multiple `session/prompt` calls
- both prompts used the same `sessionId`

These are not assumptions anymore. They are verified integration targets.

---

## 4. Implementation Plan

### Phase 1: Data Contract

1. Reuse existing workflow step `subagent` as the V1 session policy input.
2. Add workflow run session persistence table or equivalent persistence model.
3. Update frontend and backend types.

Expected fields:

- existing `workflow_steps.subagent`
- `workflow_run_sessions.*`

Acceptance:

- session policy can be resolved from step `subagent`
- workflow run session records can be stored independently of outputs/artifacts

### Phase 2: Runner Session Abstraction

1. Introduce `startSession`
2. Introduce `sendMessage`
3. Introduce `closeSession`
4. Keep `executePrompt` as fallback

Acceptance:

- workflow runtime can choose session path or fallback path at execution time
- runner has in-memory live session registry keyed by workflow session identity

### Phase 3: Codex Adapter

1. Build `codex mcp-server` transport
2. Implement MCP initialize
3. Implement first prompt with tool `codex`
4. Implement continuation prompt with tool `codex-reply`
5. Persist returned `threadId`

Acceptance:

- two main-session Codex prompts in one run reuse the same `threadId`
- no new Codex process is spawned for the second prompt in the same live session

### Phase 4: Claude Adapter

1. Build long-lived Claude stream JSON transport
2. Serialize JSONL user messages to stdin
3. Parse assistant/result events from stdout
4. Persist returned `session_id`

Acceptance:

- two main-session Claude prompts in one run reuse the same live process
- same `session_id` is observed for both prompts

### Phase 5: Gemini Adapter

1. Build Gemini ACP transport
2. Implement `initialize`
3. Implement `session/new`
4. Implement `session/prompt`
5. Persist returned `sessionId`

Acceptance:

- two main-session Gemini prompts in one run reuse the same ACP process
- same `sessionId` is observed for both prompts

### Phase 6: Workflow Runtime Integration

1. Resolve session policy before executing each step
2. Load or create the correct workflow session
3. Route prompt through session adapter
4. Persist outputs without changing artifact semantics

Acceptance:

- non-subagent steps reuse the workflow run main session
- steps with `subagent` start separate sessions
- artifact output still saves correctly

### Phase 7: Follow-Up Runtime

1. Update follow-up path to reuse step session policy
2. Continue same provider session for non-subagent steps
3. Continue same step-owned session for subagent steps when live
4. fall back safely when session is unavailable

Acceptance:

- follow-up prompt on a non-subagent step does not start a new provider process while the live session exists
- refreshed run history still shows correct prompt/output chronology

### Phase 8: UI Exposure

1. Make the UI explain that `subagent` means isolated session
2. Show session metadata in run detail

Acceptance:

- user can understand that non-subagent steps share the main session and subagent steps isolate automatically
- run detail clearly shows which steps share a session

---

## 5. Run Flow Behind The Scene

This is the intended runtime flow after refactor.

### 5.1 Main Session Step

```text
workflow-start-runtime
  -> resolve workflow step
  -> resolve subagent
  -> resolve provider + model
  -> assemble prompt
  -> find or create workflow_run_session
  -> adapter.sendMessage(...)
  -> provider returns response
  -> save output artifact
  -> update workflow_run_step
```

### 5.2 Codex Shared Session

```text
runner starts codex mcp-server
  -> first step calls MCP tool "codex"
  -> result contains threadId
  -> runner stores provider_session_id = threadId
  -> next shared step calls MCP tool "codex-reply"
  -> same threadId continues
```

### 5.3 Claude Shared Session

```text
runner starts one claude stream-json process
  -> first step writes JSONL user message
  -> result contains session_id
  -> runner stores provider_session_id = session_id
  -> next shared step writes another JSONL user message
  -> same process and same session_id continue
```

### 5.4 Gemini Shared Session

```text
runner starts one gemini --acp process
  -> initialize
  -> session/new returns sessionId
  -> first step sends session/prompt
  -> runner stores provider_session_id = sessionId
  -> next shared step sends session/prompt again
  -> same sessionId continues
```

### 5.5 Isolated Step

```text
resolve step has subagent
  -> create dedicated workflow_run_session for this step
  -> send prompt through isolated adapter session
  -> close or park session after step based on provider policy
```

### 5.6 Cross-Provider Handoff Example

```text
Step 1
  model = gpt-5.x
  subagent = null
  -> runner uses Codex main session

Step 2
  model = gemini-*
  subagent = coding-agent
  -> runner starts isolated Gemini session
  -> Gemini produces coding artifact

Step 3
  model = gpt-5.x
  subagent = null
  -> runner resumes Codex main session
  -> prompt builder injects Step 2 Gemini artifact output
```

Rule:

- main flow provider can differ from subagent provider
- session reuse never crosses providers directly
- artifact output is the handoff mechanism between provider sessions

---

## 6. Fallback Rules

If a provider session cannot be continued:

1. provider-native resume or thread continuation is optional optimization only
2. otherwise start a fresh provider process and a new provider session/thread id
3. reconstruct context from:
   - current step prompt
   - prior artifacts
   - follow-up prompt text
   - workflow state

Fallback must be explicit in logs and session metadata.

Important rule:

- the workflow run is the durable virtual identity
- provider session ids may change across restarts, crashes, or moving execution from PC A to PC B

---

## 7. Risks

### 7.1 Hidden Context Leakage

Shared sessions can leak reasoning or intermediate assumptions across steps.

Mitigation:

- keep subagent-isolated execution available
- keep the shared path as one implicit main session for V1
- default new steps conservatively

### 7.2 Process Lifecycle Complexity

Long-lived processes require:

- health checks
- cleanup
- timeouts
- cancellation
- per-session locks

Mitigation:

- central live session registry
- adapter-specific close behavior
- explicit runner shutdown cleanup

### 7.3 Recovery After Restart

The local process can die while provider session identity still exists.

Mitigation:

- persist `provider_session_id` when available
- define provider-specific resume only as best-effort optimization
- always support creating a fresh provider session for the same workflow run
- treat workflow state and artifacts as the durable continuation source

---

## 8. Test Plan

### 8.1 Unit Tests

- session policy resolution
- workflow session key generation
- adapter response parsing
- session persistence mapping
- fallback selection logic

### 8.2 Integration Tests

- Codex adapter reuses same `threadId`
- Claude adapter reuses same `session_id`
- Gemini adapter reuses same `sessionId`
- follow-up prompt reuses existing shared session
- subagent step does not reuse shared session
- Codex main session -> Gemini subagent step -> Codex main session resume works through artifact handoff
- workflow run can continue after runner restart by creating a fresh provider session from persisted workflow context

### 8.3 UI Tests

- step editor/save path preserves `subagent`
- run detail shows session metadata
- follow-up on shared session does not regress timeline rendering

---

## 9. Suggested Delivery Order

1. Data contract and schema
2. Runner session abstraction
3. Gemini ACP adapter
4. Codex MCP adapter
5. Claude stream JSON adapter
6. Workflow runtime integration
7. Follow-up runtime integration
8. UI exposure and diagnostics

Reasoning:

- Gemini ACP and Codex MCP are the cleanest structured machine protocols
- Claude stream JSON is also verified, but its parsing contract is more CLI-specific
- session abstraction should land before provider adapters

---

## 10. Definition Of Done Checklist

This feature is only marked done when every checklist item below is complete.

### 10.1 Data And Persistence

- [ ] `workflow_run_sessions` persistence exists and stores `workflow_run_id`, provider, model, transport type, provider session id, status, timestamps, and adapter metadata
- [ ] workflow runtime can distinguish one shared main session from isolated subagent sessions
- [ ] existing step `subagent` data remains intact through create, edit, load, and run flows
- [ ] no new `execution_mode` or `session_group` field is introduced for V1

### 10.2 Runner Session Abstraction

- [ ] runner exposes a session-aware abstraction such as `startSession`, `sendMessage`, and `closeSession`
- [ ] old `executePrompt` remains available as fallback for unsupported paths
- [ ] runner maintains an in-memory live session registry keyed by workflow run main session or isolated step session
- [ ] one live session cannot process two prompts concurrently without locking
- [ ] runner shutdown or cancellation cleans up live provider processes correctly

### 10.3 Provider Adapters

- [ ] Codex integration uses `codex mcp-server`
- [ ] Codex adapter stores and reuses `threadId`
- [ ] Claude integration uses long-lived stream JSON mode
- [ ] Claude adapter stores and reuses `session_id`
- [ ] Gemini integration uses ACP mode
- [ ] Gemini adapter stores and reuses `sessionId`
- [ ] provider session ids are treated as runtime optimization only, not durable workflow identity

### 10.4 Workflow Runtime Behavior

- [ ] non-subagent steps reuse the workflow run main session
- [ ] steps with `subagent` start isolated sessions
- [ ] provider/model resolution still happens per step
- [ ] cross-provider handoff works through artifact persistence and prompt/context assembly
- [ ] a main flow can use Codex while a subagent step uses Gemini or Claude
- [ ] after a subagent step finishes, the next non-subagent step resumes the original main session
- [ ] existing artifact save and reload behavior still works
- [ ] approval-gate behavior still works
- [ ] prompt-cache behavior still works

### 10.5 Follow-Up Behavior

- [ ] follow-up on a non-subagent step continues the main session while it is alive
- [ ] follow-up on a subagent step continues that isolated session while it is alive
- [ ] if the old provider session is gone, FlowPilot starts a fresh provider session and reconstructs context from workflow state and artifacts
- [ ] follow-up behavior still works after runner restart
- [ ] follow-up behavior still works when execution moves from PC A to PC B

### 10.6 UI And Diagnostics

- [ ] step detail UI explains that `subagent` means isolated session
- [ ] run detail UI shows provider, model, session kind, and session status
- [ ] run detail UI keeps correct prompt/output chronology after follow-up
- [ ] logs make it clear whether FlowPilot reused a provider session or created a fresh one
- [ ] logs make it clear when provider recovery used new session creation instead of provider-native resume

### 10.7 Tests

- [ ] unit tests cover session resolution for non-subagent and subagent steps
- [ ] unit tests cover fallback behavior when provider session id is missing or unusable
- [ ] integration tests confirm Codex reuses one `threadId`
- [ ] integration tests confirm Claude reuses one `session_id`
- [ ] integration tests confirm Gemini reuses one `sessionId`
- [ ] integration tests confirm cross-provider handoff from main session to subagent and back
- [ ] integration tests confirm workflow continuation after runner restart by creating a fresh provider session
- [ ] integration tests confirm workflow continuation on another machine by creating a fresh provider session from persisted workflow context
- [ ] UI tests cover follow-up timeline behavior and session metadata display

### 10.8 Product-Level Signoff

- [ ] workflow remains the main user-facing orchestration concept
- [ ] provider session loss no longer blocks workflow continuation
- [ ] the feature is documented by `SS-11`, `SD-12`, and this `CP-18`
- [ ] all items above are verified and signed off before the feature is marked done
