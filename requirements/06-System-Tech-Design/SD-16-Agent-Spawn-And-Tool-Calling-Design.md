# SD-16: Agent Spawn And Tool-Calling Design

## Metadata

we fix and improve a lot for agent feature with claude: from the commit 541748ac952634147fc3c0c788a16b638b1ad631 -> till now. scan all those changes and make sure codex can work same with what claude supported

- Document ID: `SD-16`
- Title: `Agent Spawn And Tool-Calling Design`
- Phase: `tech_design`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-20`
- Last Updated: `2026-06-23`
- Parent Documents: [SS-06: Workflow Skill Agent](../05-System-Specs/SS-06-Workflow-Skill-Agent.md), [SS-11: Workflow With Session](../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: [CP-19: Multiple Agents](../07-Coding-Plan/done/CP-19-Multiple-Agents.md)
- Related Documents: [SD-13: Multiple Agents](./SD-13-Multiple-Agents.md), [Task-081: Agent Abstraction And Catalog Loader](../08-Task/done/Task-081-Agent-Abstraction-And-Catalog-Loader.md), [Task-082: Spawn-Agent Tool And Orchestrator Core](../08-Task/done/Task-082-Spawn-Agent-Tool-And-Orchestrator-Core.md), [Task-083: Desktop Agents Panel And Focus Navigation](../08-Task/todo/Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md), [Task-084: Dependency Feedback Loop And Orchestration Board](../08-Task/done/Task-084-Dependency-Feedback-Loop-And-Orchestration-Board.md), [Task-085: Unified Local Run Persistence](../08-Task/todo/Task-085-Unified-Local-Run-Persistence.md)
- Replaces: `None`
- Tags: `multi-agent, spawn-agent, tool-calling, agent-catalog, local-runner, desktop`

## AI Quick View

### Summary

- FlowPilot agent spawning has two entry points: an AI-callable `spawn_agent` tool and a direct desktop/API spawn action.
- `spawn_agent` is only the provider-visible tool name; actual child creation is runner-owned and happens through `spawnChildRun`.
- A child agent is a normal provider chat/run/thread tagged with agent metadata (`parentRunId`, `agentName`, `role`, dependencies, status).
- Agent markdown files in `.claude/agents` and `.codex/agents` define persona/config for FlowPilot's catalog; they do not make the provider app spawn children by itself.
- `wait=true` keeps the tool call open until the child resolves, but the desktop must surface child waiting/approval state so the product never appears hung.

### Current Ask

- Explain the runtime design clearly enough that implementers and users can distinguish AI tool-calling, direct UI spawning, runner-owned child creation, agent markdown loading, and wait/result behavior.
- Add a QA section that answers the user questions raised during BUG-Note-Agent-Feature root-cause analysis.

### Key Decisions

- `D-1` `spawn_agent` is a FlowPilot-defined tool contract exposed to AI providers; it is not the implementation that creates the child run.
- `D-2` `spawnChildRun` is the single backend creation path for both AI-driven and UI-driven spawns.
- `D-3` FlowPilot owns child-run creation, parent/child linking, persistence, SSE events, approval/question gates, and final result routing.
- `D-4` Agent markdown files are accepted as catalog definitions using the standard frontmatter/body shape, but FlowPilot parses and applies them instead of delegating orchestration to native provider agent frameworks.
- `D-5` `wait=true` is protocol-level blocking of the tool result, not permission to freeze the UI; waiting child gates must be visible and actionable.

### Constraints

- Preserve existing provider adapter boundaries: providers may call tools, but the local runner owns side effects.
- Keep child agents as additive metadata on normal runs so existing chat/session/approval infrastructure is reused.
- Do not assume Claude, Codex, or other apps use the same tool name; `spawn_agent` is FlowPilot's contract name.
- Do not treat markdown `tools` frontmatter as an enforced allowlist unless a later task explicitly implements enforcement.

### Open Questions

- `Q-1` Should `wait=true` return a "child waiting for approval/question" interim result to the main AI, or should it remain blocked while the UI handles the gate?
- `Q-2` Should FlowPilot enforce agent markdown `tools` as per-agent provider permissions, or keep them as catalog/display intent only?
- `Q-3` Should direct UI spawn always default to `wait=false`, since there is no main AI tool call waiting for a result?

### Source Refs

- `SS-06` agent runtime intent; `SS-11` session model.
- `CP-19` P-1, P-2, P-3, P-7, P-9.
- `Task-081` agent catalog loader; `Task-082` spawn tool/orchestrator core; `Task-083` Agents panel; `Task-084` graph/bus; `Task-085` durable agent runs.
- Code refs: `agent_catalog.go`, `agent_orchestrator.go`, `interactive_service.go`, `codex_adapter.go`, `claude_permission_mcp.go`, `AgentsPanel.tsx`, `HttpWsRunnerClient.ts`.

## 1. Goal

Define how FlowPilot's agent feature works at the tool-calling and runtime boundary:

- what the AI sees (`spawn_agent` tool)
- what FlowPilot runs (`spawnChildRun`)
- how direct UI spawn differs from prompt-driven spawn
- how child agents map to provider sessions/threads
- how agent markdown files are loaded and applied
- how waiting, approvals, and child results flow back to the main run

This document is intentionally explanatory because the feature crosses three mental models: AI tool-calling, local runner orchestration, and desktop UI state.

## 2. Input Documents

- [SS-06: Workflow Skill Agent](../05-System-Specs/SS-06-Workflow-Skill-Agent.md)
- [SS-11: Workflow With Session](../05-System-Specs/SS-11-Workflow-With_Session.md)
- [CP-19: Multiple Agents](../07-Coding-Plan/done/CP-19-Multiple-Agents.md)
- [Task-081: Agent Abstraction And Catalog Loader](../08-Task/done/Task-081-Agent-Abstraction-And-Catalog-Loader.md)
- [Task-082: Spawn-Agent Tool And Orchestrator Core](../08-Task/done/Task-082-Spawn-Agent-Tool-And-Orchestrator-Core.md)
- [Task-083: Desktop Agents Panel And Focus Navigation](../08-Task/todo/Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md)
- [Task-084: Dependency Feedback Loop And Orchestration Board](../08-Task/done/Task-084-Dependency-Feedback-Loop-And-Orchestration-Board.md)
- [Task-085: Unified Local Run Persistence](../08-Task/todo/Task-085-Unified-Local-Run-Persistence.md)

## 3. Architecture Decision

- `D-1` **AI-facing tool name:** FlowPilot exposes a tool named `spawn_agent` to supported providers. The provider model can decide to call it when the user asks for a sub-agent.
- `D-2` **Runner-owned implementation:** The local runner catches the provider tool call, validates the arguments, and invokes `spawnChildRun`. The model never creates runs directly.
- `D-3` **Shared spawn path:** The desktop `+ Spawn agent` action calls the same backend spawn path through HTTP. This bypasses "AI decides to call a tool" but not runner validation, child metadata, approvals, or persistence.
- `D-4` **Child as normal run:** A child agent is represented as a normal provider-backed run/thread with additional agent metadata. This avoids a separate agent transport and reuses SSE, approval gates, session persistence, transcript replay, and provider account handling.
- `D-5` **Markdown definitions are catalog input:** `.claude/agents/*.md` and `.codex/agents/*.md` files define the agent identity, preferred provider/model, tools metadata, and system prompt. FlowPilot reads these files and applies the definition while starting the child run.
- `D-13` **Codex tool-name alias:** `spawn_agent` collides with a reserved name in the Codex app-server, so on Codex the tool is registered as `flowpilot_spawn_agent` and normalized back to `spawn_agent` at the runner/UI boundary; the model sees one consistent contract while Codex gets a non-reserved name. Legacy Codex rollouts that recorded the reserved name are migrated on resume. (BUG-124, BUG-127)

Alternatives considered:

- Let the provider's native agent framework own child spawning. Rejected because FlowPilot would lose control of parent/child persistence, UI state, approval gates, and cross-provider behavior.
- Make UI spawn a separate implementation from AI tool spawn. Rejected because it would create divergent behavior and duplicate lifecycle code.

Why this option was chosen:

- It preserves FlowPilot as the source of truth for orchestration and safety while still letting the AI request delegation through standard tool-calling.

## 4. Component Impact

- Impacted modules:
  - local-runner agent catalog and orchestrator
  - local-runner provider adapters for tool registration and tool-call handling
  - local-runner interactive service for child run creation, waiting, events, approvals, and persistence
  - desktop Agents panel, chat input, timeline, orchestration board, and runner client contract
- New modules:
  - `AgentCatalog`
  - `AgentOrchestrator`
  - desktop Agents panel and orchestration board
- Unchanged modules:
  - provider model internals
  - provider-native official desktop app agent naming
  - base SSE stream contract except additive agent graph/bus event variants

## 5. Data Model

- Entities:
  - `AgentDefinition`: loaded definition with `name`, `description`, `role`, `provider`, `model`, `tools`, `systemPrompt`, `source`, and `path`.
  - `interactiveRun`: existing run object extended with child identity fields.
  - `AgentRunSummary`: compact child run view for the desktop panel and graph.
  - `AgentGraphSnapshot`: parent-level graph state containing runs, edges, bus messages, and loop state.
- Fields:
  - `parentRunId`: links child run to parent run.
  - `agentName`: catalog name such as `coder`, `reviewer`, or `tester`.
  - `role`: normalized role used for UI and orchestration decisions.
  - `dependsOn`: child run IDs or agent dependencies.
  - `agentStatus`: child-specific status detail such as `spawned`, `waiting_dependency`, or a run status mirror.
- State transitions:
  - parent active -> child spawned -> child running -> child waiting for approval/question -> child completed/failed/cancelled
  - parent graph updates must mirror child status transitions so the UI does not appear hung while `wait=true` is blocked.

## 6. Interfaces and Contracts

- API contracts:
  - AI tool contract: `spawn_agent({ agent, prompt, provider?, dependsOn?, wait })`.
  - Desktop API contract: `POST /client/workflow-runs/{parentRunId}/spawn-agent`.
  - Listing contract: `GET /client/workflow-runs/{parentRunId}/agents`.
  - Graph contract: parent run SSE may emit additive `agent_graph_updated` and `agent_bus_message` events.
- DB contracts:
  - Phase 1 local mode persists agent metadata in existing local session records/manifests.
  - Phase 2 flow mode adds durable `agent_runs` and `agent_messages`.
- File or artifact contracts:
  - Project-local agent definitions are read from `.claude/agents/*.md` and `.codex/agents/*.md`.
  - Provider-home definitions may also be read from provider account homes.
  - FlowPilot built-ins provide fallback definitions when no files exist.
- Provider or MCP contracts:
  - The provider sees `spawn_agent` as a dynamic tool/MCP-style tool depending on adapter.
  - The provider returns a tool call request; FlowPilot executes the side effect and returns a tool result.

## 7. Execution Flow

### 7.1 Prompt-Driven Spawn

1. User asks the main AI to use a sub-agent.
2. FlowPilot has registered `spawn_agent` as an available provider tool for the main run.
3. The main AI chooses whether to call `spawn_agent`.
4. The provider sends the tool call to the local runner.
5. The runner parses arguments into `SpawnAgentInput`.
6. The runner calls `spawnChildRun`.
7. `spawnChildRun` creates a child run, links it to `parentRunId`, applies the agent definition, emits graph/sidebar state, and starts the child turn.
8. If `wait=false`, the runner returns child run metadata immediately.
9. If `wait=true`, the runner keeps the tool call open until the child resolves or the chosen wait policy returns an interim waiting result.
10. The main AI receives the tool result and can summarize it to the user.

### 7.2 Direct UI Spawn

1. User opens the Agents panel and clicks `+ Spawn agent`.
2. Desktop calls the runner `spawnAgent` client method.
3. The runner HTTP handler calls `spawnChildRun` directly.
4. The child run starts without requiring the main AI to decide or approve the spawn.
5. The child still obeys normal provider/tool approval rules for its own actions.

### 7.3 Child Agent Runtime

1. The child gets its own provider session/thread.
2. The child prompt is composed from the agent system prompt plus the user's child task prompt. Composition is **provider-independent** (BUG-128): a single helper (`composeAgentSpawnPrompt`) builds the same prompt shape for every provider — the agent system prompt first (kept first so built-in-agent prompt detection in run history keeps matching), then one identity line naming the agent/role and linking its definition file (`[FlowPilot sub-agent — agent: … | role: … | definition: <path or built-in>]`), then the user's child task prompt. The same agent name therefore yields an identical prompt shape on Claude and Codex; any content difference comes only from the resolved definition (catalog precedence per `D-5`), not from the composition path.
3. The child streams events over its own run stream.
4. Parent-level graph/bus events summarize child lifecycle for the main UI.
5. The child approval/question gates are owned by the child stream, not by the parent message body.

## 8. Failure and Edge Handling

- `F-1` If the provider does not call `spawn_agent`, no child run is created. The UI should make this visible as "tool was not called" rather than implying a hidden child exists.
- `F-2` If `spawn_agent` is not registered on a resumed provider thread, the AI may hallucinate delegation. The runner must register the tool consistently for start and resume paths.
- `F-3` If `wait=true` child enters approval/question state, the product must surface the child as waiting in the Agents panel, timeline banner, and graph. The main input should not look stuck.
- `F-4` If the user spawns through the UI, failure returns an API error and no AI summary is expected.
- `F-5` If an agent markdown file has unknown frontmatter keys, FlowPilot ignores them rather than failing catalog load.
- `F-6` If an agent file omits `name`, FlowPilot can derive a name from the file name.
- `F-7` If an agent definition declares a preferred provider/model that is unavailable, the spawn path must return a clear provider/model error or fall back only when the design explicitly allows fallback.

## 9. Security and Operational Concerns

- auth: Direct UI spawn is a local runner action under the current project/session authority; it does not require the main AI to approve the spawn.
- secrets: Agent definitions must not embed secrets; provider account credentials remain managed by provider account configuration.
- audit: Spawn events, child run IDs, parent links, tool results, approval decisions, and graph/bus messages must be persisted or replayable according to the current phase.
- rollback: Hiding the spawn tool and Agents panel reverts the app to single-agent behavior because child identity is additive.
- permissions: Agent markdown `tools` currently describes intended tools/catalog metadata. It must not be assumed to enforce provider permissions unless a later implementation wires it into provider-level tool allowlists.

## 10. Risks and Trade-Offs

- `R-1` Users may think `spawn_agent` itself creates agents. Mitigation: document that `spawn_agent` is the AI-visible request, while `spawnChildRun` is the runner implementation.
- `R-2` `wait=true` can feel like a hang if child waiting state is not mirrored to parent UI. Mitigation: emit parent graph updates for child approval/question/completion transitions.
- `R-3` Provider-native agent markdown semantics may drift from FlowPilot's parser. Mitigation: support a conservative compatible subset and document FlowPilot-specific behavior.
- `R-4` Direct UI spawn can surprise users if they expect the main AI to decide. Mitigation: label UI spawn as a direct runner action and keep child approval gates intact.
- `R-5` Tool registration drift between new and resumed provider threads can cause hallucinated delegation. Mitigation: register `spawn_agent` consistently for every provider turn path that supports dynamic tools.

## 11. Validation Strategy

- unit:
  - agent markdown parser handles frontmatter, body system prompt, fallback name, inline tools, and block-list tools
  - `spawn_agent` argument parsing rejects missing agent/prompt
  - `spawnChildRun` creates child metadata and parent graph entry
  - `wait=true` and `wait=false` result semantics are covered
- integration:
  - Codex and Claude tool-call paths both invoke `spawnChildRun`
  - Codex resumed turns still expose `spawn_agent`
  - child approval/question state updates parent graph/sidebar state
  - UI spawn and AI spawn produce equivalent child run metadata
- manual:
  - ask the main AI to call `spawn_agent` and verify a child appears
  - use the Agents panel `+ Spawn agent` button and verify no AI approval is required for spawning
  - with YOLO off, make the child request a gated write and verify the UI shows child waiting approval, not a frozen main turn
- observability:
  - log parent run ID, child run ID, agent name, provider, wait mode, and final status for each spawn

## 12. Traceability to Spec

- `SS-06 Agent Runtime` -> `D-4`, `D-5`, Sections 5 and 7.3.
- `SS-11 session continuity` -> `D-4`, Sections 5, 6, and 8.
- `CP-19 P-1` child run as extended `interactiveRun` -> Sections 5 and 7.3.
- `CP-19 P-2` two spawn entry points -> Sections 3, 6, 7.1, and 7.2.
- `CP-19 P-2` result parity across entry points -> `D-6`, `D-7`, `D-8`, Section 14 (BUG-121, BUG-122, BUG-126).
- `CP-19 P-3` agent definitions from `.claude/agents` and `.codex/agents` -> Sections 5, 6, and 13.
- `CP-19 P-9` child gates remain owned by child stream -> Sections 7.3, 8, and 9.
- `CP-19 P-9` YOLO inheritance + main-run gating -> `D-9`, `D-10`, Section 15 (BUG-129, BUG-131, BUG-133).
- `CP-19 P-7` desktop agent-list rendering + slash commands -> `D-11`, `D-12`, Section 15 (BUG-130, BUG-132, BUG-134, Task-087).
- `CP-19 P-2` Codex spawn-tool name parity -> `D-13` (BUG-124, BUG-127).
- `CP-19 P-7` child model resolution -> `D-14`, Section 15.5.
- `CP-19 P-1` child lifecycle delete cascade -> `D-15`, Section 15.6 (Task-086).

## 13. QA: Agent Spawn Mental Model

- `Q-A1` **Is `spawn_agent` just a tool name?**
  - Yes. `spawn_agent` is the tool name FlowPilot exposes to the provider model. The AI can call it, and the our runner golang catches that call.

- `Q-A2` **Can another app use a different name for the same concept?**
  - Yes. Another app could call a similar tool `Task`, `delegate`, `subagent`, or any other name. The name is app-specific; FlowPilot's contract name is `spawn_agent`.

- `Q-A3` **Does calling `spawn_agent` directly create the agent?**
  - No. The provider sends a tool-call request. FlowPilot validates it and calls the backend implementation, `spawnChildRun`.

- `Q-A4` **What is `spawnChildRun`?**
  - `spawnChildRun` is FlowPilot's internal runner function. It creates the child run, links it to the parent, applies the agent definition, emits graph/UI state, starts the child turn, and optionally waits for a result.

- `Q-A5` **Can FlowPilot create an agent without asking the main AI?**
  - Yes. The desktop Agents panel calls the runner directly. This uses the same `spawnChildRun` path but skips the step where the main AI decides to call `spawn_agent`.

- `Q-A6` **Does direct UI spawn bypass approvals?**
  - It bypasses main-AI decision making only. The child agent's own file writes, commands, MCP writes, questions, and provider actions still obey FlowPilot approval and YOLO rules.

- `Q-A7` **Is a child agent basically a new chat thread?**
  - Yes, technically it is a new provider-backed run/session/thread plus FlowPilot metadata: parent run ID, agent name, role, status, dependencies, events, and persistence.

- `Q-A8` **Who makes the main AI wait for the child result?**
  - FlowPilot does. With `wait=true`, the runner keeps the `spawn_agent` tool result open until the child completes or until the configured wait policy returns an interim state. The main AI simply waits for the tool result.

- `Q-A9` **Who summarizes the child result to the user?**
  - After FlowPilot returns the tool result, the main AI reads that result and writes the final user-facing response.

- `Q-A10` **Why does the AI need to notify FlowPilot through a tool call?**
  - The AI cannot mutate FlowPilot app state by itself. Tool-calling is the controlled boundary where FlowPilot can validate, create the child run, log it, wire UI state, enforce approvals, and persist the result.

- `Q-A11` **Do we need to follow `.codex/agents` or `.claude/agents` markdown file rules?**
  - Yes for catalog loading. FlowPilot reads markdown agent files with frontmatter and body prompt, so files should use the standard shape: `name`, `description`, optional `provider`, optional `model`, optional `role`, optional `tools`, then the system prompt body.

- `Q-A12` **Does FlowPilot delegate agent execution to the official Claude/Codex desktop agent system?**
  - No. FlowPilot uses those markdown files as definitions, then starts its own child run through the local runner. Provider models execute the child turn, but FlowPilot owns orchestration.

- `Q-A13` **If an agent file defines `model`, does FlowPilot use it?**
  - Yes, the child spawn path can apply the agent definition's preferred model when starting the child run.

- `Q-A14` **If an agent file defines `tools`, are those tools enforced today?**
  - Not as a strict permission allowlist by default. The current design treats `tools` as catalog/intent metadata unless a later task explicitly wires it into provider-level tool restrictions.

- `Q-A15` **Why can `wait=true` look like hanging?**
  - Because `wait=true` keeps the parent tool call open while the child is running. If the child enters approval/question state and the parent graph/sidebar is not updated, the user sees a blocked main turn without enough child-state visibility. That is a UI/state propagation bug, not the intended product experience.

- `Q-A16` **What should `wait=true` feel like in the UI?**
  - The main turn may still be waiting at protocol level, but the UI must immediately show the child agent, its live status, and any approval/question gate. The user should see "child waiting for approval", not "main hung".

- `Q-A17` **When should `wait=false` be used?**
  - Use `wait=false` when the main turn should continue immediately after spawning the child. The child runs in the background and reports through the Agents panel, graph, bus, or later focus navigation.

- `Q-A18` **Which path should the Agents panel button use?**
  - The button should use direct UI/API spawn, not prompt the main AI to call `spawn_agent`. This makes user-initiated spawning deterministic while preserving the same backend lifecycle.

## 14. Agent Result Sharing

This section defines how a finished child agent's result reaches the parent run, so the parent (main chat) can answer questions about its sub-agents and, in a later auto mode, act on their results automatically. It is the mechanism behind BUG-121, BUG-122, and BUG-126.

### 14.1 Principle: share results, not working context

- `D-6` Agents are isolated. A parent receives only a child's **final result** (the completed message, or the failure error) plus lightweight identity (agent name, provider, model). It never receives the child's transcript, tool calls, file diffs, or intermediate reasoning.
- This isolation is intentional multi-agent design: each agent reasons independently; only the outcome flows back. It keeps prompts bounded and prevents one agent's internal detail from polluting another's context.

### 14.2 Two delivery channels

A child result reaches the parent through exactly one of two channels, chosen by spawn mode:

| Spawn mode | wait | Delivery channel |
|------------|------|------------------|
| Tool (`spawn_agent`) | `true` | **Synchronous tool result.** `SpawnAgentResult` (runId, providerSessionId, providerKey, status, finalMessage) is returned as the tool-call result, landing natively in the parent's provider conversation in the same turn. |
| Tool (`spawn_agent`) | `false` | **Asynchronous injection** (see 14.3) — only `status: spawned` is returned synchronously, so the eventual result is injected later. |
| UI (`+ Spawn agent`) | `true` or `false` | **Asynchronous injection** — there is no main-AI turn awaiting a tool result, so the result is always injected. |

### 14.3 Asynchronous injection mechanism

- `D-7` The parent run carries a bounded `pendingAgentContext` buffer. When a child the parent needs to hear from finishes, a one-line note is appended:
  - completed: `Sub-agent "<name>" (provider: <p>) completed. Result: <finalMessage truncated>`
  - failed: `Sub-agent "<name>" (provider: <p>) failed: <error truncated>`
  - UI spawns also append a "started" note at spawn time (`provider`, `model`); tool spawns do not, because the tool call itself already records the spawn in provider history.
- On the parent's **next provider turn**, the buffered notes are composed into a `[FlowPilot system note: …]` block and prepended to the prompt sent to the provider — never to the user-visible prompt bubble. The buffer is then cleared; the note now lives in the provider session file as ordinary conversation history.
- `D-8` **No double delivery.** A note is injected iff the result was not already returned synchronously, i.e. when `uiInitiated || !waitForResult`. A tool spawn with `wait=true` is therefore never injected (its result is the tool result).

### 14.4 Persistence and restart

- The `pendingAgentContext` buffer is persisted in `sessions.ndjson` (`pending_agent_context`) and restored on resume, so a spawn-then-restart-before-asking still reaches the parent.
- Once a note has been folded into a turn, it survives restart for free: it is part of the provider session file that `seedTranscriptFromDisk` replays. This is the same durable channel that already makes synchronous tool results survive restart.

### 14.5 Foundation for auto agent-to-agent mode

- The parent accumulates child results between its turns and can summarize or react to them on its next turn. This is the building block for a future auto mode where the orchestrator launches background agents and consumes their results without a human prompt in between.
- Future inter-agent messaging (one child addressing another) is expected to reuse the same buffer plus the existing agent bus, keeping a single result-routing path rather than a second mechanism.

## 15. Runtime Behavior (Spawn, YOLO, Gating, Agent List, Slash, Lifecycle)

This section records the desktop/runner rules added while hardening the agent feature (BUG-124 … BUG-134, Task-086, Task-087) so the design stays in sync with the implementation.

### 15.1 YOLO inheritance (BUG-129)

- `D-9` A spawned child inherits the parent run's YOLO posture at creation: `spawnChildRun` copies `parentRun.yolo` into the child's `StartRunInput.YoloMode`. With YOLO on, the child's gated actions auto-approve instead of stalling the (often `wait=true`) parent on a child approval prompt.
- A per-turn YOLO override is sticky: `runTurn` writes an explicit `YoloMode` back to the run, so a spawn during that turn reads the live posture and later turns default to it. YOLO off still yields a YOLO-off child that requests approval.

### 15.2 Main-run gating on wait=true children (BUG-131, BUG-133)

- `D-10` A running child spawned with `wait=true` blocks the main run's chat send button and the spawn controls — even when the main has no turn of its own in flight (a UI `wait=true` spawn). The blocking signal is the agent summary's `waitForResult` flag (`hasBlockingChild` on the client), not the parent's turn status. `wait=false` children never block, so background concurrent spawns are allowed.

### 15.3 Agents panel list rendering (BUG-130, BUG-132)

- `D-11` The desktop agent list is fed by two sources: the live SSE `agent_graph_updated` snapshot (in-memory only) and the HTTP `listAgentRunSummaries` superset (live + historical + disk-persisted closed children). The client **merges** SSE updates into the list by `runId` (incoming wins) rather than replacing, so disk-only closed children stay visible while a new agent runs; the HTTP refresh remains the authoritative replace for the active parent. The list is de-duplicated by `runId`, sorted newest-first, and collapsed to the latest 5 per section with a show-more toggle.

### 15.4 Chat slash commands (Task-087, BUG-134)

- `D-12` The chat composer routes slash input by the command after `/`: `/s` (or `/skill…`) opens the skill picker UI; `/a` (or `/agent`) opens the spawn-agent panel (the same `openAgentSpawnGuide` UI as the right sidebar). A bare `/` does **not** open any picker — a command letter is required so the two namespaces (`/s` skills, `/a` agent) are explicit. `@name` mention routing is unchanged.

### 15.5 Child model resolution (spawn)

- `D-14` `spawnChildRun` resolves the child's model by priority: (1) the agent definition's `model` (with its `model_reasoning_effort`); else (2) if the child runs on the **same** provider as the parent, inherit the parent's current model/effort; else (3) the per-provider default (`defaultModelForProvider`). A still-empty model also falls back to the provider default so the UI always shows a concrete model. The same agent name therefore shows exactly the model used to start it.

### 15.6 Child lifecycle: delete cascades (Task-086)

- `D-15` Deleting a parent chat (`DELETE /client/workflow-runs/{runId}`) cascades to its child agent runs, so a deleted conversation does not leave orphaned children in history/sync. Child identity is additive, so deleting a parentless (normal) run behaves exactly as before.

## 16. Test


### Test 1: full - cases 18 & 20 FIXED by BUG-129 (child inherits parent YOLO)

1. Make sure YOLO is Off.
2. Start New Claude chat -> Prompt (Use spawn_agent exactly once with agent="reviewer", provider="codex", wait=true.
Child prompt: "Do not use tools. Reply exactly: Number 11."
After the child returns, tell me the exact child result.
) ----> Check result
3.  Prompt (Use spawn_agent exactly once with agent="reviewer", provider="claude", wait=true.
Child prompt: "Do not use tools. Reply exactly: Number 33."
After the child returns, tell me the exact child result.
) ----> Check result
4. New run - start with Codex
5. -> Prompt (Use spawn_agent exactly once with agent="reviewer", provider="codex", wait=true.
Child prompt: "Do not use tools. Reply exactly: Number 44."
After the child returns, tell me the exact child result.
) ----> Check result
6. Prompt (Use spawn_agent exactly once with agent="reviewer", provider="claude", wait=true.
Child prompt: "Do not use tools. Reply exactly: Number 22."
After the child returns, tell me the exact child result.
) ----> Check result
7. Back to claude chat -> switch xxx -> check
8. Ask: Do you know my mentioned number ? -> expect 11-33
9. Spawm new Claude Agent by UI -> Prompt (You are my claude sub-agent - DONT DO ANYTHING. Just return Like 66)
10. Spawm new Codex Agent by UI -> Prompt (You are my codex sub-agent - DONT DO ANYTHING. Just return Like 55)
11. Ask: Do you know my mentioned number ? -> expect 11-33-55-66
12. Back to codex chat -> switch xxx -> check
13. Ask: Do you know my mentioned number ? -> expect 22-44
14. Spawm new Claude Agent by UI -> Prompt (You are my claude sub-agent - DONT DO ANYTHING. Just return Like 77)
15. Spawm new Codex Agent by UI -> Prompt (You are my codex sub-agent - DONT DO ANYTHING. Just return 88)
16. Ask: Do you know my mentioned number ? -> expect 22-44-77-88
17. Back to claude chat - KEEP yolo off -> prompt (Use spawn_agent exactly once with agent="coder", provider="codex", wait=true.
Child prompt: "Create a file called child-agent-yolo-off.txt in the current directory with the text 'child approval works'. Then reply exactly: CHILD_APPROVAL_DONE."
Do not create the file yourself. Only the child agent should do it.) -> check APPROVE
18. YOLO = true and send again with file name = child-agent-yolo-on.txt -> check APPROVE   ----- **FIXED (BUG-129): with YOLO on, the child now auto-approves; no confirm.**
19. Switch to codex chat  - KEEP yolo off -> prompt (Use spawn_agent exactly once with agent="coder", provider="claude", wait=true.
Child prompt: "Create a file called child-agent-yolo-off.txt in the current directory with the text 'child approval works'. Then reply
exactly: CHILD_APPROVAL_DONE."
Do not create the file yourself. Only the child agent should do it.) -> check APPROVE
20. YOLO = true and send again with file name = child-agent-yolo-on.txt -> check APPROVE ----- **FIXED (BUG-129): with YOLO on, the child now auto-approves; no confirm.**
21. Re-start sever and continue with Test 2

### Test 2: regression test for YOLO - PASSED
3 prompts
Create a file called yolo-test.txt in the current directory with the text "yolo works" and after that is all skills name i mentioned

Using mcp google drive to let me know the current google email of this drive

Use the ask_user tool to ask me which programming language I prefer: Python, TypeScript, or Go. Then write a "Hello World" in whichever I pick.


- Hi with current acc
- Yolo off: full 3 prompts
- Change to other acc
- Hi with new acc
- Yolo off: full 3 prompts
- Switch to other chat -> back -> can see and continue other prompt
- Restart sever
- Open chat again
- can see old history
- change back previous acc - say hi
- YOLO = true
- Full 3 prompts now
- Sync
- Restart server
- Open chat after sync 
- YOLO = false
- 3 prompt continue


### Test 3: Wait semantics — blocking vs concurrent spawns (BUG-131, fixed in BUG-133) - PASSED

Validates the `wait` toggle's effect on the main run and on the ability to spawn more agents. NOTE: BUG-131's first attempt gated on the main *turn* status, which a UI `wait=true` spawn does not set, so this test originally FAILED (the main could still send/spawn). BUG-133 fixes it by exposing the spawn's `wait` flag on the agent summary (`waitForResult`) and blocking on a running `wait=true` child directly.

**wait=true → main blocks → no concurrent spawn**
1. In a chat, spawn agent 1 from the UI with **Wait for result = ON**. The prompt is (Do nothing, just see what did project do?)
2. Expected: the main run goes busy and stays blocked until agent 1 finishes — the chat send button AND the `+ Spawn agent` / `Spawn ▸` controls are disabled.
3. While agent 1 is still running, confirm you **cannot** start a second agent (the spawn control is disabled).
4. When agent 1 completes, the main run returns to idle and spawning is enabled again.

**wait=false → main continues → multiple concurrent spawns**
5. In a fresh chat, spawn agent A with **Wait for result = OFF**.
6. Expected: the spawn dispatches and the main run does **not** block (it returns to idle); agent A streams in the Agents panel in the background.
7. While agent A is still running, spawn agent B (wait=false) and then agent C (wait=false).
8. Expected: A, B, and C all run concurrently and appear in the Agents panel (newest on top, collapsing past 5 — see BUG-130); the count reflects all running children without flicker.
9. Mixed check: with background agents A/B/C running, starting a `wait=true` spawn blocks the main run as in step 2; the background agents keep running independently.
