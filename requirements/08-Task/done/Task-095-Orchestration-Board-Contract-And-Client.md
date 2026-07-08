# Task-095: Orchestration Board, Contract, And Client

## Metadata

- Document ID: `Task-095`
- Title: `Orchestration Board, Contract, And Client (N-Child Flow + Extend-Cap)`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-29`
- Parent Documents: [CP-36: Generic Agent-Flow Engine And Review Loop](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), [SD-18: Main-Hub Agent Review Loop](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md)
- Child Documents: `None`
- Related Documents: [Task-084: Dependency Feedback Loop And Orchestration Board](../done/Task-084-Dependency-Feedback-Loop-And-Orchestration-Board.md), [Task-090: Bounded Flow Runtime Executor](./Task-090-Bounded-Flow-Runtime-Executor.md)
- Replaces: `None`
- Tags: `multi-agent, desktop, board, contract, react, typescript`

## AI Quick View

### Summary

- Generalize the desktop surface beyond the hardcoded single coder + single reviewer: contract DTOs, client methods, store actions, and an Orchestration Board that renders **N children**, the round/cap, the open count, the loop status, and a `blocked` + **Extend-cap** control.
- Builds on the Task-084 board (CP-19); adds the additive flow fields surfaced from the `agent_graph_updated` SSE snapshot.

### Current Ask

- Add `FlowControlInput`/`Result` + `ReviewOutcome*` types, the `submitReviewOutcome`/`extendCap` client methods + store actions, and generalize `OrchestrationBoard.tsx`, with frontend tests.

### Key Decisions

- `T-1` Reuse the existing `agent_graph_updated` SSE snapshot already handled in `startOrchestrationStream`; the board reads run state over the local-runner HTTP gateway (CP-36 `P-5`), not Supabase.
- `T-2` The board generalizes from "1 coder + 1 reviewer" to "hub + a row of N child nodes" driven by the cohort in the snapshot; a single-node/no-cohort run still renders (fallback).
- `T-3` On `status==blocked` the board renders an **Extend cap** control (calls `extendCap`) and surfaces `gateReason`.

### Constraints

- Run GitNexus impact analysis before editing `OrchestrationBoard.tsx`, `state/store.ts`, `client/HttpWsRunnerClient.ts`, `client/MockRunnerClient.ts`, `types/contract.ts`; warn on HIGH/CRITICAL.
- Additive contract only; existing single-agent board behavior must still render.
- **Prerequisite:** Task-084 board exists; this task generalizes it.

### Open Questions

- None.

### Source Refs

- CP-36 `P-9`; SD-18 §6/§9; Task-084 board contract.
- Anchors: `types/contract.ts:91-136,461-466` (`AgentLoopState:123`, client methods); `state/store.ts` (`startOrchestrationStream`); `components/OrchestrationBoard.tsx:42-48` (hardcoded coder+reviewer); `client/HttpWsRunnerClient.ts`, `client/MockRunnerClient.ts`; backend routes from Task-090 (`flow-control`, `extend-cap`).

## 1. Goal

The desktop renders any flow as hub + N children with round/cap, open count, status, and a `blocked` + Extend-cap control, reading run state over HTTP from the local store, while keeping the single-agent fallback.

## 2. Parent Links

- coding plan: [CP-36](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) `P-9`
- tech design: [SD-18](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md) §6
- system spec: [SS-15](../../05-System-Specs/SS-15-Agent-Review-Loop-Until-Clean.md) `AC-10`/`AC-11`
- specific upstream ids: CP-36 `P-9`; SD-18 `D-9`

## 3. Trigger

The current board (Task-084) hardcodes one coder + one reviewer; the engine produces N-child cohorts, rounds, an open-issue count, and a `blocked` state with an extend control — none of which the board renders yet.

## 4. Exact Change

- `T-1` `contract.ts`: add `FlowControlInput`/`FlowControlResult`, `ReviewIssue`/`ReviewOutcomeInput`/`ReviewOutcomeResult`; extend `AgentLoopState` (`:123`) with `openIssues?`, `mode?`, `activeNode?`; add client methods (`:461-466`) `submitReviewOutcome?(parentRunId, in): Promise<AgentGraphSnapshot>` and `extendCap?(parentRunId): Promise<AgentGraphSnapshot>`.
- `T-2` `HttpWsRunnerClient.ts` + `MockRunnerClient.ts`: implement the two methods against the Task-090 routes (`POST .../flow-control`, `POST .../agent-loop/extend-cap`); the mock returns a deterministic snapshot.
- `T-3` `state/store.ts`: add `submitReviewOutcome`/`extendCap` actions; surface `openIssues`/`mode`/`activeNode` from the `agent_graph_updated` SSE snapshot in `startOrchestrationStream`.
- `T-4` `OrchestrationBoard.tsx` (`:42-48`): render the hub node + a **row of child nodes** (one per cohort member) with per-node status; show **round R / cap**, **open count**, and loop `status` (`synthesizing`/`blocked`/`done`); on `status==blocked` render an **Extend cap** control + `gateReason`; keep pause/resume/stop/inject; keep the single-node fallback.

## 5. Touched Areas

- files: `types/contract.ts`, `state/store.ts`, `components/OrchestrationBoard.tsx`, `client/HttpWsRunnerClient.ts`, `client/MockRunnerClient.ts`, `styles.css`; tests `state/store.test.ts`, board component tests.
- modules: desktop board + state + client.
- routes: consumes Task-090's `POST .../flow-control`, `POST .../agent-loop/extend-cap`.
- tables: none.

## 6. Acceptance Check (Definition of Done)

- [ ] The board renders **N child nodes** from an `AgentGraphSnapshot` cohort, each with per-node status.
- [ ] The board shows the current **round / cap** and the **open-issue count**.
- [ ] On `status==blocked` the board renders an **Extend cap** control and surfaces `gateReason`; clicking it calls `extendCap` and updates the snapshot.
- [ ] `submitReviewOutcome` and `extendCap` call the client against the correct routes and update the store snapshot (mock + http client tests).
- [ ] `openIssues`/`mode`/`activeNode` surface from the `agent_graph_updated` SSE snapshot.
- [ ] A single-agent / no-cohort run still renders (fallback regression).
- [ ] Existing pause/resume/stop/inject controls still work.

## 7. Out of Scope

- Backend executor/routes (Task-090); the tool/config (Task-091); the note/auto-reinvoke (Tasks 092–093); persistence (Task-085); skill/agent (Task-094).

## 8. Completion Notes

- result:
- follow-ups:
- upstream docs updated:
