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
- Related Documents: [Task-082: Spawn-Agent Tool And Orchestrator Core](./Task-082-Spawn-Agent-Tool-And-Orchestrator-Core.md), [Task-084: Dependency Feedback Loop And Orchestration Board](./Task-084-Dependency-Feedback-Loop-And-Orchestration-Board.md)
- Replaces: `None`
- Tags: `multi-agent, desktop, react, zustand, agents-panel, ui`

## AI Quick View

### Summary

- Add an Agents panel to the desktop right sidebar as a persistent, mode-agnostic component (stacked between the existing MODE and ACCOUNTS panels; visible in both Chat and Workflow modes), listing the run tree (main + children) with running/waiting/blocked/closed states and role colors.
- Add inline "agent running" highlighting in the Timeline (role-colored accent on agent-produced items) and a header "N agents running" indicator.
- Add focus-into-child (click a card → swap Timeline to that child's SSE stream) with a one-click back-to-main breadcrumb; add the `+ Spawn agent` dialog and `@mention` routing in the composer.

### Current Ask

- Build the Phase 1 multi-agent chat UI against the Task-082 endpoints, matching the design mock frames 01–03 and 05. Orchestration board is Task-084.

### Key Decisions

- `T-1` The Agents panel is a persistent right-rail component stacked between the existing MODE and ACCOUNTS panels; it is NOT a tab and does not hide MODE/ACCOUNTS. It renders identically in Chat and Workflow modes. All existing chrome (title bar, `Chat | Settings`, PROJECTS/HISTORY/REMOTE CHATS, SKILLS/PROVIDER/MODEL composer, MODE, ACCOUNTS) stays unchanged.
- `T-2` Store gains `agentRuns[]`, `activeAgentId`, and `focusAgent()`; the main stream keeps running in the background when a child is focused.
- `T-3` Role accent colors: main `--accent`, coder `--ok`, reviewer `--ask`, tester `--warn`.

### Constraints

- Run GitNexus impact analysis before editing `store.ts`, `ChatWorkspace.tsx`, `Timeline.tsx`, or `ChatInput.tsx`; warn on HIGH/CRITICAL.
- Use existing vanilla CSS variables and theme; no new UI library.
- Single-agent chat with no children must render exactly as today.

### Open Questions

- Should `@mention` to a busy child queue after its turn or interrupt it?

### Source Refs

- `CP-19` P-1, P-2, P-5; mock `output/cp19-multi-agent-mockup.html` frames 01–03, 05; `ChatWorkspace.tsx`, `Timeline.tsx`, `ChatInput.tsx`, `store.ts`.

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

- `T-1` New `AgentsPanel.tsx` as a persistent right-rail panel rendered between MODE and ACCOUNTS (both modes); render agent cards with status dots, role color, provider pill, token/last-activity, a `+ Spawn agent` button, and a "recently closed" section.
- `T-2` Extend `store.ts` with `agentRuns[]`, `activeAgentId`, `focusAgent()`, and reducers consuming child-run events.
- `T-3` Timeline highlighting: role-colored left accent on agent items + "N agents running" header indicator.
- `T-4` Focus + back-to-main breadcrumb bar in the Timeline header.
- `T-5` `+ Spawn agent` dialog (populated by `listAgents`) and `@mention` routing in `ChatInput.tsx`.

## 5. Touched Areas

- files: `apps/desktop-flowpilot/src/components/ChatWorkspace.tsx`, `Timeline.tsx`, `ChatInput.tsx`, new `AgentsPanel.tsx`, `state/store.ts`, `client/HttpWsRunnerClient.ts`, `styles.css`
- modules: desktop chat workspace + state
- routes: consumes Task-082 endpoints
- tables: none

## 6. Acceptance Check

- Spawning agents shows them in the panel with correct states and role colors; the header shows the running count.
- Clicking a child focuses its live stream; back-to-main returns in one click without losing the main stream.
- `@coder …` routes a message to the coder child; `+ Spawn agent` lists catalog agents and spawns one.
- A session with no children is visually unchanged from today.

## 7. Out of Scope

- The Graph/DAG orchestration board and feedback loop (Task-084); Supabase (Task-085).

## 8. Completion Notes

- result:
- follow-ups:
- upstream docs updated:
