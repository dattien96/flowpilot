# Task-094: Review Loop Board And Contract

## Metadata

- Document ID: `Task-094`
- Title: `Review Loop Board, Contract, And Client Surface`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [CP-36: Agent Review Loop And Main-Hub Orchestration](../../07-Coding-Plan/todo/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), [SD-18: Main-Hub Agent Review Loop](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md), [SS-15: Agent Review Loop](../../05-System-Specs/SS-15-Agent-Review-Loop-Until-Clean.md)
- Child Documents: `None`
- Related Documents: [Task-084: Dependency Feedback Loop And Orchestration Board](./Task-084-Dependency-Feedback-Loop-And-Orchestration-Board.md), [Task-090: Review Loop Driver](./Task-090-Review-Loop-Driver.md)
- Replaces: `None`
- Tags: `multi-agent, review-loop, desktop, react, contract`

## AI Quick View

### Summary

- Surface the loop in the desktop app: contract types for the verdict + extended loop state, store actions, and an Orchestration Board that renders N reviewers, the round/cap, the open-issue count, the loop status, and an Extend-cap control.

### Current Ask

- Generalize the board beyond one coder + one reviewer and add the client/store wiring for `submitReviewOutcome` and `extendRoundCap`.

### Key Decisions

- `T-1` The board renders the coder node plus a ROW of reviewer nodes (one per cohort member), each with per-node status.
- `T-2` When `status==blocked`, render an Extend-cap control (disabled once `extendCount==2`) and surface the gate reason (SD-18 `D-9`).

### Constraints

- Run GitNexus impact analysis before editing `OrchestrationBoard.tsx` and `state/store.ts`; warn on HIGH/CRITICAL.
- Additive to the existing board; single-agent / no-cohort fallback must still render. Reuse the existing `agent_graph_updated` SSE snapshot.

### Open Questions

- A one-click "review until clean" UI affordance is deferred (SS-15 `Q-4`); Phase 1 is chat/skill-driven.

### Source Refs

- CP-36 `P-6`; SD-18 §6 (interfaces), §7.9; SS-15 `AC-8`, `BR-8`.
- Anchors: `types/contract.ts:91-136,461-466`; `state/store.ts` (agent actions, `startOrchestrationStream`); `components/OrchestrationBoard.tsx:42-48`; `client/HttpWsRunnerClient.ts`, `client/MockRunnerClient.ts`.

## 1. Goal

A user can watch the loop on the board — every agent and its status, each reviewer's presence, the current round/cap, the open-issue count, the verdict — and extend, pause, resume, stop, or inject feedback.

## 2. Parent Links

- coding plan: [CP-36](../../07-Coding-Plan/todo/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) `P-6`
- tech design: [SD-18](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md) §6, §7.9
- system spec: [SS-15](../../05-System-Specs/SS-15-Agent-Review-Loop-Until-Clean.md) `AC-8`
- specific upstream ids: CP-36 `P-6`

## 3. Trigger

The current board (`OrchestrationBoard.tsx:42-48`) hardcodes a single coder + single reviewer and has no open-issue/verdict/extend affordances, so the multi-reviewer loop is not observable or controllable.

## 4. Exact Change

- `T-1` `contract.ts`: add `ReviewIssue`, `ReviewOutcomeInput`, `ReviewOutcomeResult`; extend `AgentLoopState` (`openIssues?`, `mode?`, `coderRunId?`, `extendCount?`); add client methods `submitReviewOutcome?`, `extendRoundCap?`.
- `T-2` `HttpWsRunnerClient.ts` + `MockRunnerClient.ts`: implement the two methods against the Task-090 routes; mock returns a deterministic snapshot.
- `T-3` `state/store.ts`: add `submitReviewOutcome`/`extendRoundCap` actions; surface `openIssues`/`mode`/`extendCount` from the SSE snapshot.
- `T-4` `OrchestrationBoard.tsx`: render the coder + a reviewer row (one node per cohort member); show round/cap, open-issue count, status; on `blocked` show the Extend-cap control + gate reason; keep pause/resume/stop/inject.

## 5. Touched Areas

- files: `types/contract.ts`, `state/store.ts`, `components/OrchestrationBoard.tsx`, `client/HttpWsRunnerClient.ts`, `client/MockRunnerClient.ts`, `state/store.test.ts`, `styles.css`.
- modules: desktop board + state + client.
- routes: consumes Task-090 routes.
- tables: none.

## 6. Acceptance Check

- Frontend tests: board renders N reviewers from a snapshot; shows open-issue count + round; `blocked` renders the Extend-cap control (disabled at `extendCount==2`); `submitReviewOutcome`/`extendRoundCap` call the client and update the snapshot; single-agent/no-cohort fallback still renders.
- Manual: the board reflects a live multi-round loop and the cap-hit gate.

## 7. Out of Scope

- Runner behavior (Tasks 089–092); the skill (Task-093); the legacy-mode gate (Task-095).

## 8. Completion Notes

- result:
- follow-ups:
- upstream docs updated:
