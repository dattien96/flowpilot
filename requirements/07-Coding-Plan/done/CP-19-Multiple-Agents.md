# CP-19: Multiple Agents

## Metadata

- Document ID: `CP-19`
- Title: `Multiple Agents (Sub-Agent Orchestration)`
- Phase: `coding_plan`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-19`
- Last Updated: `2026-06-23`
- Parent Documents: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md), [SD-14: Codex Cross-Account Chat Resume And Home Sync](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md)
- Child Documents: [Task-081: Agent Abstraction And Catalog Loader](../../08-Task/done/Task-081-Agent-Abstraction-And-Catalog-Loader.md), [Task-082: Spawn-Agent Tool And Orchestrator Core](../../08-Task/done/Task-082-Spawn-Agent-Tool-And-Orchestrator-Core.md), [Task-083: Desktop Agents Panel And Focus Navigation](../../08-Task/todo/Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md), [Task-084: Dependency Feedback Loop And Orchestration Board](../../08-Task/todo/Task-084-Dependency-Feedback-Loop-And-Orchestration-Board.md), [Task-085: Flow-Mode Supabase Agent Runs And Message Bus](../../08-Task/todo/Task-085-Flow-Mode-Supabase-Agent-Runs-And-Message-Bus.md), [Task-086: Delete Chat Cascades To Child Agents](../../08-Task/done/Task-086-Delete-Chat-Cascades-To-Child-Agents.md), [Task-087: Chat Slash Commands For Skill And Agent UI](../../08-Task/done/Task-087-Chat-Slash-Commands-For-Skill-And-Agent-UI.md)
- Related Documents: [CP-09: AI Orchestration](../done/CP-09-AI-Orchestration.md), [CP-17: Workflow Chat And Session](../done/CP-17-Workflow-Chat-And_Session.md), [CP-18: Refactor Workflow With Session](../done/CP-18-Refactor-Workflow-With_Session.md)
- Replaces: `None`
- Tags: `multi-agent, sub-agent, orchestration, chat, workflow, local-runner, desktop, provider-adapter`

## AI Quick View

### Summary

- Introduce a first-class **Agent** concept layered on the existing `interactiveRun` + `ProviderRuntimeAdapter` machinery; a sub-agent is a normal chat run tagged with a parent and a role, so it reuses the current Codex/Claude `SendTurn`, SSE streaming, approvals, token usage, and chat persistence.
- Add an **AgentCatalog** that loads agent definitions from `.claude/agents/` and `.codex/agents/` (plus FlowPilot built-ins), mirroring the runtime skill discovery in `interactive_catalog.go`. These directories exist but are empty today, so the loader and a built-in set must be created.
- Add a **`spawn_agent`** provider tool (modeled on the existing `ask_user` tool) plus a desktop spawn UI; a parent can spawn children that run in the background or block the parent turn until they finish.
- Add an **AgentOrchestrator** that owns a dependency graph and a message bus to coordinate parallel agents (the canonical case: a `coder` and a `reviewer` running in parallel, where the reviewer waits for the coder's diff, returns feedback, and the coder iterates to a round cap).
- Deliver in two phases: **Phase 1 chat mode** (in-memory orchestrator, agent tree persisted in the existing chat NDJSON/Drive sync, no schema migration); **Phase 2 flow mode** (graph + bus formalized as Supabase `agent_runs` and `agent_messages`, integrated with the workflow engine).

### Current Ask

- Plan a scoped, two-phase implementation that lets the main agent (and the user) spawn and manage multiple sub-agents in the desktop chat UI, with a Graph/DAG management board, inline "agent running" highlighting, a right-side Agents panel, focus + back-to-main navigation, and a reviewer-gates-coder feedback loop.
- Produce a work breakdown and per-task DoD that can drive code + test verification later, without regressing existing chat, workflow, session-resume, or Drive-sync behavior.

### Key Decisions

- `P-1` A sub-agent is an `interactiveRun` extended with `parentRunId`, `agentName`, `role`, `dependsOn[]`, and `agentStatus`. Do **not** build a parallel session/transport/event system; reuse the adapter + SSE + persistence that normal chat already uses.
- `P-2` Spawning is triggered two ways through one backend path: an AI-callable `spawn_agent` tool (modeled on `ask_user` in `codex_adapter.go`) and a UI spawn action (`+ Spawn agent` / `@agent` mention). `wait:true` blocks the parent turn until the child completes or enters an approval/question gate, returning either the child's final message or a waiting status; `wait:false` runs in the background.
- `P-3` Agent definitions load from disk via a new `AgentCatalog` (`.claude/agents/`, `.codex/agents/`, FlowPilot built-ins) using `project > provider-home` precedence, in the standard agent frontmatter format (`name`, `description`, `tools`, `model`, system-prompt body).
- `P-4` Inter-agent coordination is owned by an `AgentOrchestrator` (dependency graph + message bus) analogous to `WorkflowOrchestrator`. The reviewer-gate is the agent-driven analogue of the existing human approval gate.
- `P-5` The multi-agent management view is a **Graph/DAG board**: agents are nodes, dependencies and feedback are edges, with a live agent-bus log and run controls (resume, pause, inject feedback, add agent, stop all).
- `P-6` Phase 1 ships entirely in chat mode with no Supabase migration (agent tree in the chat run manifest / NDJSON). Phase 2 adds `agent_runs` + `agent_messages` and reuses `workflow_provider_sessions`/`workflow_provider_events` per agent run for streaming and cross-PC resume.
- `P-7` Different agents may use different providers (e.g. `coder` = Claude, `reviewer` = Codex); each child run owns its own provider session and SSE stream.
- `P-8` The desktop **Agents panel is a persistent, mode-agnostic right-rail component**, not a tab that replaces existing panels. It stacks between the existing **MODE** panel and the **ACCOUNTS** panel and stays visible in both Chat and Workflow modes. The header gains a `N agents running` status pill; the composer gains an `@` agent-mention affordance alongside the existing `/` skill picker; the timeline keeps inline `spawn_agent` rows and per-agent running/waiting banners. All existing chrome (title bar, `Chat | Settings`, PROJECTS/HISTORY/REMOTE CHATS, the SKILLS/PROVIDER/MODEL composer, MODE, ACCOUNTS) is preserved unchanged.
- `P-9` Child approval/question gates remain owned by the child's own stream; the Agents panel/Graph board surface waiting state and focus links without auto-approving or auto-answering.
- `P-10` Task-083 handles only conservative `@mention` routing to idle/completed children; Task-084 queues busy-child mentions/feedback on the agent bus and does not interrupt active provider turns unless Stop is pressed.

### Constraints

- Before any edit to a Go or TypeScript symbol, run GitNexus impact analysis and report the blast radius (CLAUDE.md mandate); warn on HIGH/CRITICAL risk before proceeding.
- Do not change the `ProviderRuntimeAdapter` interface, the `ProviderEvent`/`ProviderEventDTO` union semantics, or the SSE transport contract except by additive fields.
- Do not regress normal chat, workflow runs, session resume (SD-14), cross-PC chat sync, or Google Drive sync.
- Phase 1 must not require a Supabase migration; Phase 2 migrations must be additive and backward compatible with single-agent runs.
- Keep child-run identity additive on `interactiveRun` so existing runs (no parent, no role) behave exactly as today.

### Open Questions

- Should multi-agent get its own upstream `SS`/`SD` pair, or is extending SS-11 (Workflow With Session) sufficient? This CP currently links SS-11 / SD-14 as the closest upstream and may need a dedicated SD before Phase 2 migrations.
- Should the reviewer↔coder round cap become per-agent-definition configurable after Phase 1? Task-084 uses a default cap of 3 with an optional per-run override from the board.
- For background (`wait:false`) children, what is the idle/cleanup policy relative to the existing session idle sweeper?

### Source Refs

- `SS-11` Workflow With Session; `SD-14` cross-account resume and home sync.
- `apps/local-runner/internal/runner/provider_registry.go` (`ProviderRuntimeAdapter`, `TurnBridge`, `interactiveRun`).
- `apps/local-runner/internal/runner/codex_adapter.go` (`ask_user` tool registration pattern for `spawn_agent`).
- `apps/local-runner/internal/runner/interactive_catalog.go` (skill discovery pattern for `AgentCatalog`).
- `apps/local-runner/internal/runner/interactive_handlers.go` (SSE event stream).
- `apps/local-runner/internal/runner/workflow_orchestrator.go`, `workflow_state_machine.go` (orchestrator pattern).
- `apps/desktop-flowpilot/src/components/ChatWorkspace.tsx`, `Timeline.tsx`, `ChatInput.tsx`, `state/store.ts`, `client/HttpWsRunnerClient.ts`, `types/contract.ts`.
- Design mock: `output/cp19-multi-agent-mockup.html` (architecture diagram + early frames). Authoritative UI reference (drawn on the live desktop chrome): `output/cp19-frame01-redesign.html` (Frame 01 — main chat + mode-agnostic AGENTS right-rail panel), `output/cp19-frames-02-04-redesign.html` (Frame 02 sub-agent focus + back-to-main, Frame 04 Graph/DAG orchestration board), `output/cp19-frame03-redesign.html` (Frame 03 spawn dialog).

## 1. Goal

Let the FlowPilot main agent and the user run multiple AI agents in parallel within one project/session, observe and navigate them from the desktop chat UI, and coordinate them (notably a reviewer agent gating a coder agent through a feedback loop) — first in chat mode (Phase 1) and then formalized in flow mode on Supabase (Phase 2). The implementation must reuse the existing provider-adapter, SSE, and persistence stack rather than introducing a parallel runtime.

## 2. Input Documents

- [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- [SD-14: Codex Cross-Account Chat Resume And Home Sync](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md)
- [CP-09: AI Orchestration](../done/CP-09-AI-Orchestration.md), [CP-17](../done/CP-17-Workflow-Chat-And_Session.md), [CP-18](../done/CP-18-Refactor-Workflow-With_Session.md)
- Design mock: [`output/cp19-multi-agent-mockup.html`](../../../output/cp19-multi-agent-mockup.html)

## 3. Implementation Strategy

- **Overall approach:** Add identity + a coordinator on top of existing primitives. (1) Extend `interactiveRun` with agent identity. (2) Build an `AgentCatalog` disk loader. (3) Add a `spawn_agent` tool + UI through one backend spawn path. (4) Add an `AgentOrchestrator` (graph + bus) for dependencies and feedback loops. (5) Surface everything in the desktop UI (Agents panel, inline highlight, focus/back-to-main, Graph/DAG board). (6) In Phase 2, persist the graph + bus to Supabase and wire the workflow engine.
- **Sequencing logic:** Phase 1 (chat) = Tasks 081→082→083→084, each independently demoable. Phase 2 (flow) = Task 085, which depends on the Phase 1 orchestrator contract being stable.
- **Dependencies:** Task-082 depends on Task-081 (identity + catalog). Task-083 depends on Task-082 (child runs + list/spawn endpoints and client-side focus attach). Task-084 depends on Task-082/083 (orchestrator + panel). Task-085 depends on Task-084 (stable graph/bus contract).

## 4. Work Breakdown

- `P-1` [Task-081](../../08-Task/done/Task-081-Agent-Abstraction-And-Catalog-Loader.md) — Extend `interactiveRun` with agent identity (`parentRunId`, `agentName`, `role`, `dependsOn[]`, `agentStatus`); add `AgentDefinition` + `AgentCatalog` disk loader (`.claude/agents`, `.codex/agents`, built-ins); expose `listAgents` in the runner contract.
- `P-2` [Task-082](../../08-Task/done/Task-082-Spawn-Agent-Tool-And-Orchestrator-Core.md) — Add the `spawn_agent` provider tool + a minimal `AgentOrchestrator` (spawn / wait / list); child runs stream over their own SSE; new runner endpoints for spawning/listing child runs plus client-side focus attach via `streamRun(runId)`.
- `P-3` [Task-083](../../08-Task/todo/Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md) — Desktop Agents panel as a persistent, mode-agnostic right-rail component (between MODE and ACCOUNTS; visible in Chat and Workflow), agent run cards (running/waiting/completed/failed), header `N agents running` pill, inline "agent running/waiting" highlight in the Timeline, focus-into-child + back-to-main breadcrumb with stream replay, `+ Spawn agent` dialog + conservative `@mention` routing.
- `P-4` [Task-084](../../08-Task/todo/Task-084-Dependency-Feedback-Loop-And-Orchestration-Board.md) — Dependency edges + message bus (`ready-for-review`, `changes-requested`, `approved`, `user-feedback`) driving the coder↔reviewer loop with a default round cap of 3; graph snapshot + additive parent-run SSE updates; the Graph/DAG orchestration board with controls and a live bus log.
- `P-5` [Task-085](../../08-Task/todo/Task-085-Flow-Mode-Supabase-Agent-Runs-And-Message-Bus.md) — Phase 2: Supabase `agent_runs` + `agent_messages` (additive migration), `workflow_runs.parent_run_id`, reuse existing `workflow_provider_sessions`/`events` by writing each durable child agent against its own `workflow_run_id`, and workflow-engine integration so a workflow step can be an agent with a reviewer gate.

### 4.1 Phase 1 Hardening Deltas (implemented after the original breakdown)

These were not in the initial P-1…P-5 plan but are now implemented and verified; their design rules live in SD-16 (§7.3, §14, §15) and are listed here so CP-19 stays in sync with the code. Each links its BugFix/Task.

- `P-11` **Result sharing back to the parent** — beyond `wait=true` returning the child's final message, UI spawns and `wait=false` tool spawns deliver the child's result asynchronously by folding a bounded `pendingAgentContext` note into the parent's next provider turn (persisted, survives restart). Each agent stays isolated — only the final result is shared, never the child's transcript. (BUG-121, BUG-122, BUG-126; SD-16 §14)
- `P-12` **Provider-consistent spawn prompt** — one helper composes the first child turn identically for every provider (system prompt first, then an identity line linking the agent definition file, then the user prompt). (BUG-128; SD-16 §7.3)
- `P-13` **Codex `spawn_agent` name parity** — the tool is registered as `flowpilot_spawn_agent` on Codex (the public name is reserved) and normalized back at the boundary; legacy rollouts migrate on resume. (BUG-124, BUG-127; SD-16 `D-13`)
- `P-14` **Child YOLO inheritance** — a child inherits the parent's YOLO posture; an explicit per-turn YOLO override is sticky on the run. (BUG-129; SD-16 §15.1)
- `P-15` **Main-run gating on a running `wait=true` child** — the desktop blocks the send button and spawn controls while a `wait=true` child runs (via the `waitForResult` summary flag), independent of the parent's turn status; `wait=false` children allow concurrent spawns. (BUG-131, BUG-133; SD-16 §15.2)
- `P-16` **Agents panel list correctness** — merge SSE + HTTP by runId so disk-persisted closed children stay visible, dedup, sort newest-first, collapse past 5. (BUG-130, BUG-132; SD-16 §15.3)
- `P-17` **Composer slash namespaces** — `/s` opens skills, `/a` opens the spawn-agent panel, bare `/` opens nothing. (Task-087, BUG-134; SD-16 §15.4)
- `P-18` **Child model resolution** — agent-definition model → same-provider inheritance → per-provider default; the UI shows the exact model used. (SD-16 §15.5)
- `P-19` **Delete-chat cascade** — deleting a parent chat removes its child agent runs. (Task-086; SD-16 §15.6)
- `P-20` **Sync/restore preserves the agent tree** — remote restore keeps parent/child agent chats nested instead of flattening and losing children (the Phase-1 chat-sync analogue of R-4). (BUG-123)

## 5. Touched Areas

- **files (backend):** `apps/local-runner/internal/runner/provider_registry.go` (interactiveRun fields, TurnBridge), `codex_adapter.go` + `claude_adapter.go` (spawn_agent tool), `interactive_service.go` + `interactive_handlers.go` (child run creation, spawn/list endpoints, SSE), new `agent_catalog.go`, new `agent_orchestrator.go`.
- **files (frontend):** `apps/desktop-flowpilot/src/components/ChatWorkspace.tsx`, `Timeline.tsx`, `ChatInput.tsx`, new `AgentsPanel.tsx`, new `OrchestrationBoard.tsx`, `state/store.ts`, `client/HttpWsRunnerClient.ts`, `types/contract.ts`, `styles.css`.
- **modules:** local-runner interactive + orchestration; desktop chat workspace + state.
- **database:** none in Phase 1; Phase 2 adds `agent_runs`, `agent_messages`, and `workflow_runs.parent_run_id` via additive migrations under `supabase/migrations/`.
- **external systems:** Google Drive chat sync (agent tree appended to the existing manifest in Phase 1).

## 6. Data or Migration Steps

- **Phase 1 schema:** none. Persist the agent tree (`parentRunId`, `agentName`, `role`, `agentStatus`) in the chat run manifest / NDJSON used by `chat_session_sync.go`.
- **Phase 2 schema:** `ALTER TABLE workflow_runs ADD parent_run_id`; `agent_runs` (id, workflow_run_id unique when present, parent_agent_run_id, root_workflow_run_id, project_id, workflow_id, agent_name, role, provider, model, status, depends_on jsonb, round_index, round_cap, spawned_by, created_at, closed_at, summary); `agent_messages` (id, root_workflow_run_id, from_agent_run_id, to_agent_run_id, kind, payload_json, artifact_id, created_at, consumed_at). Reuse existing `workflow_provider_sessions`/`workflow_provider_events` with each child agent's `workflow_run_id`; do not add `agent_run_id` to provider telemetry in Phase 2 unless a later design requires it.
- **data backfill:** none (existing runs are single-agent / parentless by default).
- **config updates:** ship FlowPilot built-in agent definitions (coder, reviewer, tester) in-repo so the catalog is non-empty on first run.

## 7. Validation Plan

- **tests to add:** Go unit tests for `AgentCatalog` discovery/precedence, child `interactiveRun` lifecycle, `spawn_agent` wait/no-wait semantics, and `AgentOrchestrator` dependency + feedback-loop transitions (pure-logic state, mirroring `workflow_state_machine` tests). HTTP/contract tests cover graph snapshot, bus history, additive SSE event DTOs, and loop controls. Frontend tests cover store agent state, panel rendering, focus/back-to-main, Graph/DAG board rendering, live bus updates, controls, and `@mention` routing.
- **manual checks:** spawn coder + reviewer from a chat; confirm inline highlight, panel states, focus + back-to-main, the reviewer→coder feedback loop on the Graph/DAG board, and that normal single-agent chat is unchanged.
- **failure cases:** child provider session dies mid-turn; reviewer never approves (round cap reached); parent turn interrupted while a `wait:true` child is running; spawning an unknown agent name; background child idle-cleanup.

## 8. Rollout and Fallback

- **rollout order:** Task-081 → 082 → 083 → 084 (Phase 1, behind a feature flag if needed) → Task-085 (Phase 2).
- **fallback path:** identity fields are additive and default to "main/parentless"; disabling the spawn tool + hiding the Agents panel reverts the app to current single-agent behavior with no data changes.
- **monitoring:** runner logs per child run (spawn, status transitions, bus messages); agent-bus log surfaced in the orchestration board.

## 9. Risks

- `R-1` Scope creep into a full agent framework — mitigated by reusing `interactiveRun`/adapters and shipping Phase 1 chat-only first.
- `R-2` Concurrency/streaming races with multiple simultaneous child SSE streams — mitigate with per-run locking already used in the interactive service and orchestrator.
- `R-3` Feedback loop non-termination — mitigate with a mandatory round cap + explicit stop control.
- `R-4` Cross-PC resume of a multi-agent tree (Phase 2) — mitigate by reusing the durable `workflow_provider_sessions` model and reconstructing the tree from `agent_runs`.
- `R-5` Missing dedicated upstream SD/SS for multi-agent — flagged as an open question; resolve before Phase 2 migrations.

## 10. Definition of Done

- A user can spawn one or more sub-agents from the desktop chat (UI + AI-driven), see them listed in the Agents panel with correct running/waiting/completed/failed states and role colors.
- Inline "agent running" highlighting appears in the Timeline; clicking an agent focuses its live stream; back-to-main returns in one click without losing cached main run state.
- A coder and a reviewer can run in parallel where the reviewer waits for the coder's diff, returns feedback, and the coder iterates to a round cap, all observable on the Graph/DAG orchestration board with a live bus log.
- Agent definitions load from `.claude/agents` / `.codex/agents` / built-ins with correct precedence.
- Normal single-agent chat, workflow runs, session resume, and Drive sync are unchanged (regression tests green).
- Phase 2: `agent_runs` + `agent_messages` migrations applied additively; a workflow step can run as an agent with a reviewer gate; multi-agent run resumes after a runner restart.
