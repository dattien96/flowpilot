# Task-082: Spawn-Agent Tool And Orchestrator Core

## Metadata

- Document ID: `Task-082`
- Title: `Spawn-Agent Tool And Orchestrator Core`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-19`
- Last Updated: `2026-06-19`
- Parent Documents: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- Child Documents: `None`
- Related Documents: [Task-081: Agent Abstraction And Catalog Loader](./Task-081-Agent-Abstraction-And-Catalog-Loader.md), [Task-083: Desktop Agents Panel And Focus Navigation](./Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md)
- Replaces: `None`
- Tags: `multi-agent, local-runner, spawn-agent, orchestrator, sse, go`

## AI Quick View

### Summary

- Add a `spawn_agent` provider tool modeled on the existing `ask_user` tool registration, available to Codex and Claude turns.
- Mint child `interactiveRun`s (using Task-081 identity) that stream over their own SSE; support `wait:true` (block parent turn, return child final message) and `wait:false` (background).
- Add a minimal `AgentOrchestrator` that tracks the run tree and exposes spawn / list / focus operations and child-run endpoints.

### Current Ask

- Implement the single backend spawn path + child-run lifecycle + list/focus endpoints, with tests. No dependency graph/feedback loop yet (Task-084) and no UI (Task-083).

### Key Decisions

- `T-1` One spawn path serves both the AI tool and the future UI action.
- `T-2` `wait:true` blocks the parent turn via the orchestrator and returns the child's final message as the tool result; `wait:false` returns a child run handle immediately and streams in the background.
- `T-3` Each child run owns its own provider session + SSE stream; children may use a different provider than the parent.

### Constraints

- Run GitNexus impact analysis before editing `codex_adapter.go`, `claude_adapter.go`, `interactive_service.go`, or `interactive_handlers.go`; warn on HIGH/CRITICAL.
- Reuse existing per-run locking; do not introduce a new transport or event type.
- Respect the idle sweeper for background children.

### Open Questions

- Background child idle-cleanup policy relative to the existing idle TTL.
- Whether a child approval/question gate bubbles to the parent timeline.

### Source Refs

- `CP-19` P-1, P-2, P-7; `codex_adapter.go` (`ask_user`); `interactive_service.go`; `interactive_handlers.go`.

## 1. Goal

Enable the main agent (and, via the same path, the UI) to spawn sub-agent runs that stream independently, with wait/no-wait semantics and basic tree tracking.

## 2. Parent Links

- coding plan: [CP-19](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- tech design: [SD-14](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md)
- system spec: [SS-11](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- specific upstream ids: `CP-19` P-2

## 3. Trigger

CP-19 requires a mechanism for the AI to request new agents; the spawn path and child lifecycle do not exist yet.

## 4. Exact Change

- `T-1` Register a `spawn_agent({agent, prompt, provider?, dependsOn?, wait})` tool in the Codex and Claude adapters (mirroring `ask_user`).
- `T-2` Implement child `interactiveRun` creation, its own SSE stream, and wait/no-wait resolution through a new `AgentOrchestrator`.
- `T-3` Add runner endpoints + `RunnerClient` methods: `listAgentRuns(parentRunId)`, `focusAgentRun(runId)` (returns the child's stream handle), and child-run interrupt.
- `T-4` Persist the agent tree into the chat run manifest (Phase 1, no schema migration).

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/codex_adapter.go`, `claude_adapter.go`, `interactive_service.go`, `interactive_handlers.go`, new `agent_orchestrator.go`, `chat_session_sync.go`, `apps/desktop-flowpilot/src/client/HttpWsRunnerClient.ts`, `types/contract.ts`
- modules: local-runner interactive + orchestration
- routes: `POST /client/workflow-runs/{runId}/spawn-agent`, `GET /client/workflow-runs/{runId}/agents`
- tables: none (Phase 1)

## 6. Acceptance Check

- A `spawn_agent` tool call with `wait:false` returns a child handle and the child streams over its own SSE; `wait:true` blocks the parent turn and returns the child's final message.
- `listAgentRuns` returns the run tree; `focusAgentRun` attaches to a child stream.
- The agent tree survives a chat sync/restore round-trip.

## 7. Out of Scope

- Dependency edges, message bus, feedback loop (Task-084); all UI (Task-083); Supabase (Task-085).

## 8. Completion Notes

- result:
- follow-ups:
- upstream docs updated:
