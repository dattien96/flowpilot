# Task-083: Desktop Agents Panel And Focus Navigation

## Metadata

- Document ID: `Task-083`
- Title: `Desktop Agents Panel And Focus Navigation`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-19`
- Last Updated: `2026-06-19`
- Parent Documents: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- Child Documents: `None`
- Related Documents: [Task-082: Spawn-Agent Tool And Orchestrator Core](../done/Task-082-Spawn-Agent-Tool-And-Orchestrator-Core.md), [Task-084: Dependency Feedback Loop And Orchestration Board](./Task-084-Dependency-Feedback-Loop-And-Orchestration-Board.md)
- Replaces: `None`
- Tags: `multi-agent, desktop, react, zustand, agents-panel, ui`

## AI Quick View

### Summary

- Add an Agents panel to the desktop right sidebar as a persistent, mode-agnostic component (stacked between the existing MODE and ACCOUNTS panels; visible in both Chat and Workflow modes), listing the current main run plus Task-082 child summaries with running/waiting/completed/failed states and role colors.
- Add inline "agent running/waiting" highlighting in the Timeline where agent-run events are visible, plus a header "N agents running" indicator derived from refreshed child summaries.
- Add focus-into-child (click a card -> attach the Timeline to that child's SSE stream through `focusAgentRun(runId)`) with a one-click back-to-main breadcrumb that replays the main run stream; add the `+ Spawn agent` dialog and conservative `@mention` routing in the composer.

### Current Ask

- Build the Phase 1 multi-agent chat UI against the Task-082 contract, matching the design mock frames 01-03. Frame 04 Graph/DAG orchestration board remains Task-084.

### Key Decisions

- `T-1` The Agents panel is a persistent right-rail component stacked between the existing MODE and ACCOUNTS panels; it is NOT a tab and does not hide MODE/ACCOUNTS. It renders identically in Chat and Workflow modes. All existing chrome (title bar, `Chat | Settings`, PROJECTS/HISTORY/REMOTE CHATS, SKILLS/PROVIDER/MODEL composer, MODE, ACCOUNTS) stays unchanged.
- `T-2` Store gains `agentRuns[]`, `activeAgentRunId`, `mainRunId`, per-run timeline/status/pending maps, `refreshAgentRuns()`, `focusAgentRun()`, and `focusMainRun()`. Because the current `HttpWsRunnerClient` keeps one active SSE stream, focusing a child aborts the active stream, preserves cached main timeline state, and replays/resubscribes to the main run on back-to-main.
- `T-3` Role accent colors: main `--accent`, coder `--ok`, reviewer `--ask`, tester `--warn`.
- `T-4` Phase 1 `@mention` routing targets existing idle/completed child runs only; busy/waiting children are not interrupted or queued in this task. If no matching child exists, the composer should offer/open the spawn dialog with that agent preselected when possible.

### Constraints

- Run GitNexus impact analysis before editing `store.ts`, `ChatWorkspace.tsx`, `Timeline.tsx`, or `ChatInput.tsx`; warn on HIGH/CRITICAL.
- Use existing vanilla CSS variables and theme; no new UI library.
- Single-agent chat with no children must render exactly as today.

### Open Questions

- Queue/interrupt semantics for busy `@mention` targets are deferred to Task-084.

### Source Refs

- `CP-19` P-1, P-2, P-5, P-8; Task-082 `listAgentRuns`, `spawnAgent`, and client-side `focusAgentRun`; mock `output/cp19-frame01-redesign.html`, `output/cp19-frames-02-04-redesign.html` (Frame 02 only for this task), `output/cp19-frame03-redesign.html`; `ChatWorkspace.tsx`, `Timeline.tsx`, `ChatInput.tsx`, `store.ts`.

## 1. Goal

Give the user a clear, navigable multi-agent chat experience: see all agents and their states, jump into any child's live chat, return to main in one click, and spawn/route agents.

## 2. Parent Links

- coding plan: [CP-19](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- tech design: [SD-14](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md)
- system spec: [SS-11](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- specific upstream ids: `CP-19` P-5

## 3. Trigger

CP-19 requires the chat UI to show running agents, list children, focus into them, and return to main; none of this exists in the desktop app today.

## 4. Exact Change

- `T-1` New `AgentsPanel.tsx` as a persistent right-rail panel rendered between MODE and ACCOUNTS (both modes); synthesize a main-run card from current store state and render child cards from `listAgentRuns(mainRunId)` with status dots, role color, `createdAt`, `dependsOn`, and `agentStatus`. Provider/token details are optional in Phase 1 unless already known from local spawn/stream events.
- `T-2` Extend `store.ts` with `agentRuns[]`, `activeAgentRunId`, `mainRunId`, per-run timeline/status/pending maps, `refreshAgentRuns()`, `focusAgentRun()`, and `focusMainRun()`. Panel state refreshes via Task-082 `listAgentRuns` after spawn, on focus/back, after terminal events, and via a lightweight interval while a multi-agent run is active.
- `T-3` Timeline highlighting: role-colored left accent on focused agent items when the active timeline is a child run, plus "N agents running" header indicator derived from `agentRuns`.
- `T-4` Focus + back-to-main breadcrumb bar in the Timeline header. Focus must use `RunnerClient.focusAgentRun(runId)` / `streamRun(runId)` and must preserve cached main timeline state for replay when returning.
- `T-5` `+ Spawn agent` dialog populated by `listAgents(cwd)` and submitted through `spawnAgent({ parentRunId: mainRunId, ... })`.
- `T-6` `@mention` routing in `ChatInput.tsx`: detect an initial `@agentName` token, route the remaining prompt to the matching idle/completed child run through the normal turn path for that child, and show a clear inline blocked state for busy/waiting children. Do not implement queueing, interrupting, dependency feedback, or message bus semantics here.

## 5. Touched Areas

- files: `apps/desktop-flowpilot/src/components/ChatWorkspace.tsx`, `Timeline.tsx`, `ChatInput.tsx`, new `AgentsPanel.tsx`, `state/store.ts`, `client/HttpWsRunnerClient.ts`, `client/MockRunnerClient.ts`, `types/contract.ts`, `styles.css`
- modules: desktop chat workspace + state
- routes: consumes Task-082 `GET /client/agents`, `GET /client/workflow-runs/{runId}/agents`, `POST /client/workflow-runs/{runId}/spawn-agent`, and existing run SSE streams; no new backend focus route in this task
- tables: none

## 6. Acceptance Check

- Spawning agents shows them in the panel with correct states and role colors; the header shows the running count from refreshed child summaries.
- Clicking a child focuses its live stream through `focusAgentRun(runId)`; back-to-main returns in one click and restores/replays the cached main timeline without losing main run state.
- `@coder ...` routes a message to an existing idle/completed coder child; busy/waiting children show a clear blocked state and are not queued or interrupted.
- `+ Spawn agent` lists catalog agents, spawns one through Task-082, refreshes the panel, and works in mock/dev mode through `MockRunnerClient`.
- A session with no children is visually unchanged from today.
- Frontend tests cover store agent state refresh, focus/back-to-main replay, empty/no-child rendering, spawn dialog submission, and `@mention` idle-vs-busy behavior.

## 7. Out of Scope

- The Graph/DAG orchestration board and feedback loop (Task-084); Supabase (Task-085).

## 8. Completion Notes

- result:
- follow-ups:
- upstream docs updated:
