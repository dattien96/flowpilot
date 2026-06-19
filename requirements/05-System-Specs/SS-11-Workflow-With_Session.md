# SS-11: Workflow With Session

## 1. Goal

Define how FlowPilot keeps the workflow model as the primary product concept while introducing a second runtime concept: `AI session`.

This spec exists to solve a concrete problem in the current system:

- a workflow run is useful for orchestration
- but the current runner starts a fresh provider command for each step and each follow-up prompt
- this makes follow-up UX weaker and wastes provider startup cost
- it also prevents "continue in the same working context" behavior for non-subagent steps

The new model must let FlowPilot keep workflow steps, approvals, artifacts, and run history while reusing a live provider conversation when appropriate.

---

## 2. Core Mental Model

FlowPilot must separate:

- `workflow run`: business orchestration unit
- `workflow step`: ordered execution unit in the run
- `AI session`: live provider conversation/process used by one or more steps

Important clarification:

- a workflow run is still not a free-form chat thread
- an AI session is still not the product's main user-facing entity
- session is a runtime execution mechanism under the workflow engine
- workflow remains the user-facing orchestration model

The new model is:

`Project -> Workflow Definition -> Workflow Run -> Main Session / Isolated Subagent Session -> Step Execution`

---

## 3. Why Session Exists

Today the system behaves like this:

1. user starts a workflow run
2. runner resolves the next step
3. runner assembles prompt
4. runner starts provider CLI command
5. provider returns output
6. runner saves artifact
7. next step repeats the same cycle with a new provider process (new call to AI provider command in terminal like Codex xxx or gemini -m yyy)

This creates the following limitations:

- every step pays provider startup cost
- every follow-up prompt pays provider startup cost
- continuity is reconstructed only from prompt text and artifacts
- the provider does not keep a true live conversation unless the CLI itself supports resume/thread semantics

The new session model allows:

- several steps to share one live provider conversation
- one step to run in isolation when needed because it has `subagent`
- follow-up prompts (2nd 3rd chat) to continue the same provider session
- workflow orchestration to stay deterministic and auditable

---

## 4. Workflow Session Policy

Session reuse is derived from the existing `subagent` field.

Rules:

- if a step has no `subagent`, it belongs to the shared workflow session
- if a step has a `subagent`, it must run in its own isolated provider session

This means FlowPilot does not need a separate `execution_mode` field.

### 4.1 Step Session Fields

Each step definition only needs:

- `subagent`

Meaning:

- `subagent = null` -> reuse the shared workflow run main session
- `subagent != null` -> start a separate provider session for that step execution

---

## 5. Example Run

Example workflow with 4 steps:

1. Step 1: Product understanding
2. Step 2: Tech analysis
3. Step 3: Review by subagent
4. Step 4: Final update

Recommended configuration:

- Step 1 -> `subagent = null`
- Step 2 -> `subagent = null`
- Step 3 -> `subagent = reviewer-agent`
- Step 4 -> `subagent = null`

Runtime behavior:

1. Step 1 starts the workflow run main provider session
2. Step 2 sends the next prompt into the same main session
3. Step 3 starts a separate provider session because it has `subagent`
4. Step 4 resumes the main session

This means:

- step 1, 2, and 4 share one live conversation in the workflow run main session
- step 3 is isolated from the main session

### 5.1 Cross-Provider Handoff

The main session provider and the subagent session provider do not need to be the same.

Example:

- Step 1 planning uses `gpt-5.x` and therefore runs on Codex in the workflow run main session
- Step 2 architecture also stays on Codex in the same main session
- Step 3 coding has `subagent` and is configured with a Gemini model, so FlowPilot starts an isolated Gemini session for that step
- Step 3 produces artifact output from Gemini
- Step 4 has no `subagent`, so FlowPilot resumes the existing Codex main session and injects the Step 3 artifact result as workflow context

Important behavior:

- provider choice is resolved per step from the step model
- session continuity only applies within the same session boundary
- cross-provider continuity happens through artifacts and workflow context, not by sharing one provider session across different providers

Scope note: this subsection governs the **workflow-step** case only. Interactive desktop chat has a separate, user-initiated cross-provider mechanism defined in section 5.2. Both obey the same hard rule: a live provider session is never migrated across providers.

### 5.2 Interactive Chat Cross-Provider Handoff

A user who is already inside a completed or idle interactive chat may continue the same topic with a different AI provider. This is a distinct, user-initiated operation, not workflow-step progression.

Rules:

- the switch is explicit: the user picks a different provider, FlowPilot shows a confirmation modal, and the user confirms
- confirming never migrates or resumes the source provider session; it always creates a new chat run owned by the target provider with its own new provider session id
- FlowPilot reconstructs the source chat's visible question/answer history and sends it to the target run as one bounded handoff prompt, so context transfer is auditable and visible to the user
- the source run is preserved unchanged and remains reopenable/resumable under its original provider
- only visible user/assistant chat content transfers; hidden system/developer frames, FlowPilot prompt reinforcement, reasoning, tool payloads, credentials, and attachment bytes never transfer
- an empty chat (no turns yet) switches provider directly with no confirmation and no handoff

This preserves the section 5.1 rule (no live session crosses providers) while allowing deliberate, bounded context transfer between two separate runs. The detailed runtime contract is specified in `Task-078: Cross-Provider Chat Handoff`.

---

## 6. Follow-Up Prompt Behavior

Follow-up prompts must target a concrete step and inherit that step's session policy.

Rules:

1. follow-up prompt always belongs to one workflow step
2. if the step has no `subagent`, the follow-up must continue the shared provider session
3. if the step has `subagent`, the follow-up must continue that step's isolated session if still alive, otherwise reopen from saved provider session id when supported
4. if the provider does not support live session continuation, FlowPilot must fall back to provider-native resume/thread or transcript replay depending on adapter capability

The follow-up prompt is still workflow-controlled input, not a free-floating chat thread detached from the run.

If a subagent step used a different provider from the main session, later non-subagent steps must continue the original main session and read the subagent output through artifacts/context injection.

---

## 7. Provider Behavior Behind The Scenes

This section defines the expected runtime behavior from FlowPilot runner to provider.

### 7.1 Codex

FlowPilot must not depend on piping prompts directly into the interactive `codex` TUI because the interactive CLI requires a terminal and rejects plain piped stdin.

Verified behavior:

- `codex` interactive process over plain stdin/stdout does not work for runner-style reuse
- `codex mcp-server` works as a long-lived machine protocol

Expected runtime flow:

1. runner starts one `codex mcp-server` process for the provider session host
2. runner sends MCP `codex` tool call for the first prompt
3. Codex returns `threadId`
4. runner stores that `threadId` as `provider_session_id`
5. runner sends `codex-reply` for later prompts using the same `threadId`

Session ownership:

- one workflow run main session can map to one Codex `threadId`

### 7.2 Claude Code

FlowPilot should use Claude Code stream JSON mode rather than a local MCP adapter.

Verified behavior:

- one `claude` process can accept multiple JSONL user messages over stdin
- the same process returns the same `session_id`

Expected runtime flow:

1. runner starts one long-lived process:
   `claude -p --verbose --input-format stream-json --output-format stream-json ...`
2. runner writes one JSONL user message for the first step prompt
3. Claude returns assistant events plus a final `result` event with `session_id`
4. runner stores the `session_id` as `provider_session_id`
5. runner writes the next JSONL user message into the same process for later main-session steps

Session ownership:

- one workflow run main session can map to one live Claude process and one `session_id`

### 7.3 Gemini

FlowPilot should use Gemini ACP mode for true long-lived session reuse.

Verified behavior:

- `gemini --acp` works as a long-lived JSON-RPC process
- one session can accept multiple `session/prompt` calls and return responses in the same `sessionId`

Expected runtime flow:

1. runner starts one process:
   `gemini --acp`
2. runner sends `initialize`
3. runner sends `session/new`
4. Gemini returns `sessionId`
5. runner stores that `sessionId` as `provider_session_id`
6. runner sends `session/prompt` for the first step prompt
7. runner sends `session/prompt` again for later main-session steps

Session ownership:

- one workflow run main session can map to one Gemini `sessionId`

---

## 8. Verified Local Test Results

The following behavior has been verified locally on the current machine.

### 8.1 Codex Result

Verified:

- raw interactive `codex` over plain stdin/stdout failed with `stdin is not a terminal`
- `codex mcp-server` succeeded
- first prompt returned `PONG_ONE`
- second prompt returned `PONG_TWO`
- both used the same Codex `threadId`

### 8.2 Claude Code Result

Verified:

- one Claude process accepted two stream JSON user messages
- first response returned `CLAUDE_STREAM_ONE`
- second response returned `CLAUDE_STREAM_TWO`
- both used the same `session_id`

### 8.3 Gemini Result

Verified:

- Gemini resume across separate commands works with `--resume`
- Gemini ACP mode also works as one long-lived process
- first ACP prompt returned `GEMINI_ACP_ONE`
- second ACP prompt returned `GEMINI_ACP_TWO`
- both used the same `sessionId`

---

## 9. Session Persistence

FlowPilot must persist session state at runtime.

Each workflow run may own:

- one shared main session
- zero or more isolated subagent sessions

Each session record must store at least:

- `workflow_run_id`
- `provider`
- `model`
- `provider_session_id`
- `transport_type`
- `status`
- `started_at`
- `completed_at`

`transport_type` examples:

- `codex_mcp`
- `claude_stream_json`
- `gemini_acp`
- `provider_resume_only`
- `one_shot_fallback`

Important rule:

- `workflow_run_id` is the durable virtual identity owned by FlowPilot
- `provider_session_id` is only a runtime optimization
- if the runner restarts, the provider session disappears, or execution moves from PC A to PC B, FlowPilot may start a brand-new provider session with a new provider thread/session id and continue the same workflow run from persisted workflow state, prompts, and artifacts

---

## 10. Safety And Determinism

Shared sessions improve continuity but introduce extra risk:

- hidden context can leak from one step to another
- later steps may depend on live conversation state not fully visible in the final artifact
- retries and reproduction become harder if session state is not captured clearly

To keep the workflow product reliable:

1. use the shared session for ordinary non-subagent workflow progression
2. use `subagent` only for steps that should be isolated from the main session
3. persist provider session identifiers when available, but do not treat them as the durable source of truth
4. continue storing prompts, outputs, artifacts, and run logs as the audit trail

Workflow must remain the official source of truth, not the opaque provider transcript alone.

---

## 11. Product Rules

Final rules for FlowPilot:

1. Workflow remains the primary product concept.
2. Session is a runtime execution concept under the workflow engine.
3. Session reuse is controlled by step `subagent` presence.
4. FlowPilot must support provider-specific session adapters.
5. If a provider supports long-lived sessions, FlowPilot should prefer them over one-shot command spawning.
6. If a provider session is lost, FlowPilot may create a new provider session and continue the same workflow run from persisted workflow context.
7. Provider session ids are not the durable workflow identity.
8. Changing provider inside an existing interactive chat creates a new chat run via user-confirmed transcript context transfer (section 5.2). It never migrates the source provider session across providers, and the source run is preserved.
